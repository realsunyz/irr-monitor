package arin

import (
	"strings"
	"testing"
)

func TestParseDelegatedDataIncludesAssignedAndRanges(t *testing.T) {
	t.Parallel()

	monitor := &Monitor{}
	data := monitor.parseDelegatedData(strings.NewReader(`
arin|*|asn|*|0|summary
arin|US|asn|65000|2|20260320|allocated
arin|CA|asn|65010|1|20260320|assigned
arin|US|asn|65020|1|20260320|available
apnic|US|asn|65100|1|20260320|allocated
`))

	for _, asn := range []string{"AS65000", "AS65001", "AS65010"} {
		if _, ok := data.ASNs[asn]; !ok {
			t.Fatalf("expected %s in delegated data", asn)
		}
	}

	for _, asn := range []string{"AS65020", "AS65100"} {
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
			"AS65000": {},
			"AS65001": {},
			"AS65002": {},
		},
	}, map[string]struct{}{
		"AS65002": {},
	})

	if monitor.shouldNotifyASN("AS65000") {
		t.Fatalf("historical ASN should be suppressed")
	}
	if !monitor.shouldNotifyASN("AS65002") {
		t.Fatalf("newly delegated ASN should still notify")
	}
	if !monitor.shouldNotifyASN("AS65100") {
		t.Fatalf("unknown ASN should still notify")
	}
}

func TestDiffDelegatedDataReturnsOnlyNewASNs(t *testing.T) {
	t.Parallel()

	previous := &DelegatedData{
		ASNs: map[string]struct{}{
			"AS65000": {},
			"AS65001": {},
		},
	}
	current := &DelegatedData{
		ASNs: map[string]struct{}{
			"AS65000": {},
			"AS65001": {},
			"AS65002": {},
		},
	}

	diff := diffDelegatedData(previous, current)
	if len(diff) != 1 {
		t.Fatalf("diff size = %d, want 1", len(diff))
	}
	if _, ok := diff["AS65002"]; !ok {
		t.Fatalf("expected AS65002 in diff")
	}
}
