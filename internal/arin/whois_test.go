package arin

import (
	"strings"
	"testing"
)

func TestParseWhoisAutnum(t *testing.T) {
	t.Parallel()

	body := `
ASNumber:       402329
ASName:         P3-EARLYBIRD
ASHandle:       AS402329

OrgName:        P3 Technologies
OrgId:          TP-697
Address:        2147 N Callow Ave
City:           Bremerton
StateProv:      WA
PostalCode:     98312
Country:        US
`

	info := parseWhoisAutnum(strings.NewReader(body), "AS402329", "")
	if info.AsName != "P3-EARLYBIRD" {
		t.Fatalf("AsName = %q, want P3-EARLYBIRD", info.AsName)
	}
	if info.Org != "TP-697" {
		t.Fatalf("Org = %q, want TP-697", info.Org)
	}
	if info.OrgName != "P3 Technologies" {
		t.Fatalf("OrgName = %q, want P3 Technologies", info.OrgName)
	}
	if info.Country != "US" {
		t.Fatalf("Country = %q, want US", info.Country)
	}
}
