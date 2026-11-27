package commerce

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// The public menu (owner decision 2026-11-27): a customer reads a store's menu, its photos and what is sold out from
// the store page, before any account proof, and keeps the cart on the phone. Only the two reads below are public.
// Nothing here holds stock, names a customer or an order, or says how the store is paid; the cart, the quote and the
// payment still need the session of an account proof (customer_entrance.go), and stock is held only by that
// session's order.

// publicMenuPage is one page of a store's menu in the words of the C3 catalog.read reply, so the app reads both the
// same way. The store's name and whether it takes orders come with the first page.
type publicMenuPage struct {
	Shop           *ShopIdentity       `json:"shop,omitempty"`
	OrderState     *CustomerOrderState `json:"orderState,omitempty"`
	Catalog        []Price             `json:"catalog"`
	CatalogVersion *int64              `json:"catalogVersion,omitempty"`
	CatalogOptions []CatalogOptions    `json:"catalogOptions,omitempty"`
	CatalogPhotos  []CatalogPhoto      `json:"catalogPhotos,omitempty"`
}

// A whole number without a sign or leading zeros, so one page or part has one address.
func pathNumber(text string, limit int64) (int64, bool) {
	value, err := strconv.ParseInt(text, 10, 64)
	return value, err == nil && value >= 0 && value <= limit && strconv.FormatInt(value, 10) == text
}

// customerMenu answers one page (50 rows) of the published menu of a store that has a store page.
func (h HTTP) customerMenu(w http.ResponseWriter, r *http.Request) {
	branch := r.PathValue("branch")
	page, ok := pathNumber(r.PathValue("page"), 32767)
	if h.Store == nil || r.URL.RawQuery != "" || !operatorRef.MatchString(branch) || !ok {
		http.NotFound(w, r)
		return
	}
	var reply publicMenuPage
	var state []byte
	var label string
	err := h.Store.db.QueryRow(r.Context(), "SELECT snapshot_version,items,options,photos,state,label FROM ledger.public_menu_window($1,$2)",
		branch, int32(page)).Scan(&reply.CatalogVersion, &reply.Catalog, &reply.CatalogOptions, &reply.CatalogPhotos, &state, &label)
	if err != nil {
		errorResponse(w, domainError(err), nil)
		return
	}
	var wire struct {
		CustomerOrderState
		SoldOut []string `json:"soldOut"`
	}
	if json.Unmarshal(state, &wire) != nil {
		errorResponse(w, service.Unknown, nil)
		return
	}
	// A product the store marked sold out reads as none left, as in a session's menu.
	markSoldOut(reply.Catalog, wire.SoldOut)
	if reply.Catalog == nil {
		reply.Catalog = []Price{}
	}
	if page == 0 {
		reply.Shop, reply.OrderState = &ShopIdentity{Branch: branch, BranchLabel: label}, &wire.CustomerOrderState
	}
	writeJSON(w, http.StatusOK, reply)
}

// customerMenuPhoto answers one part (at most 18,000 bytes) of a published menu photo by its name. Photos are the
// largest answers of the entrance, so they have a request limit of their own (CustomerEntrance.PhotoLimiter).
func (h HTTP) customerMenuPhoto(w http.ResponseWriter, r *http.Request) {
	branch, digest := r.PathValue("branch"), r.PathValue("photo")
	offset, ok := pathNumber(r.PathValue("offset"), photoBytes)
	if h.Store == nil || r.URL.RawQuery != "" || !operatorRef.MatchString(branch) || !photoDigest.MatchString(digest) || !ok {
		http.NotFound(w, r)
		return
	}
	var size int
	var part []byte
	err := h.Store.db.QueryRow(r.Context(), "SELECT size,part FROM ledger.public_menu_photo_part($1,$2,$3)", branch, digest, int32(offset)).Scan(&size, &part)
	if err != nil {
		errorResponse(w, domainError(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"photo": PhotoPart{Size: size, Data: base64.StdEncoding.EncodeToString(part)}})
}
