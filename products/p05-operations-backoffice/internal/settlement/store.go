package settlement

import (
	"context"
	"crypto/cipher"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/authority"
	"math/big"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ db authority.Pool }

func Open(ctx context.Context, url, capability string) (*Store, error) {
	c, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid settlement data source")
	}
	c.ConnConfig.RuntimeParams["lock_timeout"] = "2s"
	c.ConnConfig.RuntimeParams["statement_timeout"] = "5s"
	db, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, errors.New("could not connect to settlement data source")
	}
	var safe bool
	err = db.QueryRow(ctx, `SELECT pg_has_role(current_user,$1,'USAGE') AND NOT has_schema_privilege('ledger','CREATE')
		AND NOT EXISTS (SELECT FROM unnest(ARRAY['nu54_p05_finance','nu54_p05_settlement','nu54_p05_dispatch',
		    'nu54_p05_commerce','nu54_p05_staff','nu54_p05_receiver','nu54_p05_print','nu54_p05_print_staff','nu54_p05_payout_worker']) capability WHERE capability<>$1 AND pg_has_role(current_user,capability,'MEMBER'))
		AND NOT has_table_privilege('ledger.payment','INSERT,UPDATE,DELETE')
		AND NOT has_table_privilege('ledger.quote_work','SELECT')
		AND NOT (SELECT rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb FROM pg_roles WHERE rolname=current_user)`,
		capability).Scan(&safe)
	if err != nil || !safe {
		db.Close()
		return nil, errors.New("settlement requires a migrated least-privilege database login")
	}
	return &Store{db: authority.Pool{Pool: db}}, nil
}

func (s *Store) Close() { s.db.Close() }

func quoteAAD(branch, sale string) []byte {
	return []byte("nu54-quote-store-v2:" + branch + ":" + sale)
}
func approvalAAD(sale string) []byte  { return []byte("nu54-payer-approval-v2:" + sale) }
func relayAAD(id, hash string) []byte { return []byte("nu54-relay-signed-v2:" + id + ":" + hash) }

type Forwarder struct {
	GasSponsors  commerce.GasSponsorLookup
	RentalEscrow commerce.RentalEscrowAuthority
	Store        *Store
	QuoteCipher  cipher.AEAD
	RelayCipher  cipher.AEAD
	Targets      map[string]Target
	MerchantIDs  map[string]common.Hash
	// Faucet is nil where no store offers test tokens. GasAccount is shown to an operator.
	Faucet     *Faucet
	GasAccount common.Address
	// Paused are the branches whose deployment or token was refused at start.
	Paused map[string]bool
}

func (f Forwarder) verifiedQuote(ctx context.Context, req service.Request, tenant, actor string) (Quote, Target, string, error) {
	var ciphertext, digest, signer []byte
	var state string
	err := f.Store.db.QueryRow(ctx, "SELECT * FROM ledger.forward_quote($1,$2,$3,$4)",
		tenant, req.Domain, actor, req.Object).Scan(&ciphertext, &digest, &signer, &state)
	if err != nil {
		return Quote{}, Target{}, "", sqlError(err)
	}
	target, ok := f.Targets[req.Domain]
	if !ok {
		return Quote{}, Target{}, "", service.Capability
	}
	raw, err := f.QuoteCipher.Open(nil, nil, ciphertext, quoteAAD(req.Domain, req.Object))
	if err != nil {
		return Quote{}, Target{}, "", service.Mismatch
	}
	quote, err := unpackQuote(raw, digest, signer, target)
	if err != nil || rawHex(quote.Digest[:]) != req.Body["quoteDigest"] {
		return Quote{}, Target{}, "", service.Mismatch
	}
	return quote, target, state, nil
}

func (f Forwarder) verifiedApproval(ctx context.Context, req service.Request, tenant, actor string, signature []byte) (Quote, Target, string, error) {
	quote, target, state, err := f.verifiedQuote(ctx, req, tenant, actor)
	if err != nil {
		return Quote{}, Target{}, "", err
	}
	// An escrow branch takes only order.lock; a plain payment approval cannot pay it.
	if target.Escrow() {
		return Quote{}, Target{}, "", service.Capability
	}
	if rawHex(quote.ApprovalHash[:]) != req.Body["approvalDigest"] || !matchesSigner(quote.ApprovalHash, signature, quote.Args.Payer) {
		return Quote{}, Target{}, "", service.Mismatch
	}
	return quote, target, state, nil
}

func (f Forwarder) AuthorizeForward(ctx context.Context, req service.Request, principal service.Principal, signature []byte) (*service.OrderApproval, error) {
	quote, _, _, err := f.verifiedApproval(ctx, req, principal.Tenant, principal.Subject, signature)
	if err != nil {
		return nil, err
	}
	return &service.OrderApproval{QuoteDigest: rawHex(quote.Digest[:]), ApprovalDigest: rawHex(quote.ApprovalHash[:]),
		ValidUntil: min(req.ExpiresAt, principal.ExpiresAt)}, nil
}

func (f Forwarder) Forward(ctx context.Context, bound service.Bound, signature []byte) (string, error) {
	quote, target, state, err := f.verifiedApproval(ctx, bound.Request, bound.Tenant, bound.Subject, signature)
	if err != nil {
		return "", err
	}
	return f.admit(ctx, bound, quote, target, state, quote.ApprovalHash, signature)
}

// admit stores one verified payer authorization for the relay worker, with its gas sponsorship.
func (f Forwarder) admit(ctx context.Context, bound service.Bound, quote Quote, target Target, state string,
	approval common.Hash, payload []byte) (string, error) {
	req := bound.Request
	intent, err := hex.DecodeString(bound.IntentDigest)
	if err != nil || len(intent) != 32 {
		return "", service.BadRequest
	}
	sealed := f.RelayCipher.Seal(nil, nil, payload, approvalAAD(req.Object))
	args := []any{bound.Tenant, req.Domain, bound.Subject, req.Object, req.IdempotencyKey, intent, quote.Digest[:],
		approval[:], target.Stream, sealed}
	statement := `SELECT ledger.store_forward($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)::text`
	cafe := false
	if bound.Role == "customer" && f.GasSponsors != nil {
		// This existing read also works against older schemas, which never
		// project reviewKind for an authenticated customer.
		if err := f.Store.db.QueryRow(ctx, `SELECT coalesce(ledger.menu_checkout_context($1,$2,$3,$4)->>'reviewKind'='cafe',false)`,
			bound.Tenant, req.Domain, bound.Subject, req.Object).Scan(&cafe); err != nil {
			return "", sqlError(err)
		}
	}
	if bound.Role == "terminal" || cafe {
		var currentPublication func(time.Time) bool
		var grant []byte
		var until *time.Time
		// Existing accepted submissions recover from their stored receipt.
		if state == "issued" && f.GasSponsors != nil {
			profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: quote.Args.Asset, MerchantID: quote.Args.MerchantId}
			if witness, available := f.GasSponsors(ctx, req.Domain, profile, quote.Args.Payer, time.Now()); available {
				grant = witness.Grant[:]
				currentPublication = witness.CurrentPublication
				end := time.Unix(witness.ExpiresAt, 0)
				until = &end
			}
		}
		if cafe {
			// The SQL result distinguishes a new pinned admission from
			// historical recovery, which remains available after withdrawal.
			var result string
			var admitted bool
			var admissionUntil *time.Time
			dest := []any{&result, &admitted}
			escrowSQL := `SELECT sale_ref::text,new_sponsored_admission
				FROM ledger.store_customer_forward($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`
			escrowArgs := append(args, grant, until)
			var requiresV2 bool
			if state == "issued" && quote.Args.PayerRole == 2 && quote.Args.Mode == 1 && f.RentalEscrow != nil {
				if err := f.Store.db.QueryRow(ctx, `SELECT requires_v2 FROM ledger.renter_payment_scope($1,$2,$3,$4,$5,NULL,false)`,
					bound.Tenant, req.Domain, bound.Subject, req.Object, quote.Digest[:]).Scan(&requiresV2); err != nil {
					return "", sqlError(err)
				}
			}
			if requiresV2 {
				manifest, escrowUntil, escrowChecked := f.RentalEscrow.Check(ctx, req.Domain, quote.Args.Payer,
					quote.Args.PayerGeneration, quote.Args.Amount)
				var liveUntil *time.Time
				if escrowChecked {
					liveUntil = &escrowUntil
				}
				escrowSQL = `SELECT sale_ref::text,new_sponsored_admission,admission_until
					FROM ledger.store_customer_escrow_forward_bounded($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`
				escrowArgs = append(escrowArgs, manifest, liveUntil)
				dest = append(dest, &admissionUntil)
			}
			err := f.Store.db.QueryRowBeforeCommit(ctx, func() error {
				if admitted && ((currentPublication == nil || !currentPublication(time.Now())) ||
					(requiresV2 && (admissionUntil == nil || !time.Now().Before(*admissionUntil)))) {
					return service.Capability
				}
				return nil
			}, escrowSQL, escrowArgs...).Scan(dest...)
			if err != nil {
				return "", sqlError(err)
			}
			return result, nil
		}
		statement = `SELECT ledger.store_terminal_forward($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)::text`
		args = append(args, f.GasSponsors != nil, grant, until)
	}
	var result string
	err = f.Store.db.QueryRow(ctx, statement, args...).Scan(&result)
	return result, sqlError(err)
}

type work struct {
	id, branch, sale      string
	fence                 int64
	refund                *string
	refundKey             []byte
	refundAmount          *string
	quote, digest, signer []byte
	approval, approvalSum []byte
	stream                *string
	nonce, hash, state    *string
	expires               *time.Time
	intent, raw           []byte
	action                *escrowAction // set for a job after the lock
}

func (s *Store) claim(ctx context.Context) (work, error) {
	var w work
	err := s.db.QueryRow(ctx, `SELECT work_id::text,fence,branch_id::text,sale_id::text,refund_id::text,refund_key,
		refund_amount,quote_cipher,quote_digest,merchant_signer,approval_cipher,approval_digest,stream_id::text,
		relay_nonce,relay_expiry,relay_intent,relay_hash,relay_cipher,relay_state FROM ledger.claim_settlement_work()`).
		Scan(&w.id, &w.fence, &w.branch, &w.sale, &w.refund, &w.refundKey, &w.refundAmount, &w.quote, &w.digest, &w.signer,
			&w.approval, &w.approvalSum, &w.stream, &w.nonce, &w.expires, &w.intent, &w.hash, &w.raw, &w.state)
	return w, err
}

func (s *Store) reserve(ctx context.Context, w work, sender string, pending uint64, digest [32]byte, expiry time.Time) (uint64, error) {
	var nonce string
	err := s.db.QueryRow(ctx, `SELECT ledger.reserve_relay($1,$2,$3,$4::text::numeric,$5,$6)`,
		w.id, w.fence, sender, strconv.FormatUint(pending, 10), digest[:], expiry).Scan(&nonce)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(nonce, 10, 64)
}

func (s *Store) save(ctx context.Context, w work, intent [32]byte, hash string, cipher []byte) error {
	_, err := s.db.Exec(ctx, "SELECT ledger.store_signed_relay($1,$2,$3,$4,$5)", w.id, w.fence, intent[:], hash, cipher)
	return err
}

func (s *Store) deferWork(ctx context.Context, w work, state *string, finished bool) error {
	_, err := s.db.Exec(ctx, "SELECT ledger.defer_settlement($1,$2,$3,$4)", w.id, w.fence, state, finished)
	return err
}

func lowS(signature []byte) bool {
	r, s := new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:64])
	return crypto.ValidateSignatureValues(signature[64], r, s, true)
}

func sqlError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, service.Forbidden) {
		return service.Forbidden
	}
	if errors.Is(err, service.Capability) {
		return service.Capability
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "42501":
			return service.Forbidden
		case "P1001", "23505":
			return service.Conflict
		case "P1050":
			return service.Capability
		case "P1002":
			return service.Expired
		case "P1060":
			return service.CheckoutConflict
		case "P1063":
			return service.OutOfStock
		case "P1061":
			return service.Mismatch
		case "P1062":
			return service.BadRequest
		case "P1003", "P1030":
			return service.Conflict
		case "P1004", "55P03":
			return service.Busy
		}
	}
	return fmt.Errorf("%w: settlement storage unavailable", service.Unknown)
}
