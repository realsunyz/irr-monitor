package arin

import (
	"reflect"
	"testing"

	"github.com/realSunyz/irr-monitor/internal/delegated"
	"github.com/realSunyz/irr-monitor/internal/telegram"
)

func TestNotifyNewDelegatedASNsRequiresRecentAssignmentDate(t *testing.T) {
	t.Parallel()

	var notified []telegram.AutNum
	monitor := &Monitor{
		callback: func(_ string, autNum *telegram.AutNum) {
			notified = append(notified, *autNum)
		},
		lookup: func(asn string, metadata delegated.ASNMetadata) *telegram.AutNum {
			return &telegram.AutNum{
				ASN:     asn,
				Country: metadata.Country,
				Source:  Source,
			}
		},
	}

	snapshot := &delegated.Snapshot{
		Metadata: map[string]delegated.ASNMetadata{
			"AS402316": {Country: "US", Date: "20260324"},
			"AS402317": {Country: "US", Date: "20260324"},
			"AS402318": {Country: "CA", Date: "20260323"},
			"AS47011":  {Country: "US", Date: "20230228"},
		},
	}

	lastASN := monitor.notifyNewDelegatedASNs(snapshot, []string{"AS402316", "AS402317", "AS402318", "AS47011"}, map[string]struct{}{
		"20260324": {},
		"20260323": {},
	})

	want := []telegram.AutNum{
		{ASN: "AS402316", Country: "US", Source: Source},
		{ASN: "AS402317", Country: "US", Source: Source},
		{ASN: "AS402318", Country: "CA", Source: Source},
	}
	if !reflect.DeepEqual(notified, want) {
		t.Fatalf("notified = %#v, want %#v", notified, want)
	}
	if lastASN != "AS402318" {
		t.Fatalf("lastASN = %q, want AS402318", lastASN)
	}
}
