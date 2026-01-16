package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type State struct {
	Serials  map[string]int64 `json:"serials"`
	filepath string
	mu       sync.RWMutex
}

func New(filepath string) *State {
	return &State{
		Serials:  make(map[string]int64),
		filepath: filepath,
	}
}

func (s *State) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.filepath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := os.ReadFile(s.filepath)
	if err != nil {
		if os.IsNotExist(err) {
			s.Serials = make(map[string]int64)
			return nil
		}
		return err
	}

	return json.Unmarshal(data, &s.Serials)
}

func (s *State) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := json.MarshalIndent(s.Serials, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := s.filepath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, s.filepath)
}

func (s *State) GetSerial(source string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Serials[source]
}

func (s *State) SetSerial(source string, serial int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Serials[source] = serial
}

func (s *State) UpdateSerial(source string, serial int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if serial > s.Serials[source] {
		s.Serials[source] = serial
		return true
	}
	return false
}
