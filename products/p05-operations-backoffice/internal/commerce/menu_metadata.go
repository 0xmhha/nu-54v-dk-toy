package commerce

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type menuMetadata struct {
	Labels                map[string]string `json:"labels,omitempty"`
	OptionLabels          map[string]string `json:"optionLabels,omitempty"`
	OptionGroupLabels     map[string]string `json:"optionGroupLabels,omitempty"`
	CrossContactAllergens *[]string         `json:"crossContactAllergens,omitempty"`
	CrossContactSource    string            `json:"crossContactSource,omitempty"`
	Allergens             *[]string         `json:"allergens,omitempty"`
	CrossContact          string            `json:"crossContact,omitempty"`
	// The tax the price includes: "vat10" or "exempt". Empty keeps the old way, a tax amount added to the price.
	Tax string `json:"tax,omitempty"`
	// A good a visitor may claim the tax back on. Shown only; nothing is taken off the price yet.
	TaxRefund bool `json:"taxRefund,omitempty"`
}

type publishedMenuMetadata struct {
	menuMetadata
	OptionRequired bool `json:"optionRequired,omitempty"`
}

const declaredAllergens = "|milk|egg|fish|shellfish|tree-nuts|peanut|wheat|soy|sesame|buckwheat|peach|tomato|sulfites|walnut|pine-nuts|pork|beef|chicken|squid|"

func (m menuMetadata) valid() bool {
	for _, translations := range []map[string]string{m.Labels, m.OptionLabels, m.OptionGroupLabels} {
		for language, label := range translations {
			if (language != "ko" && language != "en") || strings.TrimSpace(label) != label ||
				label == "" || utf8.RuneCountInString(label) > 120 || strings.ContainsFunc(label, unicode.IsControl) {
				return false
			}
		}
	}
	if m.CrossContact != "" && m.CrossContact != "unknown" && m.CrossContact != "possible" && m.CrossContact != "declared-none" {
		return false
	}
	for _, label := range m.OptionGroupLabels {
		if strings.ContainsAny(label, "\u202a\u202b\u202c\u202d\u202e\u2066\u2067\u2068\u2069") {
			return false
		}
	}
	if m.CrossContactSource != "" && (m.CrossContact != "possible" || (m.CrossContactSource != "shared-equipment" && m.CrossContactSource != "shared-baking-equipment")) {
		return false
	}
	if m.CrossContactAllergens != nil && (m.CrossContact != "possible" || len(*m.CrossContactAllergens) == 0) {
		return false
	}
	if (m.Tax != "" && m.Tax != "vat10" && m.Tax != "exempt") || (m.TaxRefund && m.Tax == "") {
		return false
	}
	return (m.Allergens == nil || validDeclaredAllergens(*m.Allergens)) &&
		(m.CrossContactAllergens == nil || validDeclaredAllergens(*m.CrossContactAllergens))
}

func validDeclaredAllergens(codes []string) bool {
	if len(codes) > 19 {
		return false
	}
	seen := make(map[string]bool, len(codes))
	for _, code := range codes {
		if code == "" || strings.ContainsRune(code, '|') || !strings.Contains(declaredAllergens, "|"+code+"|") || seen[code] {
			return false
		}
		seen[code] = true
	}
	return true
}
