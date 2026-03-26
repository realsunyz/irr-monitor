package arin

import (
	"bufio"
	"context"
	"io"
	"log"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/realSunyz/irr-monitor/internal/delegated"
	"github.com/realSunyz/irr-monitor/internal/telegram"
)

const Source = "ARIN"

const (
	DelegatedURL            = "https://ftp.arin.net/pub/stats/arin/delegated-arin-extended-latest"
	delegatedFilenamePrefix = "delegated-arin-extended-"
	whoisAddr               = "whois.arin.net:43"
)

type Monitor struct {
	callback  func(source string, autNum *telegram.AutNum)
	delegated *delegated.Tracker
	lookup    func(asn string, metadata delegated.ASNMetadata) *telegram.AutNum
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
	<-ctx.Done()
	log.Println("[ARIN Monitor] Shutting down...")
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

	now := time.Now().UTC()
	allowedDates := map[string]struct{}{
		now.Format("20060102"):                      {},
		now.Add(-24 * time.Hour).Format("20060102"): {},
	}
	lastASN := m.notifyNewDelegatedASNs(newData, newASNs, allowedDates)

	telegram.Status.UpdateARIN(0, lastASN)
}

func (m *Monitor) notifyNewDelegatedASNs(snapshot *delegated.Snapshot, asns []string, allowedDates map[string]struct{}) string {
	var lastASN string

	for _, asn := range asns {
		metadata, ok := snapshot.Metadata[asn]
		if !ok {
			log.Printf("[ARIN Monitor] Skipping delegated diff ASN with missing metadata: %s", asn)
			continue
		}
		if _, ok := allowedDates[metadata.Date]; !ok {
			log.Printf("[ARIN Monitor] Skipping delegated diff ASN with stale assignment date: %s (date=%q)", asn, metadata.Date)
			continue
		}

		lastASN = asn
		log.Printf("[ARIN Monitor] New ASN from delegated diff: %s", asn)
		if m.callback == nil {
			continue
		}
		info := m.lookupASNInfo(asn, metadata)
		if info == nil {
			info = &telegram.AutNum{
				ASN:     asn,
				Country: metadata.Country,
				Source:  Source,
			}
		}
		m.callback(Source, info)
	}

	return lastASN
}

func (m *Monitor) lookupASNInfo(asn string, metadata delegated.ASNMetadata) *telegram.AutNum {
	if m.lookup != nil {
		return m.lookup(asn, metadata)
	}
	return m.queryASNInfo(asn, metadata)
}

func (m *Monitor) queryASNInfo(asn string, metadata delegated.ASNMetadata) *telegram.AutNum {
	conn, err := net.DialTimeout("tcp", whoisAddr, 10*time.Second)
	if err != nil {
		log.Printf("[ARIN Monitor] WHOIS lookup failed for %s: %v", asn, err)
		return nil
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return nil
	}
	if _, err := conn.Write([]byte(asn + "\n")); err != nil {
		log.Printf("[ARIN Monitor] WHOIS write failed for %s: %v", asn, err)
		return nil
	}

	return parseWhoisAutnum(conn, asn, metadata.Country)
}

func parseWhoisAutnum(r io.Reader, asn, fallbackCountry string) *telegram.AutNum {
	info := &telegram.AutNum{
		ASN:     asn,
		Country: fallbackCountry,
		Source:  Source,
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		switch {
		case strings.HasPrefix(line, "ASName:") && info.AsName == "":
			info.AsName = strings.TrimSpace(strings.TrimPrefix(line, "ASName:"))
		case strings.HasPrefix(line, "OrgId:") && info.Org == "":
			info.Org = strings.TrimSpace(strings.TrimPrefix(line, "OrgId:"))
		case strings.HasPrefix(line, "OrgName:") && info.OrgName == "":
			info.OrgName = strings.TrimSpace(strings.TrimPrefix(line, "OrgName:"))
		case strings.HasPrefix(line, "Country:") && info.Country == fallbackCountry:
			info.Country = strings.TrimSpace(strings.TrimPrefix(line, "Country:"))
		}
	}

	return info
}
