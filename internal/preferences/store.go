package preferences

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const currentVersion = 1

var (
	allASNSizes = []string{"2b", "4b"}
	allRIRs     = []string{"APNIC", "ARIN", "RIPE"}
	allNIRs     = []string{"CNNIC", "IDNIC", "IRINN", "JPNIC", "KRNIC", "TWNIC"}
)

type Store interface {
	Load() error
	Get(userID int64) (UserPreferences, bool)
	Set(userID int64, prefs UserPreferences) error
	Update(userID int64, fn func(*UserPreferences) error) (UserPreferences, error)
	List() []UserRecord
}

type UserPreferences struct {
	Enabled        bool     `json:"enabled,omitempty"`
	ASNSizes       []string `json:"asn_sizes,omitempty"`
	RIRs           []string `json:"rirs,omitempty"`
	NIRs           []string `json:"nirs,omitempty"`
	SponsoringOrgs []string `json:"sponsoring_orgs,omitempty"`
}

type UserRecord struct {
	UserID      int64
	Preferences UserPreferences
}

type JSONStore struct {
	path string
	mu   sync.RWMutex
	data jsonData
}

type jsonData struct {
	Version int                        `json:"version"`
	Users   map[string]UserPreferences `json:"users"`
}

func NewJSONStore(path string) *JSONStore {
	return &JSONStore{
		path: path,
		data: jsonData{
			Version: currentVersion,
			Users:   make(map[string]UserPreferences),
		},
	}
}

func (s *JSONStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			s.data = jsonData{
				Version: currentVersion,
				Users:   make(map[string]UserPreferences),
			}
			return nil
		}
		return err
	}

	decoded := jsonData{
		Version: currentVersion,
		Users:   make(map[string]UserPreferences),
	}

	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		s.data = decoded
		return nil
	}

	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return err
	}

	if decoded.Version == 0 {
		decoded.Version = currentVersion
	}
	if decoded.Users == nil {
		decoded.Users = make(map[string]UserPreferences)
	}

	for userID, prefs := range decoded.Users {
		prefs.Normalize()
		decoded.Users[userID] = prefs
	}

	s.data = decoded
	return nil
}

func (s *JSONStore) Get(userID int64) (UserPreferences, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefs, ok := s.data.Users[strconv.FormatInt(userID, 10)]
	if !ok {
		return UserPreferences{}, false
	}
	prefs.Normalize()
	return prefs, true
}

func (s *JSONStore) Set(userID int64, prefs UserPreferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prefs.Normalize()
	s.data.Version = currentVersion
	if s.data.Users == nil {
		s.data.Users = make(map[string]UserPreferences)
	}
	s.data.Users[strconv.FormatInt(userID, 10)] = prefs
	return s.saveLocked()
}

func (s *JSONStore) Update(userID int64, fn func(*UserPreferences) error) (UserPreferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data.Version = currentVersion
	if s.data.Users == nil {
		s.data.Users = make(map[string]UserPreferences)
	}

	key := strconv.FormatInt(userID, 10)
	prefs := s.data.Users[key]
	prefs.Normalize()

	if err := fn(&prefs); err != nil {
		return UserPreferences{}, err
	}

	prefs.Normalize()
	s.data.Users[key] = prefs
	if err := s.saveLocked(); err != nil {
		return UserPreferences{}, err
	}

	return prefs, nil
}

func (s *JSONStore) List() []UserRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records := make([]UserRecord, 0, len(s.data.Users))
	for rawID, prefs := range s.data.Users {
		userID, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil {
			continue
		}
		prefs.Normalize()
		records = append(records, UserRecord{
			UserID:      userID,
			Preferences: prefs,
		})
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].UserID < records[j].UserID
	})

	return records
}

func (s *JSONStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := s.path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, s.path)
}

func (p *UserPreferences) Normalize() {
	p.ASNSizes = normalizeValues(p.ASNSizes, false)
	p.RIRs = normalizeValues(p.RIRs, false)
	p.NIRs = normalizeValues(p.NIRs, false)
	p.SponsoringOrgs = NormalizeSponsoringOrgs(p.SponsoringOrgs)

	if sameValues(p.ASNSizes, allASNSizes) {
		p.ASNSizes = nil
	}
	if sameValues(p.RIRs, allRIRs) && sameValues(p.NIRs, allNIRs) {
		p.RIRs = nil
		p.NIRs = nil
	}
}

func NormalizeSponsoringOrg(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func NormalizeSponsoringOrgs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return normalizeValues(values, true)
}

func normalizeValues(values []string, lowercase bool) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)
		if lowercase {
			value = strings.ToLower(value)
		}
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	if len(normalized) == 0 {
		return nil
	}

	sort.Strings(normalized)
	return normalized
}

func sameValues(values, target []string) bool {
	if len(values) != len(target) {
		return false
	}
	for i := range values {
		if values[i] != target[i] {
			return false
		}
	}
	return true
}
