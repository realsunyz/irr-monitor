package ripe

import (
	"strings"
	"testing"
)

func TestParseDelegatedDataIncludesAssignedAndRanges(t *testing.T) {
	t.Parallel()

	monitor := &Monitor{}
	data := monitor.parseDelegatedData(strings.NewReader(`
ripencc|*|asn|*|0|summary
ripencc|NL|asn|64512|2|20260320|allocated
ripe|DE|asn|65000|1|20260320|assigned
ripencc|FR|asn|65100|1|20260320|available
arin|US|asn|65200|1|20260320|allocated
`))

	for _, asn := range []string{"AS64512", "AS64513", "AS65000"} {
		if _, ok := data.ASNs[asn]; !ok {
			t.Fatalf("expected %s in delegated data", asn)
		}
	}

	for _, asn := range []string{"AS65100", "AS65200"} {
		if _, ok := data.ASNs[asn]; ok {
			t.Fatalf("did not expect %s in delegated data", asn)
		}
	}
}

func TestShouldNotifyASNSuppressesHistoricalEntries(t *testing.T) {
	t.Parallel()

	monitor := &Monitor{}
	monitor.setDelegatedData(&DelegatedData{
		ASNs: map[string]struct{}{
			"AS64512": {},
			"AS64513": {},
			"AS64514": {},
		},
	}, map[string]struct{}{
		"AS64514": {},
	})

	if monitor.shouldNotifyASN("AS64512") {
		t.Fatalf("historical ASN should be suppressed")
	}
	if !monitor.shouldNotifyASN("AS64514") {
		t.Fatalf("newly delegated ASN should still notify")
	}
	if !monitor.shouldNotifyASN("AS65000") {
		t.Fatalf("unknown ASN should still notify")
	}
}

func TestDiffDelegatedDataReturnsOnlyNewASNs(t *testing.T) {
	t.Parallel()

	previous := &DelegatedData{
		ASNs: map[string]struct{}{
			"AS64512": {},
			"AS64513": {},
		},
	}
	current := &DelegatedData{
		ASNs: map[string]struct{}{
			"AS64512": {},
			"AS64513": {},
			"AS64514": {},
		},
	}

	diff := diffDelegatedData(previous, current)
	if len(diff) != 1 {
		t.Fatalf("diff size = %d, want 1", len(diff))
	}
	if _, ok := diff["AS64514"]; !ok {
		t.Fatalf("expected AS64514 in diff")
	}
}
