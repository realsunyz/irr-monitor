package ripe

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/realSunyz/irr-monitor/internal/nrtm"
	"github.com/realSunyz/irr-monitor/internal/state"
	"github.com/realSunyz/irr-monitor/internal/telegram"
)

const Source = "RIPE"

const (
	DelegatedURL            = "https://ftp.ripe.net/pub/stats/ripencc/delegated-ripencc-latest"
	delegatedFilenamePrefix = "delegated-ripencc-"
)

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

type DelegatedData struct {
	ASNs     map[string]struct{}
	FilePath string
}

type Monitor struct {
	state        *state.State
	dataDir      string
	pollInterval time.Duration
	callback     func(source string, autNum *telegram.AutNum)
	client       *nrtm.Client
	delegatedMu  sync.RWMutex
	delegated    *DelegatedData
	newlyAdded   map[string]struct{}
}

func NewMonitor(st *state.State, dataDir string, pollInterval time.Duration, callback func(string, *telegram.AutNum)) *Monitor {
	timeout := 30 * time.Second
	return &Monitor{
		state:        st,
		dataDir:      dataDir,
		pollInterval: pollInterval,
		callback:     callback,
		client:       nrtm.NewClient(Registry, timeout),
		newlyAdded:   make(map[string]struct{}),
	}
}

func (m *Monitor) Start(ctx context.Context) {
	m.initializeDelegatedData()
	m.initializeSerial()
	go m.scheduleDelegatedRefresh(ctx)

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

func (m *Monitor) initializeDelegatedData() {
	latest, previous, err := m.loadRecentDelegatedData()
	if err != nil {
		log.Printf("[RIPE Monitor] Failed to load delegated data: %v", err)
	}

	if latest == nil {
		log.Println("[RIPE Monitor] No delegated baseline found, fetching fresh...")
		fetched, err := m.fetchAndSaveDelegated()
		if err != nil {
			log.Printf("[RIPE Monitor] Failed to fetch delegated baseline: %v", err)
			return
		}

		m.setDelegatedData(fetched, nil)
		m.cleanupOldDelegatedFiles(2)
		log.Printf("[RIPE Monitor] Saved delegated baseline with %d ASNs to %s", len(fetched.ASNs), filepath.Base(fetched.FilePath))
		return
	}

	newlyAdded := diffDelegatedData(previous, latest)
	m.setDelegatedData(latest, newlyAdded)

	if previous != nil {
		log.Printf("[RIPE Monitor] Loaded delegated baseline with %d ASNs from %s (%d newly added vs previous snapshot)", len(latest.ASNs), filepath.Base(latest.FilePath), len(newlyAdded))
		return
	}

	log.Printf("[RIPE Monitor] Loaded delegated baseline with %d ASNs from %s", len(latest.ASNs), filepath.Base(latest.FilePath))
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

func (m *Monitor) scheduleDelegatedRefresh(ctx context.Context) {
	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		if !now.Before(next) {
			next = next.Add(24 * time.Hour)
		}

		waitDuration := next.Sub(now)
		log.Printf("[RIPE Monitor] Next delegated refresh at %s (in %s)", next.Format(time.RFC3339), waitDuration)

		select {
		case <-ctx.Done():
			return
		case <-time.After(waitDuration):
			m.refreshDelegatedData()
		}
	}
}

func (m *Monitor) refreshDelegatedData() {
	log.Println("[RIPE Monitor] Refreshing delegated baseline...")

	newData, err := m.fetchAndSaveDelegated()
	if err != nil {
		log.Printf("[RIPE Monitor] Failed to refresh delegated baseline: %v", err)
		return
	}

	previous := m.currentDelegatedData()
	newlyAdded := diffDelegatedData(previous, newData)
	m.setDelegatedData(newData, newlyAdded)
	m.cleanupOldDelegatedFiles(2)

	log.Printf("[RIPE Monitor] Updated delegated baseline to %s with %d ASNs (%d newly added vs previous snapshot)", filepath.Base(newData.FilePath), len(newData.ASNs), len(newlyAdded))
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
		if !m.shouldNotifyASN(autNum.ASN) {
			log.Printf("[RIPE Monitor] Skipping historical ASN already present in delegated baseline: %s", autNum.ASN)
			continue
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

func (m *Monitor) fetchAndSaveDelegated() (*DelegatedData, error) {
	client := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequest(http.MethodGet, DelegatedURL, nil)
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

	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return nil, err
	}

	filename := fmt.Sprintf("%s%s", delegatedFilenamePrefix, time.Now().UTC().Format("20060102"))
	filePath := filepath.Join(m.dataDir, filename)
	if err := os.WriteFile(filePath, body, 0644); err != nil {
		return nil, err
	}

	data := m.parseDelegatedData(bytes.NewReader(body))
	data.FilePath = filePath

	return data, nil
}

func (m *Monitor) loadRecentDelegatedData() (latest *DelegatedData, previous *DelegatedData, err error) {
	files, err := filepath.Glob(filepath.Join(m.dataDir, delegatedFilenamePrefix+"*"))
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, nil
	}

	sort.Strings(files)

	latest, err = m.loadDelegatedFile(files[len(files)-1])
	if err != nil {
		return nil, nil, err
	}

	if len(files) > 1 {
		previous, err = m.loadDelegatedFile(files[len(files)-2])
		if err != nil {
			return nil, nil, err
		}
	}

	return latest, previous, nil
}

func (m *Monitor) loadDelegatedFile(filePath string) (*DelegatedData, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := m.parseDelegatedData(f)
	data.FilePath = filePath

	return data, nil
}

func (m *Monitor) parseDelegatedData(r io.Reader) *DelegatedData {
	data := &DelegatedData{ASNs: make(map[string]struct{})}

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
		if parts[0] != "ripencc" && parts[0] != "ripe" {
			continue
		}
		if parts[2] != "asn" {
			continue
		}
		if parts[6] != "allocated" && parts[6] != "assigned" {
			continue
		}

		start, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil {
			continue
		}
		count, err := strconv.ParseInt(parts[4], 10, 64)
		if err != nil || count <= 0 {
			continue
		}

		for i := int64(0); i < count; i++ {
			data.ASNs[fmt.Sprintf("AS%d", start+i)] = struct{}{}
		}
	}

	return data
}

func (m *Monitor) cleanupOldDelegatedFiles(keep int) {
	files, _ := filepath.Glob(filepath.Join(m.dataDir, delegatedFilenamePrefix+"*"))
	if len(files) <= keep {
		return
	}

	sort.Strings(files)
	for _, filePath := range files[:len(files)-keep] {
		if err := os.Remove(filePath); err != nil {
			log.Printf("[RIPE Monitor] Failed to remove old delegated snapshot %s: %v", filepath.Base(filePath), err)
		}
	}
}

func (m *Monitor) setDelegatedData(data *DelegatedData, newlyAdded map[string]struct{}) {
	if newlyAdded == nil {
		newlyAdded = make(map[string]struct{})
	}

	m.delegatedMu.Lock()
	defer m.delegatedMu.Unlock()
	m.delegated = data
	m.newlyAdded = newlyAdded
}

func (m *Monitor) currentDelegatedData() *DelegatedData {
	m.delegatedMu.RLock()
	defer m.delegatedMu.RUnlock()
	return m.delegated
}

func (m *Monitor) shouldNotifyASN(asn string) bool {
	m.delegatedMu.RLock()
	defer m.delegatedMu.RUnlock()

	if _, ok := m.newlyAdded[asn]; ok {
		return true
	}
	if m.delegated == nil {
		return true
	}
	if _, exists := m.delegated.ASNs[asn]; exists {
		return false
	}
	return true
}

func diffDelegatedData(previous, current *DelegatedData) map[string]struct{} {
	newlyAdded := make(map[string]struct{})
	if previous == nil || current == nil {
		return newlyAdded
	}

	for asn := range current.ASNs {
		if _, exists := previous.ASNs[asn]; !exists {
			newlyAdded[asn] = struct{}{}
		}
	}

	return newlyAdded
}
