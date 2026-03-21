package arin

import (
	"context"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/realSunyz/irr-monitor/internal/delegated"
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

type Monitor struct {
	state        *state.State
	pollInterval time.Duration
	callback     func(source string, autNum *telegram.AutNum)
	client       *nrtm.Client
	delegated    *delegated.Tracker
}

func NewMonitor(st *state.State, dataDir string, pollInterval time.Duration, callback func(string, *telegram.AutNum)) *Monitor {
	timeout := 30 * time.Second
	return &Monitor{
		state:        st,
		pollInterval: pollInterval,
		callback:     callback,
		client:       nrtm.NewClient(Registry, timeout),
		delegated: delegated.NewTracker(dataDir, delegated.Config{
			URL:                 DelegatedURL,
			FilePrefix:          delegatedFilenamePrefix,
			AllowedStatsSources: []string{"arin"},
		}),
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
	latest, diffCount, err := m.delegated.Initialize()
	if err != nil {
		log.Printf("[ARIN Monitor] Failed to load delegated data: %v", err)
	}

	if latest == nil {
		log.Println("[ARIN Monitor] Delegated baseline unavailable; proceeding without delegated snapshot")
		return
	}

	telegram.Status.UpdateARINDelegated(filepath.Base(latest.FilePath), diffCount)

	if diffCount > 0 {
		log.Printf("[ARIN Monitor] Loaded delegated baseline with %d ASNs from %s (%d newly added vs previous snapshot)", len(latest.ASNs), filepath.Base(latest.FilePath), diffCount)
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

	newData, diffCount, err := m.delegated.Refresh()
	if err != nil {
		log.Printf("[ARIN Monitor] Failed to refresh delegated baseline: %v", err)
		return
	}

	telegram.Status.UpdateARINDelegated(filepath.Base(newData.FilePath), diffCount)

	log.Printf("[ARIN Monitor] Updated delegated baseline to %s with %d ASNs (%d newly added vs previous snapshot)", filepath.Base(newData.FilePath), len(newData.ASNs), diffCount)
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
		if !m.delegated.ShouldNotifyASN(autNum.ASN) {
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
