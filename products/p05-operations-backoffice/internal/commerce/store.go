package commerce

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

type Store struct{ db authority.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid commerce database settings")
	}
	config.ConnConfig.RuntimeParams["lock_timeout"] = "2s"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "7s"
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("commerce database connection failed")
	}
	var allowed bool
	var encoding string
	err = db.QueryRow(ctx, `SELECT current_setting('server_encoding'),
		pg_has_role(current_user, 'nu54_p05_commerce', 'USAGE')
        AND NOT pg_has_role(current_user, 'nu54_p05_payout_worker', 'MEMBER')
		AND NOT pg_has_role(current_user, 'nu54_p05_staff', 'MEMBER')
		AND NOT (SELECT rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls FROM pg_roles WHERE rolname=current_user)
		AND NOT has_schema_privilege('ledger', 'CREATE')
		AND NOT has_table_privilege('ledger.quote_work', 'SELECT')
		AND NOT has_table_privilege('ledger.sale', 'INSERT,UPDATE,DELETE')`).Scan(&encoding, &allowed)
	if err != nil || !allowed {
		db.Close()
		return nil, errors.New("migrated database and least-privilege P05 commerce role required")
	}
	if encoding != "UTF8" {
		db.Close()
		return nil, errors.New("P05 commerce requires a UTF8 database; check SHOW server_encoding")
	}
	return &Store{db: authority.Pool{Pool: db}}, nil
}

func (s *Store) Close() { s.db.Close() }

type Item struct {
	SKU    string `json:"sku"`
	Option string `json:"option_key"`
	Qty    int64  `json:"quantity"`
}

type PickupChoice struct {
	Window   string `json:"window"`
	StartsAt string `json:"startsAt"`
}

type NearbyReservation struct {
	Customer  string `json:"customerRef"`
	ExpiresAt int64  `json:"expiresAt"`
}

type Cart struct {
	Origin      string             `json:"origin"`
	OriginRef   string             `json:"originRef"`
	MenuVersion int64              `json:"menuVersion"`
	Items       []Item             `json:"items"`
	Pickup      *PickupChoice      `json:"pickup,omitempty"`
	Nearby      *NearbyReservation `json:"nearby,omitempty"`
}

type reservationRejection struct {
	code  service.Code
	lines []Item
}

func (r *reservationRejection) Error() string { return string(r.code) }
func (r *reservationRejection) Unwrap() error {
	if r.code == service.OutOfStock {
		return ErrStock
	}
	return r.code
}

type Price struct {
	Version      int64                  `json:"version"`
	Expires      time.Time              `json:"expiresAt"`
	SKU          string                 `json:"sku"`
	Option       string                 `json:"optionKey"`
	Label        string                 `json:"label"`
	Asset        string                 `json:"asset"`
	UnitPrice    string                 `json:"unitPrice"`
	Available    int64                  `json:"available"`
	SKUAvailable int64                  `json:"skuAvailable"`
	Metadata     *publishedMenuMetadata `json:"metadata,omitempty"`
	Category     *CatalogCategory       `json:"category,omitempty"`
}

type OrderPayment struct {
	NearbyUnpaid  bool       `json:"nearbyUnpaid,omitempty"`
	Stage         string     `json:"stage"`
	ChainID       int        `json:"chainId"`
	RelayTxHash   *string    `json:"relayTxHash,omitempty"`
	TxHash        *string    `json:"txHash,omitempty"`
	IncludedAt    *time.Time `json:"includedAt,omitempty"`
	FinalizedAt   *time.Time `json:"finalizedAt,omitempty"`
	PickupReadyAt *time.Time `json:"pickupReadyAt,omitempty"`
	PickupStage   *string    `json:"pickupStage,omitempty"`
}

type MenuCheckout struct {
	ReviewKind   string  `json:"reviewKind,omitempty"`
	BranchLabel  string  `json:"branchLabel"`
	PayerBound   bool    `json:"payerBound"`
	CustomerRef  *string `json:"customerRef,omitempty"`
	FinalOutcome *string `json:"finalOutcome,omitempty"`
}

type CafeReview struct {
	BranchLabel string `json:"branchLabel"`
	PayerBound  bool   `json:"payerBound"`
}

type Order struct {
	ID           string            `json:"id"`
	Revision     int64             `json:"revision"`
	State        string            `json:"state"`
	Asset        string            `json:"asset"`
	Amount       string            `json:"amount"`
	Expires      time.Time         `json:"expiresAt"`
	PickupNo     *int64            `json:"pickupNo,omitempty"`
	ReceiptRef   *string           `json:"receiptRef,omitempty"`
	ReceiptKind  *string           `json:"receiptKind,omitempty"`
	QuoteDigest  *string           `json:"quoteDigest,omitempty"`
	OrderKey     *string           `json:"orderKey,omitempty"`
	Subscription *InvoiceLifecycle `json:"subscription,omitempty"`
	Payment      *OrderPayment     `json:"payment,omitempty"`
	MenuCheckout *MenuCheckout     `json:"menuCheckout,omitempty"`
	CafeReview   *CafeReview       `json:"cafeReview,omitempty"`
	accessEndsAt *time.Time
}

type ShopIdentity struct {
	Branch      string `json:"branch"`
	BranchLabel string `json:"branchLabel"`
}

func (s *Store) ShopIdentity(ctx context.Context, auth service.Bound) (*ShopIdentity, error) {
	var label *string
	err := s.db.QueryRow(ctx, "SELECT ledger.shop_branch_label($1,$2,$3)", auth.Tenant, auth.Request.Domain, auth.Subject).Scan(&label)
	if err != nil || label == nil {
		return nil, domainError(err)
	}
	return &ShopIdentity{Branch: auth.Request.Domain, BranchLabel: *label}, nil
}

func (s *Store) Catalog(ctx context.Context, auth service.Bound, page int32) ([]Price, error) {
	prices, _, err := s.catalogWindow(ctx, auth, page)
	return prices, err
}

func (s *Store) catalogWindow(ctx context.Context, auth service.Bound, page int32) ([]Price, *int64, error) {
	var version *int64
	var prices []Price
	err := s.db.QueryRow(ctx, "SELECT snapshot_version,items FROM ledger.catalog_category_window($1,$2,$3,$4)",
		auth.Tenant, auth.Request.Domain, auth.Subject, page).Scan(&version, &prices)
	return prices, version, domainError(err)
}

func (s *Store) Reserve(ctx context.Context, auth service.Bound, cart Cart) (string, error) {
	lines, err := json.Marshal(cart.Items)
	if err != nil {
		return "", service.BadRequest
	}
	if cart.Pickup != nil {
		var id string
		err := s.db.QueryRow(ctx, `SELECT ledger.reserve_travel_pickup($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)::text`,
			auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.IdempotencyKey, cart.OriginRef, cart.MenuVersion,
			auth.Request.Body["asset"], lines, cart.Pickup.Window, cart.Pickup.StartsAt).Scan(&id)
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "P0702" {
			return "", service.CatalogChanged
		}
		return id, domainError(err)
	}
	if cart.Origin == "menu" {
		var customer *string
		var expires *int64
		if cart.Nearby != nil {
			customer, expires = &cart.Nearby.Customer, &cart.Nearby.ExpiresAt
		}
		return scanReservation(s.db.QueryRow(ctx, `SELECT sale_ref::text,rejection_code,unavailable_lines
			FROM ledger.reserve_menu_with_outcome($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.IdempotencyKey, cart.OriginRef, cart.MenuVersion,
			auth.Request.Body["asset"], lines, auth.Role, customer, expires))
	}

	args := []any{auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.IdempotencyKey,
		cart.Origin, cart.OriginRef, cart.MenuVersion, auth.Request.Body["asset"], lines, auth.Role}
	var existing *string
	err = s.db.QueryRow(ctx, `SELECT ledger.resume_reserve_order($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)::text`, args...).Scan(&existing)
	if err != nil {
		return "", domainError(err)
	}
	if existing != nil {
		return *existing, nil
	}
	var id string
	err = s.db.QueryRow(ctx, `SELECT ledger.reserve_order($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)::text`, args...).Scan(&id)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "P0702" {
		return "", service.CatalogChanged
	}
	return id, domainError(err)
}

func scanReservation(row pgx.Row) (string, error) {
	var sale, code *string
	var lines []Item
	if err := row.Scan(&sale, &code, &lines); err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "P0502" {
			return "", &reservationRejection{code: service.Expired}
		}
		return "", domainError(err)
	}
	if code != nil {
		return "", &reservationRejection{code: service.Code(*code), lines: lines}
	}
	if sale == nil {
		return "", service.Unknown
	}
	return *sale, nil
}

func (s *Store) NearbyReservation(ctx context.Context, auth service.Bound, byOrder bool) (string, error) {
	var sale, code *string
	var lines []Item
	err := s.db.QueryRow(ctx, `SELECT sale_ref::text,rejection_code,unavailable_lines
		FROM ledger.nearby_menu_outcome($1,$2,$3,$4,$5,$6)`, auth.Tenant, auth.Request.Domain,
		auth.Subject, auth.Request.Body["customerRef"], auth.Request.Object, byOrder).Scan(&sale, &code, &lines)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", nil
	case err != nil:
		return "", domainError(err)
	case code != nil:
		return "", &reservationRejection{code: service.Code(*code), lines: lines}
	case sale != nil:
		return *sale, nil
	default:
		return "", service.Unknown
	}
}

func (s *Store) CancelPublicOrder(ctx context.Context, auth service.Bound) error {
	_, err := s.db.Exec(ctx, "SELECT ledger.cancel_public_menu_order($1,$2,$3,$4,$5)",
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object, auth.Request.IdempotencyKey)
	return domainError(err)
}

type CustomerOrderSummary struct {
	ID           string     `json:"id"`
	State        string     `json:"state"`
	Asset        string     `json:"asset"`
	Amount       string     `json:"amount"`
	AccessEndsAt *time.Time `json:"accessEndsAt,omitempty"`
}

func (s *Store) Orders(ctx context.Context, auth service.Bound, after string) ([]CustomerOrderSummary, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text,state,asset_ref,amount,access_ends_at
        FROM ledger.customer_order_rows($1,$2,$3,$4)`, auth.Tenant, auth.Request.Domain, auth.Subject, after)
	if err != nil {
		return nil, domainError(err)
	}
	defer rows.Close()
	var result []CustomerOrderSummary
	for rows.Next() {
		var order CustomerOrderSummary
		if err := rows.Scan(&order.ID, &order.State, &order.Asset, &order.Amount, &order.AccessEndsAt); err != nil {
			return nil, domainError(err)
		}
		result = append(result, order)
	}
	return result, domainError(rows.Err())
}

func (s *Store) Status(ctx context.Context, auth service.Bound) (Order, error) {
	var order Order
	var checkout *MenuCheckout
	err := s.db.QueryRow(ctx, `SELECT id::text,revision,state,asset_ref,amount::text,expires_at,pickup_no,receipt_ref::text,quote_digest,order_key,receipt_kind,subscription,payment,access_ends_at,
		CASE WHEN $5 IN ('terminal','customer') THEN ledger.menu_checkout_context($1,$2,$3,$4) END
		FROM ledger.commerce_order_receipt($1,$2,$3,$4)`, auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object, auth.Role).
		Scan(&order.ID, &order.Revision, &order.State, &order.Asset, &order.Amount, &order.Expires, &order.PickupNo, &order.ReceiptRef, &order.QuoteDigest, &order.OrderKey, &order.ReceiptKind, &order.Subscription, &order.Payment, &order.accessEndsAt, &checkout)
	if errors.Is(err, pgx.ErrNoRows) {
		err = service.Forbidden
	}
	if checkout != nil {
		if auth.Role == "customer" && checkout.ReviewKind == "cafe" {
			order.CafeReview = &CafeReview{BranchLabel: checkout.BranchLabel, PayerBound: checkout.PayerBound}
		} else if auth.Role == "terminal" {
			order.MenuCheckout = checkout
		}
	}
	return order, domainError(err)
}

type Ready struct {
	QuoteOrder
	Fence                            int64
	Ciphertext, Digest, IssuedSigner []byte
}

func (p Profile) Digest() [32]byte {
	h := sha256.New()
	h.Write([]byte("nu54-p05-profile-v2"))
	h.Write(binary.BigEndian.AppendUint64(nil, p.Chain))
	for _, addr := range []common.Address{p.Contract, p.Asset, p.Beneficiary, p.Signer} {
		h.Write(addr.Bytes())
	}
	h.Write(p.MerchantID[:])
	h.Write(binary.BigEndian.AppendUint64(nil, p.KeyVersion))
	h.Write(binary.BigEndian.AppendUint64(nil, p.BeneficiaryVersion))
	keyCommitment := sha256.Sum256(p.CommitmentKey[:])
	h.Write(keyCommitment[:])
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func (s *Store) BeginQuote(ctx context.Context, auth service.Bound, claim [32]byte, expected int64, profile Profile) (Ready, error) {
	quote, err := freshId()
	if err != nil {
		return Ready{}, err
	}
	order, err := freshId()
	if err != nil {
		return Ready{}, err
	}
	nonce, err := freshId()
	if err != nil {
		return Ready{}, err
	}
	profileDigest := profile.Digest()
	ready := Ready{QuoteOrder: QuoteOrder{Tenant: auth.Tenant, Branch: auth.Request.Domain, Sale: auth.Request.Object, Claim: claim}}
	var asset string
	var qid, oid, rawNonce, payer, intent []byte
	commandDigest, err := hex.DecodeString(auth.IntentDigest)
	if err != nil {
		return Ready{}, service.BadRequest
	}
	var gen int64
	var role int16
	err = s.db.QueryRow(ctx, `SELECT quote_id,order_id,nonce,payer,payer_generation,payer_role,issued_at,expires_at,
		fence,payload_cipher,signed_digest,intent_digest,amount,asset,issued_signer
		FROM ledger.prepare_quote($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object, expected, auth.Request.IdempotencyKey, commandDigest, claim[:], profileDigest[:], profile.Signer.Bytes(),
		quote[:], order[:], nonce[:]).Scan(&qid, &oid, &rawNonce, &payer, &gen, &role,
		&ready.Issued, &ready.Expires, &ready.Fence, &ready.Ciphertext, &ready.Digest, &intent, &ready.Amount, &asset, &ready.IssuedSigner)
	if err != nil {
		return Ready{}, domainError(err)
	}
	if len(ready.Ciphertext) == 0 && (!common.IsHexAddress(asset) || common.HexToAddress(asset) != profile.Asset) {
		return Ready{}, service.Mismatch
	}
	copy(ready.QuoteID[:], qid)
	copy(ready.OrderID[:], oid)
	copy(ready.Nonce[:], rawNonce)
	copy(ready.Intent[:], intent)
	ready.Payer, ready.PayerGeneration, ready.PayerRole = common.BytesToAddress(payer), uint64(gen), byte(role)
	return ready, nil
}

func (s *Store) FinishQuote(ctx context.Context, auth service.Bound, ready Ready, profile Profile, ciphertext []byte, digest [32]byte) error {
	var ok bool
	profileDigest := profile.Digest()
	err := s.db.QueryRow(ctx, "SELECT ledger.finish_quote($1,$2,$3,$4,$5,$6,$7,$8,$9)",
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object, ready.Claim[:], profileDigest[:], ready.Fence,
		ciphertext, digest[:]).Scan(&ok)
	if err == nil && !ok {
		return service.Unknown
	}
	return domainError(err)
}

func domainError(err error) error {
	if err == nil || service.CodeOf(err) != "" {
		return err
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "P0761":
			return service.CheckoutConflict
		case "P0501", "23505":
			return service.Conflict
		case "P0502", "P0702":
			return service.Expired
		case "P0701":
			return ErrStock
		case "P0703", "55P03", "40P01", "40001":
			return service.Busy
		case "42501":
			return service.Forbidden
		case "23514", "22P02", "22003":
			return service.BadRequest
		}
	}
	return service.Unknown
}

var ErrStock = errors.New("OUT_OF_STOCK")

func (s *Store) ReclaimExpired(ctx context.Context) (bool, error) {
	var reclaimed bool
	err := s.db.QueryRow(ctx, "SELECT ledger.reclaim_expired_order()").Scan(&reclaimed)
	return reclaimed, domainError(err)
}

func (s *Store) Scope(ctx context.Context, req service.Request, principal service.Principal) (bool, error) {
	var allowed bool
	var claim []byte
	if req.Operation == "quote.issue" {
		claim, _ = hex.DecodeString(req.Body["claimDigest"])
	}
	err := s.db.QueryRow(ctx, "SELECT ledger.c3_scope($1,$2,$3,$4,$5,$6,$7)",
		principal.Tenant, req.Domain, principal.Subject, principal.Role, req.Object, req.Operation, claim).Scan(&allowed)
	return allowed, domainError(err)
}

type Claim struct {
	Digest    string `json:"claimDigest"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (s *Store) AuthorizeCheckout(ctx context.Context, auth service.Bound) (Claim, error) {
	random, err := freshId()
	if err != nil {
		return Claim{}, service.Unknown
	}
	intent, err := hex.DecodeString(auth.IntentDigest)
	if err != nil {
		return Claim{}, service.BadRequest
	}
	var claim []byte
	var expiry time.Time
	err = s.db.QueryRow(ctx, `SELECT claim_digest,expires_at FROM ledger.authorize_checkout($1,$2,$3,$4,$5,$6,$7,$8)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object, auth.Request.Body["customerRef"],
		auth.Request.IdempotencyKey, intent, random[:]).Scan(&claim, &expiry)
	if err != nil {
		return Claim{}, domainError(err)
	}
	return Claim{Digest: hex.EncodeToString(claim), ExpiresAt: expiry.Unix()}, nil
}

type TerminalMode struct {
	Mode     string `json:"mode"`
	Revision int64  `json:"revision"`
}

type StaffAccess struct {
	BranchRef   string   `json:"branchRef"`
	ActorRef    string   `json:"actorRef"`
	Permissions []string `json:"permissions"`
	Console     string   `json:"console,omitempty"`
}

func (s *Store) StaffAccess(ctx context.Context, auth service.Bound) (StaffAccess, error) {
	var out StaffAccess
	if auth.Request.Object != auth.Subject {
		return out, service.Forbidden
	}
	err := s.db.QueryRow(ctx, `SELECT ledger.kiosk_staff_access($1,$2,$3,$4)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Role).Scan(&out)
	return out, domainError(err)
}

func (s *Store) TerminalOrdersPage(ctx context.Context, auth service.Bound, revision int64) ([]string, error) {
	var out []string
	args := auth.Request.Body
	err := s.db.QueryRow(ctx, `SELECT ledger.terminal_orders_page($1,$2,$3,$4,$5,$6,$7,$8)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object,
		args["kind"], args["scopeRef"], revision, args["afterSale"]).Scan(&out)
	return out, domainError(err)
}

type TerminalTransition struct {
	RequestRef     string `json:"requestRef"`
	SourceRevision int64  `json:"sourceRevision"`
	TargetRevision int64  `json:"targetRevision"`
	TargetMode     string `json:"targetMode"`
	Handoff        bool   `json:"handoff"`
	Ack            bool   `json:"ack"`
	State          string `json:"state"`
}

type TerminalSettings struct {
	TerminalMode
	OrderCount *int                `json:"orderCount,omitempty"`
	OrderRefs  []string            `json:"orderRefs,omitempty"`
	Transition *TerminalTransition `json:"transition,omitempty"`
}

func (s *Store) TerminalState(ctx context.Context, auth service.Bound) (TerminalSettings, error) {
	var out TerminalSettings
	err := s.db.QueryRow(ctx, `SELECT ledger.terminal_transition_state($1,$2,$3,$4,$5,NULL)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object, auth.Role).Scan(&out)
	return out, domainError(err)
}

func (s *Store) ChangeTerminalMode(ctx context.Context, auth service.Bound, expected int64) (TerminalSettings, error) {
	var out TerminalSettings
	intent, err := hex.DecodeString(auth.IntentDigest)
	if err != nil {
		return out, service.BadRequest
	}
	err = s.db.QueryRow(ctx, `SELECT ledger.request_terminal_transition($1,$2,$3,$4,$5,$6,$7,$8)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object, auth.Request.IdempotencyKey,
		expected, auth.Request.Body["mode"], intent).Scan(&out)
	return out, domainError(err)
}

func (s *Store) ConfirmTerminalTransition(ctx context.Context, auth service.Bound, revision int64) (TerminalSettings, error) {
	var out TerminalSettings
	err := s.db.QueryRow(ctx, `SELECT ledger.confirm_terminal_transition($1,$2,$3,$4,$5,$6,$7)`,
		auth.Tenant, auth.Request.Domain, auth.Subject, auth.Request.Object, auth.Role,
		auth.Request.Body["transitionRef"], revision).Scan(&out)
	return out, domainError(err)
}

func (s *Store) RetireTerminalReceipts(ctx context.Context) error {
	_, err := s.db.Exec(ctx, "SELECT ledger.expire_terminal_receipts(),ledger.expire_operator_catalog_receipts(),ledger.retire_menu_reservation_details()")
	return domainError(err)
}
