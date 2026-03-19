package nrtm

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Registry struct {
	Name      string
	Source    string
	Host      string
	Port      int
	SerialURL string
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

type Client struct {
	registry Registry
	timeout  time.Duration
}

func NewClient(registry Registry, timeout time.Duration) *Client {
	return &Client{
		registry: registry,
		timeout:  timeout,
	}
}

func (c *Client) Registry() Registry {
	return c.registry
}

func (c *Client) CurrentSerial(ctx context.Context) (int64, error) {
	client := &http.Client{Timeout: c.timeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.registry.SerialURL, nil)
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

func (c *Client) Updates(ctx context.Context, fromSerial int64) ([]Update, error) {
	var updates []Update
	err := c.withQuery(ctx, fromSerial, func(r io.Reader) error {
		parsed, err := ParseResponse(r)
		if err != nil {
			return err
		}
		updates = parsed
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *Client) FirstResponseLine(ctx context.Context, fromSerial int64) (string, error) {
	var line string
	err := c.withQuery(ctx, fromSerial, func(r io.Reader) error {
		first, err := ReadFirstResponseLine(r)
		if err != nil {
			return err
		}
		line = first
		return nil
	})
	if err != nil {
		return "", err
	}
	return line, nil
}

func (c *Client) withQuery(ctx context.Context, fromSerial int64, consume func(io.Reader) error) error {
	addr := net.JoinHostPort(c.registry.Host, strconv.Itoa(c.registry.Port))

	dialer := &net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	} else {
		if err := conn.SetDeadline(time.Now().Add(c.timeout * 2)); err != nil {
			return err
		}
	}

	query := fmt.Sprintf("-g %s:3:%d-LAST\n", c.registry.Source, fromSerial)
	if _, err := conn.Write([]byte(query)); err != nil {
		return err
	}

	return consume(conn)
}

func ParseResponse(r io.Reader) ([]Update, error) {
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

func ParseRPSLObject(text string) *RPSLObject {
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

func ReadFirstResponseLine(r io.Reader) (string, error) {
	reader := bufio.NewReader(r)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && line != "" {
				return SanitizeLine(line), nil
			}
			return "", err
		}

		line = SanitizeLine(line)
		if line == "" {
			continue
		}

		return line, nil
	}
}

func SanitizeLine(line string) string {
	return strings.TrimSpace(strings.TrimRight(line, "\r\n"))
}
