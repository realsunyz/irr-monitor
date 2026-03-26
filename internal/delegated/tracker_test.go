package delegated

import (
	"strings"
	"testing"
)

func TestParseDataRespectsAllowedStatusesAndRanges(t *testing.T) {
	t.Parallel()

	tracker := NewTracker("", Config{
		AllowedStatsSources: []string{"arin", "ripencc"},
		AllowedStatuses:     []string{"assigned"},
	})
	data := tracker.parseData(strings.NewReader(`
arin|*|asn|*|0|summary
arin|US|asn|65000|2|20260320|allocated
ripencc|NL|asn|65010|1|20260320|assigned
arin|US|asn|65020|1|20260320|available
arin||asn|65021|1||reserved|
apnic|US|asn|65100|1|20260320|allocated
`))

	for _, asn := range []string{"AS65010"} {
		if _, ok := data.ASNs[asn]; !ok {
			t.Fatalf("expected %s in delegated data", asn)
		}
	}
	if got := data.Metadata["AS65010"].Date; got != "20260320" {
		t.Fatalf("AS65010 date = %q, want 20260320", got)
	}

	for _, asn := range []string{"AS65000", "AS65001", "AS65020", "AS65021", "AS65100"} {
		if _, ok := data.ASNs[asn]; ok {
			t.Fatalf("did not expect %s in delegated data", asn)
		}
	}
}

func TestShouldNotifyASNSuppressesHistoricalEntries(t *testing.T) {
	t.Parallel()

	tracker := NewTracker("", Config{})
	tracker.setSnapshot(&Snapshot{
		ASNs: map[string]struct{}{
			"AS65000": {},
			"AS65001": {},
			"AS65002": {},
		},
	}, map[string]struct{}{
		"AS65002": {},
	})

	if tracker.ShouldNotifyASN("AS65000") {
		t.Fatalf("historical ASN should be suppressed")
	}
	if !tracker.ShouldNotifyASN("AS65002") {
		t.Fatalf("newly delegated ASN should still notify")
	}
	if !tracker.ShouldNotifyASN("AS65100") {
		t.Fatalf("unknown ASN should still notify")
	}
}

func TestDiffReturnsOnlyNewASNs(t *testing.T) {
	t.Parallel()

	previous := &Snapshot{
		ASNs: map[string]struct{}{
			"AS65000": {},
			"AS65001": {},
		},
	}
	current := &Snapshot{
		ASNs: map[string]struct{}{
			"AS65000": {},
			"AS65001": {},
			"AS65002": {},
		},
	}

	diff := Diff(previous, current)
	if len(diff) != 1 {
		t.Fatalf("diff size = %d, want 1", len(diff))
	}
	if _, ok := diff["AS65002"]; !ok {
		t.Fatalf("expected AS65002 in diff")
	}
}

func TestStatusReturnsCurrentFileAndDiffCount(t *testing.T) {
	t.Parallel()

	tracker := NewTracker("", Config{})
	tracker.setSnapshot(&Snapshot{FilePath: "delegated-20260320"}, map[string]struct{}{
		"AS65000": {},
		"AS65001": {},
	})

	filePath, diffCount := tracker.Status()
	if filePath != "delegated-20260320" {
		t.Fatalf("filePath = %q, want delegated-20260320", filePath)
	}
	if diffCount != 2 {
		t.Fatalf("diffCount = %d, want 2", diffCount)
	}
}
