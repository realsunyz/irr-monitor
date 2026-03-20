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
	RIPEFilePath   string
	RIPEDiffCount  int
	ARINSerial     int64
	ARINLastCheck  time.Time
	ARINLastASN    string
	ARINFilePath   string
	ARINDiffCount  int
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

func (s *SyncStatus) UpdateRIPEDelegated(filePath string, diffCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RIPEFilePath = filePath
	s.RIPEDiffCount = diffCount
}

func (s *SyncStatus) UpdateARIN(serial int64, lastASN string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ARINSerial = serial
	s.ARINLastCheck = time.Now()
	if lastASN != "" {
		s.ARINLastASN = lastASN
	}
}

func (s *SyncStatus) UpdateARINDelegated(filePath string, diffCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ARINFilePath = filePath
	s.ARINDiffCount = diffCount
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

func (s *SyncStatus) GetStatus() (
	ripeSerial int64,
	ripeLastCheck time.Time,
	ripeLastASN string,
	ripeFile string,
	ripeDiff int,
	arinSerial int64,
	arinLastCheck time.Time,
	arinLastASN string,
	arinFile string,
	arinDiff int,
	apnicCount int,
	apnicLastCheck time.Time,
	apnicFile string,
	apnicLastASN string,
) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.RIPESerial, s.RIPELastCheck, s.RIPELastASN, s.RIPEFilePath, s.RIPEDiffCount,
		s.ARINSerial, s.ARINLastCheck, s.ARINLastASN, s.ARINFilePath, s.ARINDiffCount,
		s.APNICASNCount, s.APNICLastCheck, s.APNICFilePath, s.APNICLastASN
}
