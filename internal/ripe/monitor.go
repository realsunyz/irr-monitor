package ripe

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/realSunyz/irr-monitor/internal/state"
	"github.com/realSunyz/irr-monitor/internal/telegram"
)

const (
	Host      = "whois.ripe.net"
	Port      = 4444
	Source    = "RIPE"
	SerialURL = "https://ftp.ripe.net/ripe/dbase/RIPE.CURRENTSERIAL"
)

type AutNum struct {
	ASN     string
	AsName  string
	Descr   string
	Country string
	Org     string
	Created string
}

type Update struct {
	Operation string
	Serial    int64
	Object    *RPSLObject
}

type RPSLObject struct {
	Type       string
	Attributes map[string]string
}

type Monitor struct {
	state        *state.State
	pollInterval time.Duration
	callback     func(source string, autNum *telegram.AutNum)
	timeout      time.Duration
}

func NewMonitor(st *state.State, pollInterval time.Duration, callback func(string, *telegram.AutNum)) *Monitor {
	return &Monitor{
		state:        st,
		pollInterval: pollInterval,
		callback:     callback,
		timeout:      30 * time.Second,
	}
}

func (m *Monitor) Start(ctx context.Context) {
	m.initializeSerial()

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	m.poll()

	for {
		select {
		case <-ctx.Done():
			log.Println("RIPE Monitor: Shutting down...")
			return
		case <-ticker.C:
			m.poll()
		}
	}
}

func (m *Monitor) initializeSerial() {
	if m.state.GetSerial(Source) == 0 {
		serial, err := m.getCurrentSerial()
		if err != nil {
			log.Printf("RIPE Monitor: Failed to get current serial: %v", err)
			return
		}
		m.state.SetSerial(Source, serial)
		log.Printf("RIPE Monitor: Initialized at serial %d", serial)
	} else {
		log.Printf("RIPE Monitor: Resuming from serial %d", m.state.GetSerial(Source))
	}

	telegram.Status.UpdateRIPE(m.state.GetSerial(Source), "")

	if err := m.state.Save(); err != nil {
		log.Printf("RIPE Monitor: Failed to save state: %v", err)
	}
}

func (m *Monitor) getCurrentSerial() (int64, error) {
	client := &http.Client{Timeout: m.timeout}

	req, err := http.NewRequest("GET", SerialURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "irr-monitor/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)
}

func (m *Monitor) poll() {
	fromSerial := m.state.GetSerial(Source)
	if fromSerial == 0 {
		return
	}

	updates, err := m.fetchUpdates(fromSerial + 1)
	if err != nil {
		log.Printf("RIPE Monitor: Error fetching updates: %v", err)
		return
	}

	if len(updates) == 0 {
		return
	}

	log.Printf("RIPE Monitor: Received %d updates", len(updates))

	var maxSerial int64
	var wg sync.WaitGroup

	for _, update := range updates {
		if update.Serial > maxSerial {
			maxSerial = update.Serial
		}

		if update.Operation != "ADD" || update.Object == nil || update.Object.Type != "aut-num" {
			continue
		}

		autNum := m.parseAutNum(update.Object)
		if autNum == nil {
			continue
		}

		// Check if truly new (created within 24h)
		if autNum.Created != "" {
			if createdTime, err := time.Parse("2006-01-02T15:04:05Z", autNum.Created); err == nil {
				if time.Since(createdTime) > 24*time.Hour {
					continue
				}
			}
		}

		// Lookup org's country
		if autNum.Org != "" {
			if country := m.queryOrgCountry(autNum.Org); country != "" {
				autNum.Country = country
			}
		}

		log.Printf("RIPE Monitor: New ASN: %s (%s)", autNum.ASN, autNum.AsName)

		tgAutNum := &telegram.AutNum{
			ASN:     autNum.ASN,
			AsName:  autNum.AsName,
			Descr:   autNum.Descr,
			Country: autNum.Country,
			Source:  Source,
		}

		if m.callback != nil {
			wg.Add(1)
			go func(an *telegram.AutNum) {
				defer wg.Done()
				m.callback(Source, an)
			}(tgAutNum)
		}
	}

	wg.Wait()

	if maxSerial > 0 {
		m.state.UpdateSerial(Source, maxSerial)
		telegram.Status.UpdateRIPE(maxSerial, "")
		if err := m.state.Save(); err != nil {
			log.Printf("RIPE Monitor: Failed to save state: %v", err)
		}
	}
}

func (m *Monitor) fetchUpdates(fromSerial int64) ([]Update, error) {
	addr := net.JoinHostPort(Host, fmt.Sprintf("%d", Port))

	conn, err := net.DialTimeout("tcp", addr, m.timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(m.timeout * 2))

	query := fmt.Sprintf("-g %s:3:%d-LAST\n", Source, fromSerial)
	if _, err := conn.Write([]byte(query)); err != nil {
		return nil, err
	}

	return m.parseResponse(conn)
}

func (m *Monitor) parseResponse(r io.Reader) ([]Update, error) {
	var updates []Update
	reader := bufio.NewReader(r)

	var currentOp string
	var currentSerial int64
	var objectLines []string
	inObject := false

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			if len(updates) > 0 {
				break
			}
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if inObject && len(objectLines) > 0 {
				obj := m.parseRPSLObject(strings.Join(objectLines, "\n"))
				if obj != nil {
					updates = append(updates, Update{
						Operation: currentOp,
						Serial:    currentSerial,
						Object:    obj,
					})
				}
				objectLines = nil
				inObject = false
			}
			continue
		}

		if strings.HasPrefix(line, "%START") {
			continue
		}
		if strings.HasPrefix(line, "%END") {
			break
		}
		if strings.HasPrefix(line, "%") {
			continue
		}

		if strings.HasPrefix(line, "ADD ") || strings.HasPrefix(line, "DEL ") {
			if inObject && len(objectLines) > 0 {
				obj := m.parseRPSLObject(strings.Join(objectLines, "\n"))
				if obj != nil {
					updates = append(updates, Update{
						Operation: currentOp,
						Serial:    currentSerial,
						Object:    obj,
					})
				}
				objectLines = nil
			}

			parts := strings.SplitN(line, " ", 2)
			currentOp = parts[0]
			if len(parts) > 1 {
				currentSerial, _ = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
			}
			inObject = true
			continue
		}

		if inObject {
			objectLines = append(objectLines, line)
		}
	}

	if inObject && len(objectLines) > 0 {
		obj := m.parseRPSLObject(strings.Join(objectLines, "\n"))
		if obj != nil {
			updates = append(updates, Update{
				Operation: currentOp,
				Serial:    currentSerial,
				Object:    obj,
			})
		}
	}

	return updates, nil
}

func (m *Monitor) parseRPSLObject(text string) *RPSLObject {
	if text == "" {
		return nil
	}

	obj := &RPSLObject{Attributes: make(map[string]string)}

	lines := strings.Split(text, "\n")
	var currentAttr string
	var currentValue strings.Builder

	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}

		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t' || line[0] == '+') {
			if currentAttr != "" {
				currentValue.WriteString(" ")
				currentValue.WriteString(strings.TrimSpace(line))
			}
			continue
		}

		if currentAttr != "" {
			if _, exists := obj.Attributes[currentAttr]; !exists {
				obj.Attributes[currentAttr] = currentValue.String()
			}
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		currentAttr = strings.TrimSpace(line[:colonIdx])
		currentValue.Reset()
		currentValue.WriteString(strings.TrimSpace(line[colonIdx+1:]))

		if obj.Type == "" {
			obj.Type = currentAttr
		}
	}

	if currentAttr != "" {
		if _, exists := obj.Attributes[currentAttr]; !exists {
			obj.Attributes[currentAttr] = currentValue.String()
		}
	}

	if obj.Type == "" {
		return nil
	}

	return obj
}

func (m *Monitor) parseAutNum(obj *RPSLObject) *AutNum {
	return &AutNum{
		ASN:     obj.Attributes["aut-num"],
		AsName:  obj.Attributes["as-name"],
		Descr:   obj.Attributes["descr"],
		Country: obj.Attributes["country"],
		Org:     obj.Attributes["org"],
		Created: obj.Attributes["created"],
	}
}

func (m *Monitor) queryOrgCountry(orgID string) string {
	conn, err := net.DialTimeout("tcp", "whois.ripe.net:43", 10*time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	query := fmt.Sprintf("-r -T organisation %s\n", orgID)
	conn.Write([]byte(query))

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "country:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "country:"))
		}
	}

	return ""
}
