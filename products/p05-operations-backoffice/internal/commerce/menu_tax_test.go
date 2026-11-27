package commerce

import (
	"encoding/json"
	"testing"
)

func TestMenuTaxKind(t *testing.T) {
	for _, raw := range []string{`{"tax":"vat10"}`, `{"tax":"exempt"}`, `{"tax":"vat10","takeAway":true,"taxRefund":true}`, `{"takeAway":true}`, `{}`} {
		var m menuMetadata
		if json.Unmarshal([]byte(raw), &m) != nil || !m.valid() {
			t.Error("refused", raw)
		}
	}
	for _, raw := range []string{`{"tax":"vat20"}`, `{"taxRefund":true}`, `{"tax":"VAT10"}`,
		// A refund needs a taxed good that is taken away.
		`{"tax":"vat10","taxRefund":true}`, `{"tax":"exempt","takeAway":true,"taxRefund":true}`} {
		var m menuMetadata
		if json.Unmarshal([]byte(raw), &m) == nil && m.valid() {
			t.Error("taken", raw)
		}
	}
	// The combinations of a product carry its tax, and a choice may add to a price that includes its tax.
	row := latteRow()
	row.Metadata.Tax, row.Metadata.TakeAway, row.Metadata.TaxRefund = "vat10", true, true
	rows, err := expandOptions([]operatorRow{row}, latte(), 2, wonForOptions)
	if err != nil || len(rows) != 4 {
		t.Fatal(err)
	}
	for _, combination := range rows {
		if combination.Metadata.Tax != "vat10" || !combination.Metadata.TaxRefund || !combination.Metadata.valid() {
			t.Fatalf("%+v", combination.Metadata)
		}
	}
}
