package arin

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/realSunyz/irr-monitor/internal/delegated"
	"github.com/realSunyz/irr-monitor/internal/telegram"
)

const Source = "ARIN"

const (
	DelegatedURL            = "https://ftp.arin.net/pub/stats/arin/delegated-arin-extended-latest"
	delegatedFilenamePrefix = "delegated-arin-extended-"
)

type Monitor struct {
	callback  func(source string, autNum *telegram.AutNum)
	delegated *delegated.Tracker
}

func NewMonitor(dataDir string, callback func(string, *telegram.AutNum)) *Monitor {
	return &Monitor{
		callback: callback,
		delegated: delegated.NewTracker(dataDir, delegated.Config{
			URL:                 DelegatedURL,
			FilePrefix:          delegatedFilenamePrefix,
			AllowedStatsSources: []string{"arin"},
			AllowedStatuses:     []string{"assigned"},
		}),
	}
}

func (m *Monitor) Start(ctx context.Context) {
	m.initializeDelegatedData()
	log.Println("[ARIN Monitor] Monitoring delegated daily diffs only")

	go m.scheduleDelegatedRefresh(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Println("[ARIN Monitor] Shutting down...")
			return
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
	telegram.Status.UpdateARIN(0, "")

	if diffCount > 0 {
		log.Printf("[ARIN Monitor] Loaded delegated baseline with %d ASNs from %s (%d newly added vs previous snapshot)", len(latest.ASNs), filepath.Base(latest.FilePath), diffCount)
		return
	}

	log.Printf("[ARIN Monitor] Loaded delegated baseline with %d ASNs from %s", len(latest.ASNs), filepath.Base(latest.FilePath))
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
	newASNs := m.delegated.NewlyAddedASNs()

	log.Printf("[ARIN Monitor] Updated delegated baseline to %s with %d ASNs (%d newly added vs previous snapshot)", filepath.Base(newData.FilePath), len(newData.ASNs), diffCount)

	var lastASN string
	for _, asn := range newASNs {
		lastASN = asn
		log.Printf("[ARIN Monitor] New ASN from delegated diff: %s", asn)
		if m.callback == nil {
			continue
		}
		m.callback(Source, &telegram.AutNum{
			ASN:    asn,
			Source: Source,
		})
	}

	telegram.Status.UpdateARIN(0, lastASN)
}
