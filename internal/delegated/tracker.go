package delegated

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Snapshot struct {
	ASNs     map[string]struct{}
	Metadata map[string]ASNMetadata
	FilePath string
}

type ASNMetadata struct {
	Country string
	Date    string
}

type Config struct {
	URL                 string
	FilePrefix          string
	AllowedStatsSources []string
	AllowedStatuses     []string
}

type Tracker struct {
	dataDir             string
	url                 string
	filePrefix          string
	allowedStatsSources map[string]struct{}
	allowedStatuses     map[string]struct{}
	mu                  sync.RWMutex
	current             *Snapshot
	newlyAdded          map[string]struct{}
	lastDiffCount       int
}

func NewTracker(dataDir string, cfg Config) *Tracker {
	allowed := make(map[string]struct{}, len(cfg.AllowedStatsSources))
	for _, source := range cfg.AllowedStatsSources {
		allowed[source] = struct{}{}
	}

	statuses := cfg.AllowedStatuses
	if len(statuses) == 0 {
		statuses = []string{"allocated", "assigned"}
	}
	allowedStatuses := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		allowedStatuses[status] = struct{}{}
	}

	return &Tracker{
		dataDir:             dataDir,
		url:                 cfg.URL,
		filePrefix:          cfg.FilePrefix,
		allowedStatsSources: allowed,
		allowedStatuses:     allowedStatuses,
		newlyAdded:          make(map[string]struct{}),
	}
}

func (t *Tracker) Initialize() (*Snapshot, int, error) {
	latest, previous, err := t.loadRecentSnapshots()
	if err != nil {
		return nil, 0, err
	}

	if latest == nil {
		fetched, err := t.fetchAndSave()
		if err != nil {
			return nil, 0, err
		}

		t.setSnapshot(fetched, nil)
		t.cleanupOldFiles(2)
		return fetched, 0, nil
	}

	newlyAdded := Diff(previous, latest)
	t.setSnapshot(latest, newlyAdded)
	return latest, len(newlyAdded), nil
}

func (t *Tracker) Refresh() (*Snapshot, int, error) {
	newData, err := t.fetchAndSave()
	if err != nil {
		return nil, 0, err
	}

	previous := t.currentSnapshot()
	newlyAdded := Diff(previous, newData)
	t.setSnapshot(newData, newlyAdded)
	t.cleanupOldFiles(2)

	return newData, len(newlyAdded), nil
}

func (t *Tracker) ShouldNotifyASN(asn string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if _, ok := t.newlyAdded[asn]; ok {
		return true
	}
	if t.current == nil {
		return true
	}
	if _, exists := t.current.ASNs[asn]; exists {
		return false
	}
	return true
}

func (t *Tracker) Status() (filePath string, diffCount int) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.current != nil {
		filePath = t.current.FilePath
	}

	return filePath, t.lastDiffCount
}

func (t *Tracker) NewlyAddedASNs() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	asns := make([]string, 0, len(t.newlyAdded))
	for asn := range t.newlyAdded {
		asns = append(asns, asn)
	}
	sort.Strings(asns)
	return asns
}

func Diff(previous, current *Snapshot) map[string]struct{} {
	newlyAdded := make(map[string]struct{})
	if previous == nil || current == nil {
		return newlyAdded
	}

	for asn := range current.ASNs {
		if _, exists := previous.ASNs[asn]; !exists {
			newlyAdded[asn] = struct{}{}
		}
	}

	return newlyAdded
}

func (t *Tracker) fetchAndSave() (*Snapshot, error) {
	client := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequest(http.MethodGet, t.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "irr-monitor/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(t.dataDir, 0755); err != nil {
		return nil, err
	}

	filename := fmt.Sprintf("%s%s", t.filePrefix, time.Now().UTC().Format("20060102"))
	filePath := filepath.Join(t.dataDir, filename)
	if err := os.WriteFile(filePath, body, 0644); err != nil {
		return nil, err
	}

	data := t.parseData(bytes.NewReader(body))
	data.FilePath = filePath

	return data, nil
}

func (t *Tracker) loadRecentSnapshots() (latest *Snapshot, previous *Snapshot, err error) {
	files, err := filepath.Glob(filepath.Join(t.dataDir, t.filePrefix+"*"))
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, nil
	}

	sort.Strings(files)

	latest, err = t.loadFile(files[len(files)-1])
	if err != nil {
		return nil, nil, err
	}

	if len(files) > 1 {
		previous, err = t.loadFile(files[len(files)-2])
		if err != nil {
			return nil, nil, err
		}
	}

	return latest, previous, nil
}

func (t *Tracker) loadFile(filePath string) (*Snapshot, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data := t.parseData(f)
	data.FilePath = filePath

	return data, nil
}

func (t *Tracker) parseData(r io.Reader) *Snapshot {
	data := &Snapshot{
		ASNs:     make(map[string]struct{}),
		Metadata: make(map[string]ASNMetadata),
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 7 {
			continue
		}
		if _, ok := t.allowedStatsSources[parts[0]]; !ok {
			continue
		}
		if parts[2] != "asn" {
			continue
		}
		if _, ok := t.allowedStatuses[parts[6]]; !ok {
			continue
		}

		start, err := strconv.ParseInt(parts[3], 10, 64)
		if err != nil {
			continue
		}
		count, err := strconv.ParseInt(parts[4], 10, 64)
		if err != nil || count <= 0 {
			continue
		}

		for i := int64(0); i < count; i++ {
			asn := fmt.Sprintf("AS%d", start+i)
			data.ASNs[asn] = struct{}{}
			data.Metadata[asn] = ASNMetadata{
				Country: parts[1],
				Date:    parts[5],
			}
		}
	}

	return data
}

func (t *Tracker) cleanupOldFiles(keep int) {
	files, _ := filepath.Glob(filepath.Join(t.dataDir, t.filePrefix+"*"))
	if len(files) <= keep {
		return
	}

	sort.Strings(files)
	for _, filePath := range files[:len(files)-keep] {
		_ = os.Remove(filePath)
	}
}

func (t *Tracker) setSnapshot(data *Snapshot, newlyAdded map[string]struct{}) {
	if newlyAdded == nil {
		newlyAdded = make(map[string]struct{})
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.current = data
	t.newlyAdded = newlyAdded
	t.lastDiffCount = len(newlyAdded)
}

func (t *Tracker) currentSnapshot() *Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.current
}
