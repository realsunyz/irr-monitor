package nrtm

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type RIRConfig struct {
	Name      string
	Host      string
	Port      int
	Source    string
	SerialURL string
}

func DefaultRIRConfigs() map[string]RIRConfig {
	return map[string]RIRConfig{
		"RIPE": {
			Name:      "RIPE",
			Host:      "whois.ripe.net",
			Port:      4444,
			Source:    "RIPE",
			SerialURL: "https://ftp.ripe.net/ripe/dbase/RIPE.CURRENTSERIAL",
		},
		"APNIC": {
			Name:      "APNIC",
			Host:      "whois.apnic.net",
			Port:      43,
			Source:    "APNIC",
			SerialURL: "https://ftp.apnic.net/apnic/whois/APNIC.CURRENTSERIAL",
		},
	}
}

type Client struct {
	Config  RIRConfig
	Timeout time.Duration
}

func NewClient(config RIRConfig) *Client {
	return &Client{
		Config:  config,
		Timeout: 30 * time.Second,
	}
}

type Update struct {
	Operation string
	Serial    int64
	Object    *RPSLObject
}

func (c *Client) GetCurrentSerial() (int64, error) {
	client := &http.Client{Timeout: c.Timeout}

	req, err := http.NewRequest("GET", c.Config.SerialURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "irr-monitor/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch current serial: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}

	serialStr := strings.TrimSpace(string(body))
	serial, err := strconv.ParseInt(serialStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse serial number '%s': %w", serialStr, err)
	}

	return serial, nil
}

func (c *Client) FetchUpdates(fromSerial int64) ([]Update, error) {
	addr := net.JoinHostPort(c.Config.Host, fmt.Sprintf("%d", c.Config.Port))

	conn, err := net.DialTimeout("tcp", addr, c.Timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(c.Timeout * 2)); err != nil {
		return nil, fmt.Errorf("failed to set deadline: %w", err)
	}

	query := fmt.Sprintf("-g %s:3:%d-LAST\n", c.Config.Source, fromSerial)
	if _, err := conn.Write([]byte(query)); err != nil {
		return nil, fmt.Errorf("failed to send query: %w", err)
	}

	return c.parseResponse(conn)
}

func (c *Client) parseResponse(r io.Reader) ([]Update, error) {
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
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if inObject && len(objectLines) > 0 {
				obj := ParseRPSLObject(strings.Join(objectLines, "\n"))
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
				obj := ParseRPSLObject(strings.Join(objectLines, "\n"))
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
		obj := ParseRPSLObject(strings.Join(objectLines, "\n"))
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
