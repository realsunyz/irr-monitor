package apnic

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"github.com/realSunyz/irr-monitor/pkg/nrtm"
	"github.com/realSunyz/irr-monitor/pkg/telegram"
)

type ASNCallback func(source string, autNum *nrtm.AutNum)

type Monitor struct {
	previousData *DelegatedData
	callback     ASNCallback
	dataDir      string
}

func NewMonitor(dataDir string, callback ASNCallback) *Monitor {
	return &Monitor{
		callback: callback,
		dataDir:  dataDir,
	}
}

func (m *Monitor) Start(ctx context.Context) {
	data, err := LoadLatestDelegatedData(m.dataDir)
	if err != nil {
		log.Printf("APNIC Monitor: Failed to load existing data: %v", err)
	}

	if data != nil {
		m.previousData = data
		log.Printf("APNIC Monitor: Loaded %d ASNs from %s", len(data.ASNs), data.FilePath)
		telegram.Status.UpdateAPNIC(len(data.ASNs), filepath.Base(data.FilePath), "")
	} else {
		log.Println("APNIC Monitor: No existing data, fetching fresh...")
		newData, err := FetchAndSaveDelegatedData(m.dataDir)
		if err != nil {
			log.Printf("APNIC Monitor: Failed to fetch initial data: %v", err)
		} else {
			m.previousData = newData
			log.Printf("APNIC Monitor: Saved %d ASNs to %s", len(newData.ASNs), newData.FilePath)
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
		log.Printf("APNIC Monitor: Next check scheduled at %s (in %s)", next.Format(time.RFC3339), waitDuration)

		select {
		case <-ctx.Done():
			log.Println("APNIC Monitor: Shutting down...")
			return
		case <-time.After(waitDuration):
			m.checkForNewASNs()
		}
	}
}

func (m *Monitor) checkForNewASNs() {
	log.Println("APNIC Monitor: Fetching delegated data...")
	newData, err := FetchAndSaveDelegatedData(m.dataDir)
	if err != nil {
		log.Printf("APNIC Monitor: Failed to fetch data: %v", err)
		return
	}

	log.Printf("APNIC Monitor: Saved to %s", newData.FilePath)

	CleanupOldFiles(m.dataDir, 2)

	if m.previousData == nil {
		m.previousData = newData
		log.Printf("APNIC Monitor: Stored %d ASNs (first run)", len(newData.ASNs))
		return
	}

	newASNs := CompareData(m.previousData, newData)
	log.Printf("APNIC Monitor: Found %d new ASNs", len(newASNs))

	for _, entry := range newASNs {
		info, err := QueryASNInfo(entry.ASN)
		if err != nil {
			log.Printf("APNIC Monitor: Failed to query %s: %v", entry.ASN, err)
			info = &ASNInfo{
				ASN:     entry.ASN,
				Country: entry.Country,
			}
		}

		autNum := &nrtm.AutNum{
			ASN:     info.ASN,
			AsName:  info.AsName,
			Descr:   info.Descr,
			Country: info.Country,
			Source:  "APNIC",
		}

		log.Printf("APNIC Monitor: New ASN: %s (%s)", autNum.ASN, autNum.AsName)

		if m.callback != nil {
			m.callback("APNIC", autNum)
		}
	}

	m.previousData = newData
}

func (m *Monitor) CheckNow() {
	m.checkForNewASNs()
}
