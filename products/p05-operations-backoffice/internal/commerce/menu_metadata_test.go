package commerce

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMenuMetadataDoesNotInferDeclarations(t *testing.T) {
	for _, test := range []struct {
		input string
		ok    bool
	}{
		{`{"labels":{"ko":"차","en":"Tea"},"optionLabels":{"ko":"따뜻함"},"allergens":[],"crossContact":"unknown"}`, true},
		{`{"allergens":["milk","soy"]}`, true},
		{`{"allergens":["milk","milk"]}`, false},
		{`{"labels":{"auto":"translated"}}`, false},
		{`{"crossContact":"guaranteed-safe"}`, false},
		{`{"allergens":["guess"]}`, false},
		{`{"optionGroupLabels":{"ko":"온도","en":"Temperature"},"crossContact":"possible","crossContactAllergens":["tree-nuts"],"crossContactSource":"shared-equipment"}`, true},
		{`{"optionGroupLabels":{"fr":"Température"}}`, false},
		{`{"optionGroupLabels":{"en":"Temperature\u202e"}}`, false},
		{`{"crossContact":"unknown","crossContactAllergens":["tree-nuts"]}`, false},
		{`{"crossContact":"possible","crossContactAllergens":["tree-nuts","tree-nuts"]}`, false},
		{`{"crossContact":"possible","crossContactAllergens":["unknown"]}`, false},
		{`{"crossContact":"possible","crossContactAllergens":[]}`, false},
		{`{"crossContact":"possible","crossContactSource":"unsupported"}`, false},
	} {
		var metadata menuMetadata
		if err := json.Unmarshal([]byte(test.input), &metadata); err != nil {
			t.Fatal(err)
		}
		if metadata.valid() != test.ok {
			t.Errorf("%s expected valid=%t", test.input, test.ok)
		}
	}
}

func TestOperatorWallExpiryAtDSTBoundary(t *testing.T) {
	first := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	for _, instant := range []time.Time{first, second} {
		if !matchesWallExpiry(instant, "America/New_York", "2026-11-01T01:30") {
			t.Fatal("valid DST fold occurrence refused")
		}
	}
	if matchesWallExpiry(time.Date(2026, 3, 8, 7, 30, 0, 0, time.UTC), "America/New_York", "2026-03-08T02:30") {
		t.Fatal("nonexistent date accepted")
	}
}
