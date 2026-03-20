package preferences

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestJSONStoreSetAndLoad(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "preferences.json")
	store := NewJSONStore(path)

	err := store.Set(1001, UserPreferences{
		Enabled:        true,
		ASNSizes:       []string{"4b", "2b", "4b"},
		RIRs:           []string{"ARIN", "APNIC"},
		NIRs:           []string{"JPNIC"},
		SponsoringOrgs: []string{" Example Sponsor ", "example sponsor"},
	})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	reloaded := NewJSONStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	got, ok := reloaded.Get(1001)
	if !ok {
		t.Fatalf("expected preferences to exist")
	}

	want := UserPreferences{
		Enabled:        true,
		ASNSizes:       []string{"2b", "4b"},
		RIRs:           []string{"APNIC", "ARIN"},
		NIRs:           []string{"JPNIC"},
		SponsoringOrgs: []string{"example sponsor"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Get() = %#v, want %#v", got, want)
	}
}

func TestJSONStoreLoadMissingOptionalFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "preferences.json")
	payload := map[string]any{
		"version": 1,
		"users": map[string]any{
			"42": map[string]any{
				"enabled": true,
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	store := NewJSONStore(path)
	if err := store.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	got, ok := store.Get(42)
	if !ok {
		t.Fatalf("expected preferences to exist")
	}

	want := UserPreferences{Enabled: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Get() = %#v, want %#v", got, want)
	}
}
