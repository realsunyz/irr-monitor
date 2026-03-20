package arin

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
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

const Source = "ARIN"

const (
	DelegatedURL            = "https://ftp.arin.net/pub/stats/arin/delegated-arin-extended-latest"
	delegatedFilenamePrefix = "delegated-arin-extended-"
)

var Registry = nrtm.Registry{
	Name:      Source,
	Source:    Source,
	Host:      "rr.arin.net",
	Port:      43,
	SerialURL: "https://ftp.arin.net/pub/rr/ARIN.CURRENTSERIAL",
}

type AutNum struct {
	ASN          string
	AsName       string
	Descr        string
	MntBy        string
	Created      string
	LastModified string
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
			log.Println("[ARIN Monitor] Shutting down...")
			return
		case <-ticker.C:
			m.poll()
		}
	}
}

func (m *Monitor) initializeDelegatedData() {
	latest, previous, err := m.loadRecentDelegatedData()
	if err != nil {
		log.Printf("[ARIN Monitor] Failed to load delegated data: %v", err)
	}

	if latest == nil {
		log.Println("[ARIN Monitor] No delegated baseline found, fetching fresh...")
		fetched, err := m.fetchAndSaveDelegated()
		if err != nil {
			log.Printf("[ARIN Monitor] Failed to fetch delegated baseline: %v", err)
			return
		}

		m.setDelegatedData(fetched, nil)
		m.cleanupOldDelegatedFiles(2)
		log.Printf("[ARIN Monitor] Saved delegated baseline with %d ASNs to %s", len(fetched.ASNs), filepath.Base(fetched.FilePath))
		return
	}

	newlyAdded := diffDelegatedData(previous, latest)
	m.setDelegatedData(latest, newlyAdded)

	if previous != nil {
		log.Printf("[ARIN Monitor] Loaded delegated baseline with %d ASNs from %s (%d newly added vs previous snapshot)", len(latest.ASNs), filepath.Base(latest.FilePath), len(newlyAdded))
		return
	}

	log.Printf("[ARIN Monitor] Loaded delegated baseline with %d ASNs from %s", len(latest.ASNs), filepath.Base(latest.FilePath))
}

func (m *Monitor) initializeSerial() {
	if m.state.GetSerial(Source) == 0 {
		serial, err := m.client.CurrentSerial(context.Background())
		if err != nil {
			log.Printf("[ARIN Monitor] Failed to get current serial: %v", err)
			return
		}
		m.state.SetSerial(Source, serial)
		log.Printf("[ARIN Monitor] Initialized at serial %d", serial)
	} else {
		log.Printf("[ARIN Monitor] Resuming from serial %d", m.state.GetSerial(Source))
	}

	telegram.Status.UpdateARIN(m.state.GetSerial(Source), "")

	if err := m.state.Save(); err != nil {
		log.Printf("[ARIN Monitor] Failed to save state: %v", err)
	}
}

func (m *Monitor) scheduleDelegatedRefresh(ctx context.Context) {
	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, time.UTC)
		if !now.Before(next) {
			next = next.Add(24 * time.Hour)
		}

		waitDuration := next.Sub(now)
		log.Printf("[ARIN Monitor] Next delegated refresh at %s (in %s)", next.Format(time.RFC3339), waitDuration)

		select {
		case <-ctx.Done():
			return
		case <-time.After(waitDuration):
			m.refreshDelegatedData()
		}
	}
}

func (m *Monitor) refreshDelegatedData() {
	log.Println("[ARIN Monitor] Refreshing delegated baseline...")

	newData, err := m.fetchAndSaveDelegated()
	if err != nil {
		log.Printf("[ARIN Monitor] Failed to refresh delegated baseline: %v", err)
		return
	}

	previous := m.currentDelegatedData()
	newlyAdded := diffDelegatedData(previous, newData)
	m.setDelegatedData(newData, newlyAdded)
	m.cleanupOldDelegatedFiles(2)

	log.Printf("[ARIN Monitor] Updated delegated baseline to %s with %d ASNs (%d newly added vs previous snapshot)", filepath.Base(newData.FilePath), len(newData.ASNs), len(newlyAdded))
}

func (m *Monitor) poll() {
	fromSerial := m.state.GetSerial(Source)
	if fromSerial == 0 {
		return
	}

	updates, err := m.fetchUpdates(fromSerial + 1)
	if err != nil {
		log.Printf("[ARIN Monitor] Error fetching updates: %v", err)
		return
	}

	if len(updates) == 0 {
		return
	}

	log.Printf("[ARIN Monitor] Received %d updates", len(updates))

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

		if autNum.Created != "" && autNum.LastModified != "" && autNum.Created != autNum.LastModified {
			continue
		}
		if !m.shouldNotifyASN(autNum.ASN) {
			log.Printf("[ARIN Monitor] Skipping historical ASN already present in delegated baseline: %s", autNum.ASN)
			continue
		}

		log.Printf("[ARIN Monitor] New ASN: %s (%s)", autNum.ASN, autNum.AsName)

		tgAutNum := &telegram.AutNum{
			ASN:    autNum.ASN,
			AsName: autNum.AsName,
			Descr:  autNum.Descr,
			MntBy:  autNum.MntBy,
			Source: Source,
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
		telegram.Status.UpdateARIN(maxSerial, "")
		if err := m.state.Save(); err != nil {
			log.Printf("[ARIN Monitor] Failed to save state: %v", err)
		}
	}
}

func (m *Monitor) fetchUpdates(fromSerial int64) ([]nrtm.Update, error) {
	return m.client.Updates(context.Background(), fromSerial)
}

func (m *Monitor) parseAutNum(obj *nrtm.RPSLObject) *AutNum {
	return &AutNum{
		ASN:          obj.Attributes["aut-num"],
		AsName:       obj.Attributes["as-name"],
		Descr:        obj.Attributes["descr"],
		MntBy:        obj.Attributes["mnt-by"],
		Created:      obj.Attributes["created"],
		LastModified: obj.Attributes["last-modified"],
	}
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
		if parts[0] != "arin" || parts[2] != "asn" {
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
			log.Printf("[ARIN Monitor] Failed to remove old delegated snapshot %s: %v", filepath.Base(filePath), err)
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
