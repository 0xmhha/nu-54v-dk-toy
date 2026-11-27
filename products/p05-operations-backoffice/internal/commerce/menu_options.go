package commerce

import (
	"encoding/json"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

const (
	catalogRowLimit     = 600
	optionGroupLimit    = 4
	optionValueLimit    = 12
	optionLabelLimit    = 24
	catalogPageRows     = 50
	catalogPageBytes    = 31 << 10 // the apps refuse a reply above 32768 bytes, which also carries the envelope and the shop
	nearbyRowBytes      = 1700     // one row has to fit the 2023-byte nearby reply, with its choices
	publishBytes        = 256 << 10
	catalogRequestBytes = 64 << 10
)

// A product's independent choices. Every combination is published as its own price row.
type skuOptions struct {
	SKU    string        `json:"sku"`
	Base   *optionBase   `json:"base,omitempty"` // filled by the server, returned for the next edit
	Groups []optionGroup `json:"groups"`
}

type optionGroup struct {
	Key    string            `json:"key"`
	Labels map[string]string `json:"labels"`
	Values []optionValue     `json:"values"`
}

type optionValue struct {
	Key       string            `json:"key"`
	Labels    map[string]string `json:"labels"`
	Price     string            `json:"price,omitempty"` // added to the product's price
	Allergens *[]string         `json:"allergens,omitempty"`
}

type optionBase struct {
	Label        string        `json:"label"`
	DisplayPrice string        `json:"displayPrice"`
	Tax          string        `json:"tax"`
	Discount     string        `json:"discount"`
	Metadata     *menuMetadata `json:"metadata,omitempty"`
}

// What a customer's screen needs: no amounts, those are in the rows.
type CatalogOptions struct {
	SKU    string         `json:"sku"`
	Groups []CatalogGroup `json:"groups"`
}

type CatalogGroup struct {
	Key    string            `json:"key"`
	Labels map[string]string `json:"labels"`
	Values []CatalogValue    `json:"values"`
}

type CatalogValue struct {
	Key    string            `json:"key"`
	Labels map[string]string `json:"labels"`
}

var (
	optionGroupKey = regexp.MustCompile(`^[a-z0-9]{1,16}$`)
	optionValueKey = regexp.MustCompile(`^[a-z0-9]{1,31}$`)
)

// Korean is required; a screen without the English text shows the Korean one.
func validOptionLabels(labels map[string]string) bool {
	if labels["ko"] == "" {
		return false
	}
	for language, label := range labels {
		if (language != "ko" && language != "en") || label == "" || strings.TrimSpace(label) != label ||
			utf8.RuneCountInString(label) > optionLabelLimit || strings.ContainsFunc(label, unicode.IsControl) ||
			strings.ContainsAny(label, "‪‫‬‭‮⁦⁧⁨⁩") {
			return false
		}
	}
	return true
}

func (o skuOptions) valid() bool {
	if o.Base != nil || len(o.Groups) < 1 || len(o.Groups) > optionGroupLimit {
		return false
	}
	groups := make(map[string]bool)
	for _, group := range o.Groups {
		if !optionGroupKey.MatchString(group.Key) || groups[group.Key] || !validOptionLabels(group.Labels) ||
			len(group.Values) < 2 || len(group.Values) > optionValueLimit {
			return false
		}
		groups[group.Key] = true
		values := make(map[string]bool)
		for _, value := range group.Values {
			if !optionValueKey.MatchString(value.Key) || values[value.Key] || !validOptionLabels(value.Labels) ||
				(value.Allergens != nil && !validDeclaredAllergens(*value.Allergens)) {
				return false
			}
			values[value.Key] = true
		}
	}
	return true
}

// expandOptions replaces each product row that has choices, already in base units, with one row for every combination.
// The amounts of the choices become base units too.
func expandOptions(rows []operatorRow, options []skuOptions, decimals int, token Token) ([]operatorRow, error) {
	bySKU := make(map[string]*skuOptions, len(options))
	for index := range options {
		if bySKU[options[index].SKU] != nil {
			return nil, service.BadRequest
		}
		bySKU[options[index].SKU] = &options[index]
	}
	var out []operatorRow
	for _, row := range rows {
		option := bySKU[row.SKU]
		if option == nil {
			out = append(out, row)
			continue
		}
		if option.Base != nil || row.DisplayPrice == "" {
			return nil, service.BadRequest // two rows for the product, or one priced in base units
		}
		option.Base = &optionBase{row.Label, row.DisplayPrice, row.Tax, row.Discount, row.Metadata}
		combined, err := option.combine(row, decimals, token, catalogRowLimit-len(out))
		if err != nil {
			return nil, err
		}
		out = append(out, combined...)
	}
	for _, option := range options {
		if option.Base == nil {
			return nil, service.BadRequest
		}
	}
	if len(out) > catalogRowLimit {
		return nil, service.BadRequest
	}
	return out, nil
}

func (o *skuOptions) combine(row operatorRow, decimals int, token Token, room int) ([]operatorRow, error) {
	metadata := menuMetadata{}
	if row.Metadata != nil {
		metadata = *row.Metadata
	}
	if len(metadata.OptionLabels) != 0 || len(metadata.OptionGroupLabels) != 0 {
		return nil, service.BadRequest // these come from the choices
	}
	rows := []operatorRow{row}
	rows[0].Required, rows[0].Metadata = true, nil
	allergens := []*[]string{metadata.Allergens}
	labels := []map[string]string{nil}
	groupLabels := map[string]string(nil)
	for g := range o.Groups {
		group := &o.Groups[g]
		groupLabels = joinLabels(groupLabels, group.Labels)
		if len(rows)*len(group.Values) > room {
			return nil, service.BadRequest
		}
		var nextRows []operatorRow
		var nextAllergens []*[]string
		var nextLabels []map[string]string
		for v := range group.Values {
			value := &group.Values[v]
			if value.Price == "" || strings.Trim(value.Price, "0.") == "" && displayAmount.MatchString(value.Price) {
				value.Price = "0"
			} else {
				units, err := operatorDecimalUnits(value.Price, decimals)
				// The tax is an amount of the product, so a dearer choice would carry too little of it.
				if err != nil || !token.shownPrice(units) || row.Tax != "0" {
					return nil, service.BadRequest
				}
				value.Price = units
			}
			// Adding an ingredient to a product that declares none would read as a complete list.
			if value.Allergens != nil && metadata.Allergens == nil {
				return nil, service.BadRequest
			}
		}
		for index := range rows {
			for _, value := range group.Values {
				next := rows[index]
				if next.Option != "" {
					next.Option += "-"
				}
				next.Option += value.Key
				var err error
				if next.DisplayPrice, err = addUnits(next.DisplayPrice, value.Price); err == nil {
					next.Price, err = addUnits(next.Price, value.Price)
				}
				if err != nil || !sku.MatchString(next.Option) || next.Option == "standard" {
					return nil, service.BadRequest
				}
				nextRows = append(nextRows, next)
				nextAllergens = append(nextAllergens, joinAllergens(allergens[index], value.Allergens))
				nextLabels = append(nextLabels, joinLabels(labels[index], value.Labels))
			}
		}
		rows, allergens, labels = nextRows, nextAllergens, nextLabels
	}
	for index := range rows {
		own := metadata
		own.Allergens, own.OptionLabels, own.OptionGroupLabels = allergens[index], labels[index], groupLabels
		rows[index].Metadata = &own
	}
	return rows, nil
}

// joinLabels writes "뜨거움 · 큰 컵". Apps that only know one list of options show this text.
func joinLabels(before, after map[string]string) map[string]string {
	label := func(labels map[string]string, language string) string {
		if labels[language] == "" {
			return labels["ko"]
		}
		return labels[language]
	}
	out := make(map[string]string, 2)
	for _, language := range []string{"ko", "en"} {
		out[language] = label(after, language)
		if before != nil {
			out[language] = label(before, language) + " · " + out[language]
		}
	}
	return out
}

func joinAllergens(before, after *[]string) *[]string {
	if before == nil || after == nil {
		return before
	}
	out := slices.Clone(*before)
	for _, code := range *after {
		if !slices.Contains(out, code) {
			out = append(out, code)
		}
	}
	return &out
}

func addUnits(a, b string) (string, error) {
	left, okLeft := new(big.Int).SetString(a, 10)
	right, okRight := new(big.Int).SetString(b, 10)
	if !okLeft || !okRight || left.Add(left, right).BitLen() > 256 {
		return "", service.BadRequest
	}
	return left.String(), nil
}

// readableCatalog refuses a publication the apps could not read back: each page of rows and, for a phone next to the
// kiosk, each single row has a size limit.
func readableCatalog(rows []operatorRow, options []skuOptions, categories []json.RawMessage, bindings []categoryBinding, photos ...menuPhoto) bool {
	// A product with a photo carries its two names once on a page, and in each of its rows next to the kiosk.
	pictured := make(map[string]int, len(photos))
	for _, photo := range photos {
		pictured[photo.SKU] = size(CatalogPhoto{photo.SKU, photo.Photo, photo.Photo}) + 1
	}
	byCategory := make(map[string]*CatalogCategory)
	for _, raw := range categories {
		var category CatalogCategory
		if json.Unmarshal(raw, &category) != nil {
			return false
		}
		byCategory[category.Key] = &category
	}
	bySKU := make(map[string]*CatalogCategory)
	for _, binding := range bindings {
		bySKU[binding.SKU] = byCategory[binding.CategoryKey]
	}
	type choices struct{ page, row int }
	sizes := make(map[string]choices)
	for _, option := range options {
		widest := choices{page: size(option.public())}
		for _, group := range option.public().Groups {
			most := 0
			for _, value := range group.Values {
				most = max(most, size(value))
			}
			widest.row += size(CatalogGroup{Key: group.Key, Labels: group.Labels}) + most
		}
		sizes[option.SKU] = widest
	}
	ordered := slices.Clone(rows)
	slices.SortFunc(ordered, func(a, b operatorRow) int {
		if order := strings.Compare(a.SKU, b.SKU); order != 0 {
			return order
		}
		return strings.Compare(a.Option, b.Option)
	})
	page, seen := 0, make(map[string]bool)
	for index, row := range ordered {
		if index%catalogPageRows == 0 {
			page = 0
			clear(seen)
		}
		published := Price{Version: 1 << 53, SKU: row.SKU, Option: row.Option, Label: row.Label, Asset: row.Asset,
			UnitPrice: row.Price, Available: 1 << 53, SKUAvailable: 1 << 53, Category: bySKU[row.SKU]}
		if row.Metadata != nil || row.Required {
			published.Metadata = &publishedMenuMetadata{OptionRequired: row.Required}
			if row.Metadata != nil {
				published.Metadata.menuMetadata = *row.Metadata
			}
		}
		bytes := size(published) + 8 // microseconds in the expiry, and the comma
		if bytes+sizes[row.SKU].row+pictured[row.SKU] > nearbyRowBytes {
			return false
		}
		if page += bytes; !seen[row.SKU] {
			page += sizes[row.SKU].page + 1 + pictured[row.SKU]
			seen[row.SKU] = true
		}
		if page > catalogPageBytes {
			return false
		}
	}
	return true
}

func size(value any) int {
	raw, err := json.Marshal(value)
	if err != nil {
		return 1 << 30
	}
	return len(raw)
}

func (o skuOptions) public() CatalogOptions {
	out := CatalogOptions{SKU: o.SKU}
	for _, group := range o.Groups {
		shown := CatalogGroup{Key: group.Key, Labels: group.Labels}
		for _, value := range group.Values {
			shown.Values = append(shown.Values, CatalogValue{value.Key, value.Labels})
		}
		out.Groups = append(out.Groups, shown)
	}
	return out
}
