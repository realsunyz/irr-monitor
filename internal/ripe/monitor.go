package ripe

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/realSunyz/irr-monitor/internal/nrtm"
	"github.com/realSunyz/irr-monitor/internal/state"
	"github.com/realSunyz/irr-monitor/internal/telegram"
)

const Source = "RIPE"

var Registry = nrtm.Registry{
	Name:      Source,
	Source:    Source,
	Host:      "whois.ripe.net",
	Port:      4444,
	SerialURL: "https://ftp.ripe.net/ripe/dbase/RIPE.CURRENTSERIAL",
}

type AutNum struct {
	ASN           string
	AsName        string
	Descr         string
	Country       string
	Org           string
	SponsoringOrg string
	Created       string
	LastModified  string
}

type Monitor struct {
	state        *state.State
	pollInterval time.Duration
	callback     func(source string, autNum *telegram.AutNum)
	client       *nrtm.Client
}

func NewMonitor(st *state.State, pollInterval time.Duration, callback func(string, *telegram.AutNum)) *Monitor {
	timeout := 30 * time.Second
	return &Monitor{
		state:        st,
		pollInterval: pollInterval,
		callback:     callback,
		client:       nrtm.NewClient(Registry, timeout),
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
			log.Println("[RIPE Monitor] Shutting down...")
			return
		case <-ticker.C:
			m.poll()
		}
	}
}

func (m *Monitor) initializeSerial() {
	if m.state.GetSerial(Source) == 0 {
		serial, err := m.client.CurrentSerial(context.Background())
		if err != nil {
			log.Printf("[RIPE Monitor] Failed to get current serial: %v", err)
			return
		}
		m.state.SetSerial(Source, serial)
		log.Printf("[RIPE Monitor] Initialized at serial %d", serial)
	} else {
		log.Printf("[RIPE Monitor] Resuming from serial %d", m.state.GetSerial(Source))
	}

	telegram.Status.UpdateRIPE(m.state.GetSerial(Source), "")

	if err := m.state.Save(); err != nil {
		log.Printf("[RIPE Monitor] Failed to save state: %v", err)
	}
}

func (m *Monitor) poll() {
	fromSerial := m.state.GetSerial(Source)
	if fromSerial == 0 {
		return
	}

	updates, err := m.fetchUpdates(fromSerial + 1)
	if err != nil {
		log.Printf("[RIPE Monitor] Error fetching updates: %v", err)
		return
	}

	if len(updates) == 0 {
		return
	}

	log.Printf("[RIPE Monitor] Received %d updates", len(updates))

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

		if autNum.Created != "" && autNum.LastModified != "" {
			if autNum.Created != autNum.LastModified {
				continue
			}
		}

		var orgName, orgType, orgCountry string
		if autNum.Org != "" {
			orgName, orgType, orgCountry = m.queryOrgInfo(autNum.Org)
		}

		var sponsoringOrgName string
		if autNum.SponsoringOrg != "" {
			sponsoringOrgName, _, _ = m.queryOrgInfo(autNum.SponsoringOrg)
		}

		log.Printf("[RIPE Monitor] New ASN: %s (%s)", autNum.ASN, autNum.AsName)

		tgAutNum := &telegram.AutNum{
			ASN:               autNum.ASN,
			AsName:            autNum.AsName,
			Descr:             autNum.Descr,
			Country:           autNum.Country,
			Org:               autNum.Org,
			OrgName:           orgName,
			OrgType:           orgType,
			OrgCountry:        orgCountry,
			SponsoringOrg:     autNum.SponsoringOrg,
			SponsoringOrgName: sponsoringOrgName,
			Source:            Source,
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
			log.Printf("[RIPE Monitor] Failed to save state: %v", err)
		}
	}
}

func (m *Monitor) fetchUpdates(fromSerial int64) ([]nrtm.Update, error) {
	return m.client.Updates(context.Background(), fromSerial)
}

func (m *Monitor) parseAutNum(obj *nrtm.RPSLObject) *AutNum {
	return &AutNum{
		ASN:           obj.Attributes["aut-num"],
		AsName:        obj.Attributes["as-name"],
		Descr:         obj.Attributes["descr"],
		Country:       obj.Attributes["country"],
		Org:           obj.Attributes["org"],
		SponsoringOrg: obj.Attributes["sponsoring-org"],
		Created:       obj.Attributes["created"],
		LastModified:  obj.Attributes["last-modified"],
	}
}

func (m *Monitor) queryOrgInfo(orgID string) (orgName, orgType, country string) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(Registry.Host, "43"), 10*time.Second)
	if err != nil {
		return "", "", ""
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	query := fmt.Sprintf("-r -T organisation %s\n", orgID)
	conn.Write([]byte(query))

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "org-name:") && orgName == "" {
			orgName = strings.TrimSpace(strings.TrimPrefix(line, "org-name:"))
		} else if strings.HasPrefix(line, "org-type:") && orgType == "" {
			orgType = strings.TrimSpace(strings.TrimPrefix(line, "org-type:"))
		} else if strings.HasPrefix(line, "country:") && country == "" {
			country = strings.TrimSpace(strings.TrimPrefix(line, "country:"))
		}
	}

	return orgName, orgType, country
}
