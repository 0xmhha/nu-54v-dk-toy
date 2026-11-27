package commerce

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
)

// The store page (traveller-services.md chapter 9 items 6 and 7): customers pick a store from the app's list or open
// one with its shortcut QR, before any account proof. The two reads on the customer entrance therefore take no
// session; they answer only what the store published on the operations web, the store's registered name, whether it
// takes orders now, the token it is paid in, its number in the order contract and the operator's contact. The store writes the page; nothing is made up
// here, and a field the store left empty is left out.
//
// The status of a page says whether an order can be placed now (`orderable`), and when not, why (`reason`:
// closed_day, paused, no_menu or outside_hours). Its `mode` is open or busy only while an order can be placed.
//
// A store is public only when it said so: the page, the menu and the photos answer not found until the store
// switches its page to published on the operations web (off by default). Whether a published store also appears in
// the app's list is a second switch of its own.

const storeListLimit = 200

// customerStores lists the published stores that chose to be listed, by name. The app searches the list by name itself.
func (h HTTP) customerStores(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	var stores json.RawMessage
	if err := h.Store.db.QueryRow(r.Context(), "SELECT ledger.public_store_list($1)", storeListLimit).Scan(&stores); err != nil {
		errorResponse(w, domainError(err), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]json.RawMessage{"stores": stores})
}

// customerStore answers one store's page. A branch that does not exist, has no registered name or did not publish
// its page is not found, all three alike.
func (h HTTP) customerStore(w http.ResponseWriter, r *http.Request) {
	branch := r.PathValue("branch")
	if h.Store == nil || r.URL.RawQuery != "" || !operatorRef.MatchString(branch) {
		http.NotFound(w, r)
		return
	}
	var raw []byte
	if err := h.Store.db.QueryRow(r.Context(), "SELECT ledger.public_store_page($1)", branch).Scan(&raw); err != nil {
		errorResponse(w, domainError(err), nil)
		return
	}
	var page map[string]json.RawMessage
	if raw == nil || json.Unmarshal(raw, &page) != nil || page == nil {
		http.NotFound(w, r)
		return
	}
	if profile, enrolled := h.Profiles[branch]; enrolled {
		for key, value := range storePayment(profile) {
			page[key], _ = json.Marshal(value)
		}
	}
	if contact := h.Customer.contact(); contact != "" {
		page["operatorContact"], _ = json.Marshal(contact)
	}
	writeJSON(w, http.StatusOK, page)
}

// storePayment is all a store page says about paying the store: the label of the token a board pays it in, when this
// server issues its quotes, and the store's number in the order contract. The app holds the chain, the contract and
// the token in its own build and reads that merchant's signer and beneficiary from the contract itself, so a page
// never names an address, a chain, a signer or a limit: those would be values the app must not take from a server.
func storePayment(profile Profile) map[string]string {
	payment := map[string]string{"paymentToken": profile.Token.OrDollar().Label}
	if profile.Escrow && profile.MerchantID != ([32]byte{}) {
		payment["merchantId"] = hex.EncodeToString(profile.MerchantID[:])
	}
	return payment
}

// contact is the operator's contact for payment and refund questions, when the deployment set a usable one.
func (c *CustomerEntrance) contact() string {
	if c == nil {
		return ""
	}
	text := strings.TrimSpace(c.OperatorContact)
	if text == "" || utf8.RuneCountInString(text) > 120 || strings.ContainsFunc(text, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return ""
	}
	return text
}

// storePageChange is the whole page as the store's operator writes it. Hours is seven entries from Monday, each
// null or {"opens","closes"}; null as a whole means the store has not said its hours.
type storePageChange struct {
	Published  *bool           `json:"published"`
	Listed     *bool           `json:"listed"`
	Kind       *string         `json:"kind"`
	Intro      *string         `json:"intro"`
	Address    *string         `json:"address"`
	Directions *string         `json:"directions"`
	Phone      *string         `json:"phone"`
	Amenities  *string         `json:"amenities"`
	Parking    *string         `json:"parking"`
	ClosedNote *string         `json:"closedNote"`
	Hours      json.RawMessage `json:"hours"`
}

func (c storePageChange) complete() bool {
	for _, text := range []*string{c.Kind, c.Intro, c.Address, c.Directions, c.Phone, c.Amenities, c.Parking, c.ClosedNote} {
		if text == nil {
			return false
		}
	}
	return c.Published != nil && c.Listed != nil && len(c.Hours) != 0
}

func (h HTTP) registerStorePage(mux *http.ServeMux) {
	mux.HandleFunc("GET /v2/operations/store-page/{branch}", h.operateStorePage)
	mux.HandleFunc("POST /v2/operations/store-page/{branch}", h.operateStorePage)
}

// operateStorePage reads or replaces the store page from the operations web. The database checks every field.
func (h HTTP) operateStorePage(w http.ResponseWriter, r *http.Request) {
	branch := r.PathValue("branch")
	actor, ok := h.operatorCredential(r)
	if !operatorRef.MatchString(branch) || !ok || h.Store == nil {
		http.NotFound(w, r)
		return
	}
	ctx := authority.Scope(r.Context(), actor, branch)
	var reply json.RawMessage
	if r.Method == http.MethodGet {
		err := h.Store.db.QueryRow(ctx, "SELECT ledger.operator_store_page($1,$2,$3)", actor.Tenant, branch, actor.Subject).Scan(&reply)
		operatorResponse(w, reply, err)
		return
	}
	if h.OperatorOrigin == "" || r.Header.Get("Origin") != h.OperatorOrigin || strings.TrimSpace(r.Header.Get("Content-Type")) != "application/json" {
		http.NotFound(w, r)
		return
	}
	var change storePageChange
	if !decodeStrict(w, r, 8<<10, &change) || !change.complete() {
		operatorResponse(w, nil, service.BadRequest)
		return
	}
	page, _ := json.Marshal(change)
	err := h.Store.db.QueryRow(ctx, "SELECT ledger.set_store_page($1,$2,$3,$4)", actor.Tenant, branch, actor.Subject, page).Scan(&reply)
	operatorResponse(w, reply, err)
}
