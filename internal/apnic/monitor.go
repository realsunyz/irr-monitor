package apnic

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/realSunyz/irr-monitor/internal/telegram"
)

const DelegatedURL = "https://ftp.apnic.net/stats/apnic/delegated-apnic-latest"

type ASNEntry struct {
	ASN     string
	Country string
}

type DelegatedData struct {
	ASNs     map[string]ASNEntry
	FilePath string
}

type Monitor struct {
	previousData *DelegatedData
	callback     func(source string, autNum *telegram.AutNum)
	dataDir      string
}

func NewMonitor(dataDir string, callback func(string, *telegram.AutNum)) *Monitor {
	return &Monitor{
		callback: callback,
		dataDir:  dataDir,
	}
}

func (m *Monitor) Start(ctx context.Context) {
	data, err := m.loadLatestData()
	if err != nil {
		log.Printf("[APNIC Monitor] Failed to load existing data: %v", err)
	}

	if data != nil {
		m.previousData = data
		log.Printf("[APNIC Monitor] Loaded %d ASNs from %s", len(data.ASNs), filepath.Base(data.FilePath))
		telegram.Status.UpdateAPNIC(len(data.ASNs), filepath.Base(data.FilePath), "")
	} else {
		log.Println("[APNIC Monitor] No existing data, fetching fresh...")
		newData, err := m.fetchAndSave()
		if err != nil {
			log.Printf("[APNIC Monitor] Failed to fetch initial data: %v", err)
		} else {
			m.previousData = newData
			log.Printf("[APNIC Monitor] Saved %d ASNs to %s", len(newData.ASNs), filepath.Base(newData.FilePath))
			telegram.Status.UpdateAPNIC(len(newData.ASNs), filepath.Base(newData.FilePath), "")
		}
	}

	m.scheduleDaily(ctx)
}

func (m *Monitor) scheduleDaily(ctx context.Context) {
	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), 16, 0, 0, 0, time.UTC)
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}

		waitDuration := next.Sub(now)
		log.Printf("[APNIC Monitor] Next check at %s (in %s)", next.Format(time.RFC3339), waitDuration)

		select {
		case <-ctx.Done():
			log.Println("[APNIC Monitor] Shutting down...")
			return
		case <-time.After(waitDuration):
			m.checkForNewASNs()
		}
	}
}

func (m *Monitor) checkForNewASNs() {
	log.Println("[APNIC Monitor] Fetching delegated data...")
	newData, err := m.fetchAndSave()
	if err != nil {
		log.Printf("[APNIC Monitor] Failed to fetch data: %v", err)
		return
	}

	log.Printf("[APNIC Monitor] Saved to %s", filepath.Base(newData.FilePath))
	m.cleanupOldFiles(2)

	if m.previousData == nil {
		m.previousData = newData
		telegram.Status.UpdateAPNIC(len(newData.ASNs), filepath.Base(newData.FilePath), "")
		return
	}

	var newASNs []ASNEntry
	for asn, entry := range newData.ASNs {
		if _, exists := m.previousData.ASNs[asn]; !exists {
			newASNs = append(newASNs, entry)
		}
	}

	log.Printf("[APNIC Monitor] Found %d new ASNs", len(newASNs))

	for _, entry := range newASNs {
		info := m.queryASNInfo(entry.ASN)
		if info == nil {
			info = &telegram.AutNum{
				ASN:     entry.ASN,
				Country: entry.Country,
				Source:  "APNIC",
			}
		}

		log.Printf("[APNIC Monitor] New ASN: %s (%s)", info.ASN, info.AsName)

		if m.callback != nil {
			m.callback("APNIC", info)
		}
	}

	m.previousData = newData
	telegram.Status.UpdateAPNIC(len(newData.ASNs), filepath.Base(newData.FilePath), "")
}

func (m *Monitor) fetchAndSave() (*DelegatedData, error) {
	client := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequest("GET", DelegatedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "irr-monitor/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	dateStr := time.Now().UTC().Format("20060102")
	filename := fmt.Sprintf("delegated-apnic-%s", dateStr)
	filePath := filepath.Join(m.dataDir, filename)

	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return nil, err
	}

	if err := os.WriteFile(filePath, body, 0644); err != nil {
		return nil, err
	}

	data := m.parseData(strings.NewReader(string(body)))
	data.FilePath = filePath

	return data, nil
}

func (m *Monitor) loadLatestData() (*DelegatedData, error) {
	files, err := filepath.Glob(filepath.Join(m.dataDir, "delegated-apnic-*"))
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
		return nil, err
	}
	defer f.Close()

	data := m.parseData(f)
	data.FilePath = latestFile

	return data, nil
}

func (m *Monitor) parseData(r io.Reader) *DelegatedData {
	data := &DelegatedData{ASNs: make(map[string]ASNEntry)}

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

		if parts[0] != "apnic" || parts[2] != "asn" {
			continue
		}

		if parts[6] != "allocated" && parts[6] != "assigned" {
			continue
		}

		asn := fmt.Sprintf("AS%s", parts[3])
		data.ASNs[asn] = ASNEntry{ASN: asn, Country: parts[1]}
	}

	return data
}

func (m *Monitor) cleanupOldFiles(keep int) {
	files, _ := filepath.Glob(filepath.Join(m.dataDir, "delegated-apnic-*"))
	if len(files) <= keep {
		return
	}

	sort.Strings(files)
	for _, f := range files[:len(files)-keep] {
		os.Remove(f)
	}
}

func (m *Monitor) queryASNInfo(asn string) *telegram.AutNum {
	conn, err := net.DialTimeout("tcp", "whois.apnic.net:43", 10*time.Second)
	if err != nil {
		return nil
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))
	conn.Write([]byte(asn + "\n"))

	info := &telegram.AutNum{ASN: asn, Source: "APNIC"}
	var org string
	var sponsoringOrg string
	var mntBy string
	inAutNum := false

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "aut-num:") {
			inAutNum = true
			continue
		}

		if line == "" {
			if inAutNum {
				break
			}
			continue
		}

		if !inAutNum {
			continue
		}

		if strings.HasPrefix(line, "as-name:") && info.AsName == "" {
			info.AsName = strings.TrimSpace(strings.TrimPrefix(line, "as-name:"))
		} else if strings.HasPrefix(line, "descr:") && info.Descr == "" {
			info.Descr = strings.TrimSpace(strings.TrimPrefix(line, "descr:"))
		} else if strings.HasPrefix(line, "country:") && info.Country == "" {
			info.Country = strings.TrimSpace(strings.TrimPrefix(line, "country:"))
		} else if strings.HasPrefix(line, "org:") && org == "" {
			org = strings.TrimSpace(strings.TrimPrefix(line, "org:"))
		} else if strings.HasPrefix(line, "sponsoring-org:") && sponsoringOrg == "" {
			sponsoringOrg = strings.TrimSpace(strings.TrimPrefix(line, "sponsoring-org:"))
		} else if strings.HasPrefix(line, "mnt-by:") && mntBy == "" {
			mntBy = strings.TrimSpace(strings.TrimPrefix(line, "mnt-by:"))
		}
	}

	info.MntBy = mntBy

	if org != "" {
		info.Org = org
		orgName, orgType, orgCountry := m.queryOrgInfo(org)
		info.OrgName = orgName
		info.OrgType = orgType
		info.OrgCountry = orgCountry
	}

	if sponsoringOrg != "" {
		info.SponsoringOrg = sponsoringOrg
		sponsoringOrgName, _, _ := m.queryOrgInfo(sponsoringOrg)
		info.SponsoringOrgName = sponsoringOrgName
	}

	return info
}

func (m *Monitor) queryOrgInfo(orgID string) (orgName, orgType, country string) {
	conn, err := net.DialTimeout("tcp", "whois.apnic.net:43", 10*time.Second)
	if err != nil {
		return "", "", ""
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))
	conn.Write([]byte(orgID + "\n"))

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
