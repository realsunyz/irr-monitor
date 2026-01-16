package nrtm

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"
)

func QueryOrgCountry(orgID string) (string, error) {
	conn, err := net.DialTimeout("tcp", "whois.ripe.net:43", 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	query := fmt.Sprintf("-r -T organisation %s\n", orgID)
	if _, err := conn.Write([]byte(query)); err != nil {
		return "", fmt.Errorf("failed to send query: %w", err)
	}

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "country:") {
			country := strings.TrimSpace(strings.TrimPrefix(line, "country:"))
			return country, nil
		}
	}

	return "", nil
}
