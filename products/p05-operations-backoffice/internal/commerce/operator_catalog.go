package commerce

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
	"io"
	"maps"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

type operatorRow struct {
	SKU          string        `json:"sku"`
	Option       string        `json:"option"`
	Label        string        `json:"label"`
	Asset        string        `json:"asset"`
	Price        string        `json:"price,omitempty"`
	DisplayPrice string        `json:"displayPrice,omitempty"`
	Tax          string        `json:"tax,omitempty"`
	Discount     string        `json:"discount,omitempty"`
	Required     bool          `json:"required,omitempty"`
	Metadata     *menuMetadata `json:"metadata,omitempty"`
}
type CatalogCategory struct {
	Key       string            `json:"key"`
	SortOrder int               `json:"sortOrder"`
	Labels    map[string]string `json:"labels"`
}

type categoryBinding struct {
	SKU         string `json:"sku"`
	CategoryKey string `json:"categoryKey"`
}

type operatorMutation struct {
	Key              string            `json:"requestKey"`
	Version          int64             `json:"version"`
	ExpiresAt        *time.Time        `json:"expiresAt,omitempty"`
	LocalExpiry      string            `json:"localExpiry,omitempty"`
	Rows             []operatorRow     `json:"rows,omitempty"`
	SKU              string            `json:"sku,omitempty"`
	Delta            *int64            `json:"delta,omitempty"`
	Option           string            `json:"option,omitempty"`
	Hours            *operatorHours    `json:"hours,omitempty"`
	Categories       []json.RawMessage `json:"categories,omitempty"`
	CategoryBindings []categoryBinding `json:"categoryBindings,omitempty"`
}

var operatorRef = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var catalogAsset = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
var catalogUnits = regexp.MustCompile(`^[1-9][0-9]{0,77}$`)

type operatorHours struct {
	Opens  string `json:"opens"`
	Closes string `json:"closes"`
	Zone   string `json:"zone"`
}

var localTime = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
var zoneName = regexp.MustCompile(`^(UTC|[A-Za-z]+(/[A-Za-z0-9_+.-]+){1,3})$`)

func (h operatorHours) valid() bool {
	return localTime.MatchString(h.Opens) && localTime.MatchString(h.Closes) &&
		h.Opens < h.Closes && zoneName.MatchString(h.Zone)
}

type operatorAsset struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
	Chain    uint64 `json:"chainId"`
}

var displayAmount = regexp.MustCompile(`^(0|[1-9][0-9]{0,77})(\.[0-9]{1,18})?$`)

func operatorDecimalUnits(input string, decimals int) (string, error) {
	if decimals < 0 || decimals > 18 || !displayAmount.MatchString(input) {
		return "", service.BadRequest
	}
	whole, fraction, _ := strings.Cut(input, ".")
	if len(fraction) > decimals {
		return "", service.BadRequest
	}
	units := strings.TrimLeft(whole+fraction+strings.Repeat("0", decimals-len(fraction)), "0")
	parsed, ok := new(big.Int).SetString(units, 10)
	if !ok || parsed.BitLen() > 256 {
		return "", service.BadRequest
	}
	return units, nil
}

func (row *operatorRow) resolvePrice(decimals int) error {
	base, err := operatorDecimalUnits(row.DisplayPrice, decimals)
	if err != nil || row.Price != "" {
		return service.BadRequest
	}
	adjustment := func(display string) (string, error) {
		if display == "" || (displayAmount.MatchString(display) && strings.Trim(display, "0.") == "") {
			return "0", nil
		}
		return operatorDecimalUnits(display, decimals)
	}
	tax, err := adjustment(row.Tax)
	if err != nil {
		return err
	}
	discount, err := adjustment(row.Discount)
	if err != nil {
		return err
	}
	amount, _ := new(big.Int).SetString(base, 10)
	taxUnits, _ := new(big.Int).SetString(tax, 10)
	discountUnits, _ := new(big.Int).SetString(discount, 10)
	amount.Add(amount, taxUnits).Sub(amount, discountUnits)
	if amount.Sign() <= 0 || amount.BitLen() > 256 {
		return service.BadRequest
	}
	row.Price, row.DisplayPrice, row.Tax, row.Discount = amount.String(), base, tax, discount
	return nil
}

func (h HTTP) assetForOperations(branch string) *operatorAsset {
	profile, ok := h.Profiles[branch]
	if !ok || profile.Chain != 8283 || profile.Asset.Hex() == "0x0000000000000000000000000000000000000000" {
		return nil
	}
	return &operatorAsset{strings.ToLower(profile.Asset.Hex()), "TestUSDC", 6, profile.Chain}
}

func (h HTTP) registerOperators(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/operations/scopes", h.operateScopes)
	mux.HandleFunc("GET /v2/operations/catalog/{branch}", h.operateCatalog)
	mux.HandleFunc("POST /v2/operations/catalog/{branch}/{operation}", h.operateCatalog)
}

func (h HTTP) operatorCredential(r *http.Request) (service.Principal, bool) {
	principal, _, exists := service.CurrentClientPrincipal(r.TLS, h.Credentials, time.Now())
	if !exists || principal.Role != "operator" || principal.ExpiresAt <= time.Now().Unix() ||
		!operatorRef.MatchString(principal.Tenant) || !operatorRef.MatchString(principal.Subject) {
		return service.Principal{}, false
	}
	return principal, true
}

func (h HTTP) operateScopes(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.operatorCredential(r)
	if !ok || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	var reply json.RawMessage
	err := h.Store.db.QueryRow(r.Context(), "SELECT ledger.operator_scopes($1,$2,'operator')",
		actor.Tenant, actor.Subject).Scan(&reply)
	operatorResponse(w, reply, err)
}

func (h HTTP) operateCatalog(w http.ResponseWriter, r *http.Request) {
	branch := r.PathValue("branch")
	actor, ok := h.operatorCredential(r)
	if !operatorRef.MatchString(branch) || !ok || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	ctx := authority.Scope(r.Context(), actor, branch)
	asset := h.assetForOperations(branch)
	var reply json.RawMessage
	if r.Method == http.MethodGet {
		err := h.Store.db.QueryRow(ctx, "SELECT ledger.operator_catalog_home($1,$2,$3)", actor.Tenant, branch, actor.Subject).Scan(&reply)
		if err != nil {
			operatorResponse(w, nil, err)
			return
		}
		certificate := r.TLS.PeerCertificates[0].Raw
		scope := sha256.Sum256(append(append([]byte("nu54-catalog-console-v1"), certificate...), branch...))
		writeJSON(w, http.StatusOK, struct {
			Catalog      json.RawMessage `json:"catalog"`
			ReceiptScope string          `json:"receiptScope"`
			Asset        *operatorAsset  `json:"asset,omitempty"`
		}{reply, hex.EncodeToString(scope[:]), asset})
		return
	}
	origin := r.Header.Get("Origin")
	if h.OperatorOrigin == "" || origin != h.OperatorOrigin || strings.TrimSpace(r.Header.Get("Content-Type")) != "application/json" {
		http.NotFound(w, r)
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536))
	decoder.DisallowUnknownFields()
	var mutation operatorMutation
	if err := decoder.Decode(&mutation); err != nil || !operatorRef.MatchString(mutation.Key) || mutation.Version < 0 {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	if _, err := decoder.Token(); err != io.EOF {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	requestKey := mutation.Key
	var err error
	switch r.PathValue("operation") {
	case "publish":
		if mutation.ExpiresAt == nil || (mutation.Hours != nil && !mutation.Hours.valid()) || mutation.SKU != "" || mutation.Option != "" || mutation.Delta != nil || len(mutation.Rows) < 1 || len(mutation.Rows) > 150 {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		if mutation.LocalExpiry != "" {
			var zone string
			err := h.Store.db.QueryRow(ctx, "SELECT ledger.operator_catalog_home($1,$2,$3)->>'timezone'",
				actor.Tenant, branch, actor.Subject).Scan(&zone)
			if err != nil {
				operatorResponse(w, nil, err)
				return
			}
			if !matchesWallExpiry(*mutation.ExpiresAt, zone, mutation.LocalExpiry) {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
		}
		groups := make(map[string]map[string]string)
		for index := range mutation.Rows {
			row := &mutation.Rows[index]
			if asset != nil && row.Asset != asset.Address {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			if row.Metadata != nil && (row.DisplayPrice == "" || !row.Metadata.valid()) {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			var group map[string]string
			if row.Metadata != nil {
				group = row.Metadata.OptionGroupLabels
			}
			previous, seen := groups[row.SKU]
			if (len(group) != 0 && !row.Required) || (seen && !maps.Equal(previous, group)) {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			groups[row.SKU] = group
			if row.DisplayPrice != "" {
				if asset == nil || row.resolvePrice(asset.Decimals) != nil {
					operatorResponse(w, nil, service.BadRequest)
					return
				}
			} else if row.Tax != "" || row.Discount != "" || row.Required || mutation.Hours != nil {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			if !sku.MatchString(row.SKU) || !sku.MatchString(row.Option) || len(strings.TrimSpace(row.Label)) < 1 ||
				len([]rune(row.Label)) > 120 || !catalogAsset.MatchString(row.Asset) || row.Asset == "0x"+strings.Repeat("0", 40) ||
				!catalogUnits.MatchString(row.Price) {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
		}
		rows, marshalErr := json.Marshal(mutation.Rows)
		if marshalErr != nil {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		hours, marshalErr := json.Marshal(mutation.Hours)
		if marshalErr != nil {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		if mutation.Categories == nil && mutation.CategoryBindings == nil {
			err = h.Store.db.QueryRow(ctx, "SELECT ledger.publish_operator_catalog_pricing($1,$2,$3,$4,$5,$6,$7,$8)",
				actor.Tenant, branch, actor.Subject, requestKey, mutation.Version, mutation.ExpiresAt.UTC(), rows, hours).Scan(&reply)
			break
		}
		if mutation.Categories == nil || mutation.CategoryBindings == nil || len(mutation.Categories) > 50 || len(mutation.CategoryBindings) > 150 {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		for _, raw := range mutation.Categories {
			var category CatalogCategory
			if json.Unmarshal(raw, &category) != nil || strings.TrimSpace(category.Labels["ko"]) != category.Labels["ko"] ||
				strings.TrimSpace(category.Labels["en"]) != category.Labels["en"] {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
		}
		categories, categoryErr := json.Marshal(mutation.Categories)
		bindings, bindingErr := json.Marshal(mutation.CategoryBindings)
		if categoryErr != nil || bindingErr != nil {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.publish_operator_catalog_categories($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)",
			actor.Tenant, branch, actor.Subject, requestKey, mutation.Version, mutation.ExpiresAt.UTC(), rows, hours, categories, bindings).Scan(&reply)
	case "stock", "option-stock":
		if mutation.ExpiresAt != nil || mutation.LocalExpiry != "" || mutation.Hours != nil || mutation.Rows != nil || mutation.Categories != nil || mutation.CategoryBindings != nil || mutation.Delta == nil || !sku.MatchString(mutation.SKU) {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		if r.PathValue("operation") == "option-stock" {
			if !sku.MatchString(mutation.Option) {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			err = h.Store.db.QueryRow(ctx, "SELECT ledger.adjust_catalog_option($1,$2,$3,$4,$5,$6,$7,$8)",
				actor.Tenant, branch, actor.Subject, requestKey, mutation.Version, mutation.SKU, mutation.Option, *mutation.Delta).Scan(&reply)
			break
		}
		if mutation.Option != "" {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		err = h.Store.db.QueryRow(ctx, "SELECT ledger.adjust_operator_stock($1,$2,$3,$4,$5,$6,$7)",
			actor.Tenant, branch, actor.Subject, requestKey, mutation.Version, mutation.SKU, *mutation.Delta).Scan(&reply)
	default:
		http.NotFound(w, r)
		return
	}
	operatorResponse(w, reply, err)
}

func operatorResponse(w http.ResponseWriter, reply json.RawMessage, err error) {
	if err != nil {
		errorResponse(w, domainError(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, reply)
}

func matchesWallExpiry(expires time.Time, zone, local string) bool {
	location, err := time.LoadLocation(zone)
	return err == nil && expires.In(location).Format("2006-01-02T15:04") == local
}
