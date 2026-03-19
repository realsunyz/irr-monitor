package arin

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/realSunyz/irr-monitor/internal/nrtm"
	"github.com/realSunyz/irr-monitor/internal/state"
	"github.com/realSunyz/irr-monitor/internal/telegram"
)

const Source = "ARIN"

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
}

func NewMonitor(st *state.State, pollInterval time.Duration, callback func(string, *telegram.AutNum)) *Monitor {
	timeout := 30 * time.Second
	return &Monitor{
		state:        st,
		pollInterval: pollInterval,
		callback:     callback,
		client:       nrtm.NewClient(Registry, timeout),
	}
}

func (m *Monitor) Start(ctx context.Context) {
	m.initializeSerial()

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
