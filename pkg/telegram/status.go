package telegram

import (
	"sync"
	"time"
)

type SyncStatus struct {
	mu             sync.RWMutex
	RIPESerial     int64
	RIPELastCheck  time.Time
	RIPELastASN    string
	APNICASNCount  int
	APNICLastCheck time.Time
	APNICLastASN   string
	APNICFilePath  string
}

var Status = &SyncStatus{}

func (s *SyncStatus) UpdateRIPE(serial int64, lastASN string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RIPESerial = serial
	s.RIPELastCheck = time.Now()
	if lastASN != "" {
		s.RIPELastASN = lastASN
	}
}

func (s *SyncStatus) UpdateAPNIC(asnCount int, filePath, lastASN string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.APNICASNCount = asnCount
	s.APNICLastCheck = time.Now()
	s.APNICFilePath = filePath
	if lastASN != "" {
		s.APNICLastASN = lastASN
	}
}

func (s *SyncStatus) GetStatus() (ripeSerial int64, ripeLastCheck time.Time, ripeLastASN string,
	apnicCount int, apnicLastCheck time.Time, apnicFile, apnicLastASN string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.RIPESerial, s.RIPELastCheck, s.RIPELastASN,
		s.APNICASNCount, s.APNICLastCheck, s.APNICFilePath, s.APNICLastASN
}
