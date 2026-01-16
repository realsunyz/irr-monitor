package monitor

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/realSunyz/irr-monitor/pkg/nrtm"
)

type ASNCallback func(source string, autNum *nrtm.AutNum)

type ASNMonitor struct {
	clients      map[string]*nrtm.Client
	state        *State
	pollInterval time.Duration
	callback     ASNCallback
	mu           sync.RWMutex
}

func NewASNMonitor(rirs []string, state *State, pollInterval time.Duration, callback ASNCallback) *ASNMonitor {
	configs := nrtm.DefaultRIRConfigs()
	clients := make(map[string]*nrtm.Client)

	for _, rir := range rirs {
		if config, ok := configs[rir]; ok {
			clients[rir] = nrtm.NewClient(config)
			log.Printf("Configured monitor for %s (%s:%d)", rir, config.Host, config.Port)
		} else {
			log.Printf("Warning: Unknown RIR '%s', skipping", rir)
		}
	}

	return &ASNMonitor{
		clients:      clients,
		state:        state,
		pollInterval: pollInterval,
		callback:     callback,
	}
}

func (m *ASNMonitor) Start(ctx context.Context) {
	m.initializeSerials()

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	m.poll()

	for {
		select {
		case <-ctx.Done():
			log.Println("ASN Monitor shutting down...")
			return
		case <-ticker.C:
			m.poll()
		}
	}
}

func (m *ASNMonitor) initializeSerials() {
	for rir, client := range m.clients {
		if m.state.GetSerial(rir) == 0 {
			serial, err := client.GetCurrentSerial()
			if err != nil {
				log.Printf("Warning: Failed to get current serial for %s: %v", rir, err)
				continue
			}
			m.state.SetSerial(rir, serial)
			log.Printf("Initialized %s at serial %d", rir, serial)
		} else {
			log.Printf("Resuming %s from serial %d", rir, m.state.GetSerial(rir))
		}
	}

	if err := m.state.Save(); err != nil {
		log.Printf("Warning: Failed to save state: %v", err)
	}
}

func (m *ASNMonitor) poll() {
	var wg sync.WaitGroup

	for rir, client := range m.clients {
		wg.Add(1)
		go func(rir string, client *nrtm.Client) {
			defer wg.Done()
			m.pollRIR(rir, client)
		}(rir, client)
	}

	wg.Wait()
}

func (m *ASNMonitor) pollRIR(rir string, client *nrtm.Client) {
	fromSerial := m.state.GetSerial(rir)
	if fromSerial == 0 {
		log.Printf("No serial for %s, skipping poll", rir)
		return
	}

	fromSerial++

	updates, err := client.FetchUpdates(fromSerial)
	if err != nil {
		log.Printf("Error fetching updates from %s: %v", rir, err)
		return
	}

	if len(updates) == 0 {
		return
	}

	log.Printf("Received %d updates from %s", len(updates), rir)

	var maxSerial int64
	newASNCount := 0

	for _, update := range updates {
		if update.Serial > maxSerial {
			maxSerial = update.Serial
		}

		if update.Operation != "ADD" {
			continue
		}

		if update.Object == nil || !update.Object.IsAutNum() {
			continue
		}

		autNum := update.Object.ToAutNum()
		if autNum == nil {
			continue
		}

		newASNCount++
		log.Printf("New ASN allocation from %s: %s (%s)", rir, autNum.ASN, autNum.AsName)

		if m.callback != nil {
			m.callback(rir, autNum)
		}
	}

	if maxSerial > 0 {
		m.state.UpdateSerial(rir, maxSerial)
		if err := m.state.Save(); err != nil {
			log.Printf("Warning: Failed to save state: %v", err)
		}
	}

	if newASNCount > 0 {
		log.Printf("Found %d new ASN allocations from %s", newASNCount, rir)
	}
}

func (m *ASNMonitor) GetClients() map[string]*nrtm.Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clients
}
