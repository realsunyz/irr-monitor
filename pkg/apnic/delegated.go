package apnic

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const DelegatedURL = "https://ftp.apnic.net/stats/apnic/delegated-apnic-latest"

type ASNEntry struct {
	ASN     string
	Country string
	Date    string
}

type DelegatedData struct {
	ASNs      map[string]ASNEntry
	FetchTime time.Time
	FilePath  string
}

func FetchAndSaveDelegatedData(dataDir string) (*DelegatedData, error) {
	client := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequest("GET", DelegatedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "irr-monitor/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch delegated file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	dateStr := time.Now().UTC().Format("20060102")
	filename := fmt.Sprintf("delegated-apnic-%s", dateStr)
	filePath := filepath.Join(dataDir, filename)

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	if err := os.WriteFile(filePath, body, 0644); err != nil {
		return nil, fmt.Errorf("failed to save delegated file: %w", err)
	}

	data, err := parseDelegatedData(strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	data.FilePath = filePath

	return data, nil
}

func LoadLatestDelegatedData(dataDir string) (*DelegatedData, error) {
	files, err := filepath.Glob(filepath.Join(dataDir, "delegated-apnic-*"))
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, nil
	}

	sort.Strings(files)
	latestFile := files[len(files)-1]

	f, err := os.Open(latestFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", latestFile, err)
	}
	defer f.Close()

	data, err := parseDelegatedData(f)
	if err != nil {
		return nil, err
	}
	data.FilePath = latestFile

	return data, nil
}

func CleanupOldFiles(dataDir string, keepCount int) {
	files, err := filepath.Glob(filepath.Join(dataDir, "delegated-apnic-*"))
	if err != nil || len(files) <= keepCount {
		return
	}

	sort.Strings(files)
	for _, f := range files[:len(files)-keepCount] {
		os.Remove(f)
	}
}

func parseDelegatedData(r io.Reader) (*DelegatedData, error) {
	data := &DelegatedData{
		ASNs:      make(map[string]ASNEntry),
		FetchTime: time.Now(),
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 7 {
			continue
		}

		registry := parts[0]
		cc := parts[1]
		recordType := parts[2]
		start := parts[3]
		date := parts[5]
		status := parts[6]

		if registry != "apnic" {
			continue
		}

		if recordType != "asn" {
			continue
		}

		if status != "allocated" && status != "assigned" {
			continue
		}

		asn := fmt.Sprintf("AS%s", start)
		data.ASNs[asn] = ASNEntry{
			ASN:     asn,
			Country: cc,
			Date:    date,
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse delegated data: %w", err)
	}

	return data, nil
}

func CompareData(oldData, newData *DelegatedData) []ASNEntry {
	var newASNs []ASNEntry

	for asn, entry := range newData.ASNs {
		if _, exists := oldData.ASNs[asn]; !exists {
			newASNs = append(newASNs, entry)
		}
	}

	return newASNs
}

type ASNInfo struct {
	ASN     string
	AsName  string
	Descr   string
	Country string
}

func QueryASNInfo(asn string) (*ASNInfo, error) {
	conn, err := net.DialTimeout("tcp", "whois.apnic.net:43", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	query := fmt.Sprintf("%s\n", asn)
	if _, err := conn.Write([]byte(query)); err != nil {
		return nil, fmt.Errorf("failed to send query: %w", err)
	}

	info := &ASNInfo{ASN: asn}
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "as-name:") {
			info.AsName = strings.TrimSpace(strings.TrimPrefix(line, "as-name:"))
		} else if strings.HasPrefix(line, "descr:") && info.Descr == "" {
			info.Descr = strings.TrimSpace(strings.TrimPrefix(line, "descr:"))
		} else if strings.HasPrefix(line, "country:") && info.Country == "" {
			info.Country = strings.TrimSpace(strings.TrimPrefix(line, "country:"))
		}
	}

	return info, nil
}
