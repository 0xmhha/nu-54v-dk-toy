package commerce

import (
	"context"
	"crypto/cipher"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type protectedCloseRequest struct {
	Tenant, Branch, Actor, Device string
	Generation                    int64
	Key, Review, RecoveryReview   string
}

func (r protectedCloseRequest) arguments(tail ...any) []any {
	return append([]any{r.Tenant, r.Branch, r.Actor, r.Device, r.Generation}, tail...)
}

type protectedClosePlan struct {
	ScopeDigest, ProofKey, ReviewRef, LegacyScopeDigest, LegacyReviewRef               string
	Account, Destination, Registration, Measurement, Deployment, Settlement, Challenge string
	Stream, StreamGeneration, Assessment, Hash                                         string
	Height, ExpiresAt                                                                  int64
}

type protectedCloseCandidate struct {
	State   string
	Hash    common.Hash
	Nonce   uint64
	Account common.Address
	Signer  common.Address
	sealed  []byte
}

type protectedCloseStore interface {
	recover(context.Context, protectedCloseRequest) (protectedCloseCandidate, error)
	plan(context.Context, protectedCloseRequest, [32]byte) (protectedClosePlan, error)
	admit(context.Context, protectedCloseRequest, [32]byte, protectedClosePlan, int64, common.Address, protectedCloseCandidate) (bool, error)
	mark(context.Context, protectedCloseRequest, [32]byte, protectedClosePlan, int64, common.Hash, string) (bool, error)
}

type protectedCloseChain interface {
	AccountOf(context.Context, common.Address) (core.Account, error)
	PrepareRentalClose(context.Context, *ecdsa.PrivateKey, common.Address) (core.RentalCloseTransaction, error)
	InspectRentalClose(context.Context, common.Address, common.Address, core.RentalCloseTransaction) (core.RentalCloseChainState, error)
	PublishRentalClose(context.Context, common.Address, common.Address, core.RentalCloseTransaction) error
}

type protectedCloseAnchor interface {
	ConfirmCloseAnchor(context.Context, rentalv1.CloseAnchorRequest) (rentalv1.CloseAnchorConfirmation, error)
	CallerFingerprint() [32]byte
}

type protectedCloseExecutor struct {
	store      protectedCloseStore
	chain      protectedCloseChain
	anchor     protectedCloseAnchor
	cipher     cipher.AEAD
	operator   *ecdsa.PrivateKey
	deployment core.Deployment
	manifest   [32]byte
	now        func() time.Time
}

func openProtectedCloseExecutor(ctx context.Context, dsn string, chain *core.Chain, anchor *core.RentalEvidenceReader,
	seal cipher.AEAD, operator *ecdsa.PrivateKey, deployment core.Deployment, manifest [32]byte) (*protectedCloseExecutor, error) {
	if chain == nil || anchor == nil || anchor.CallerFingerprint() == ([32]byte{}) || seal == nil ||
		seal.NonceSize() != 0 || operator == nil || deployment.ChainID != 8283 || manifest == ([32]byte{}) {
		return nil, errors.New("protected rental close requires a registered signer, authority and sealed storage")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid protected close database configuration")
	}
	config.ConnConfig.RuntimeParams["lock_timeout"] = "2s"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "7s"
	config.ConnConfig.RuntimeParams["jit"] = "off" // see commerce.Open
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("protected close database unavailable")
	}
	var scoped bool
	err = pool.QueryRow(ctx, `SELECT session_user=current_user AND pg_has_role(current_user,'nu54_p05_rental_close','USAGE')
		AND NOT EXISTS(SELECT FROM pg_roles WHERE rolname IN ('nu54_p05_commerce','nu54_p05_rental_authority',
			'nu54_p05_rental_authority_read','nu54_p05_rental_evidence','nu54_p05_rental_reconcile') AND pg_has_role(current_user,oid,'MEMBER'))
		AND NOT (SELECT rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls FROM pg_roles WHERE rolname=current_user)
		AND NOT has_schema_privilege('ledger','CREATE')
		AND NOT has_table_privilege('ledger.rental_close_candidate','SELECT,INSERT,UPDATE,DELETE,TRUNCATE')`).Scan(&scoped)
	if err != nil || !scoped {
		pool.Close()
		return nil, errors.New("dedicated protected close execution login required")
	}
	return &protectedCloseExecutor{store: &postgresProtectedCloseStore{db: pool}, chain: chain, anchor: anchor,
		cipher: seal, operator: operator, deployment: deployment, manifest: manifest, now: time.Now}, nil
}

func (e *protectedCloseExecutor) execute(ctx context.Context, req protectedCloseRequest) (protectedCloseCandidate, error) {
	candidate, err := e.store.recover(ctx, req)
	if err != nil {
		return candidate, err
	}
	if candidate.State == "confirmed" || candidate.State == "reverted" || candidate.State == "superseded" {
		return candidate, nil
	}
	signer := crypto.PubkeyToAddress(e.operator.PublicKey)
	transaction := core.RentalCloseTransaction{Hash: candidate.Hash, Nonce: candidate.Nonce}
	if candidate.State != "" {
		state, err := e.chain.InspectRentalClose(ctx, candidate.Signer, candidate.Account, transaction)
		if err != nil {
			return candidate, service.Unknown
		}
		switch state {
		case core.RentalCloseSubmitted, core.RentalCloseConfirmed, core.RentalCloseReverted, core.RentalCloseSuperseded:
			if candidate.State == string(state) {
				return candidate, nil
			}
			return e.finish(ctx, req, candidate, string(state))
		case core.RentalCloseNotObserved:
			if candidate.Signer != signer {
				return candidate, service.Conflict
			}
		default:
			return candidate, service.Unknown
		}
	}
	writer := e.anchor.CallerFingerprint()
	plan, err := e.store.plan(ctx, req, writer)
	if err != nil {
		return candidate, err
	}
	if req.RecoveryReview != "" && (candidate.State == "" || req.RecoveryReview != plan.ReviewRef) {
		return candidate, service.Conflict
	}
	if req.RecoveryReview == "" && candidate.State != "" && plan.LegacyReviewRef == req.Review && plan.LegacyScopeDigest != "" {
		// Existing originals use their stored historical approval. SQL checks
		// that the stored digest still matches before granting a send lease.
		plan.ReviewRef, plan.ScopeDigest = plan.LegacyReviewRef, plan.LegacyScopeDigest
	}
	if (req.RecoveryReview == "" && plan.ReviewRef != req.Review) || plan.ExpiresAt <= e.now().Unix() ||
		plan.Deployment != common.Hash(e.manifest).Hex() || plan.Settlement != strings.ToLower(e.deployment.Settlement.Hex()) {
		return candidate, service.Conflict
	}
	account := common.HexToAddress(plan.Account)
	if candidate.State != "" && candidate.Account != account {
		return candidate, service.Conflict
	}
	if candidate.State == "" || req.RecoveryReview != "" {
		view, err := e.chain.AccountOf(ctx, account)
		if err != nil || !view.Exists || view.ClosedAt != 0 || view.WithdrawAddress != common.HexToAddress(plan.Destination) {
			return candidate, service.Conflict
		}
	}

	lease, err := e.confirm(ctx, req, plan)
	if err != nil {
		return candidate, err
	}
	if candidate.State == "" {
		transaction, err = e.chain.PrepareRentalClose(ctx, e.operator, account)
		if err != nil {
			return candidate, service.Unknown
		}
		defer clear(transaction.Signed)
		lease, err = e.confirm(ctx, req, plan)
		if err != nil {
			return candidate, err
		}
		if len(transaction.Signed) == 0 || transaction.Hash == (common.Hash{}) {
			return candidate, service.Unknown
		}
		candidate = protectedCloseCandidate{State: "prepared", Hash: transaction.Hash, Nonce: transaction.Nonce, Account: account, Signer: signer,
			sealed: e.cipher.Seal(nil, nil, transaction.Signed, closeCandidateContext(req, transaction.Hash))}
		won, err := e.store.admit(ctx, req, writer, plan, lease.ExpiresAt, signer, candidate)
		if err != nil {
			return protectedCloseCandidate{}, err
		}
		if !won {
			return e.store.recover(ctx, req)
		}
	}
	if req.RecoveryReview != "" {
		reauthorizer, ok := e.store.(interface {
			reauthorize(context.Context, protectedCloseRequest, [32]byte, protectedClosePlan, int64, common.Hash) (bool, error)
		})
		if !ok {
			return candidate, service.Conflict
		}
		allowed, err := reauthorizer.reauthorize(ctx, req, writer, plan, lease.ExpiresAt, candidate.Hash)
		if err != nil {
			return candidate, err
		}
		if !allowed {
			return candidate, service.Conflict
		}
	}
	ok, err := e.store.mark(ctx, req, writer, plan, lease.ExpiresAt, candidate.Hash, "attempting")
	if err != nil {
		return candidate, err
	}
	if !ok {
		return e.store.recover(ctx, req)
	}
	candidate.State = "attempting"
	raw, err := e.cipher.Open(nil, nil, candidate.sealed, closeCandidateContext(req, candidate.Hash))
	if err != nil {
		return candidate, service.Unknown
	}
	defer clear(raw)
	transaction.Signed = raw
	expires := time.Unix(lease.ExpiresAt, 0)
	if !expires.After(e.now()) {
		return candidate, service.Conflict
	}
	sendCtx, cancel := context.WithDeadline(ctx, expires)
	defer cancel()
	if err := e.chain.PublishRentalClose(sendCtx, signer, account, transaction); err != nil {
		// Timeout and provider refusal are ambiguous until the original hash is observed.
		return candidate, nil
	}
	return e.finish(ctx, req, candidate, "submitted")
}

func (e *protectedCloseExecutor) finish(ctx context.Context, req protectedCloseRequest, candidate protectedCloseCandidate, state string) (protectedCloseCandidate, error) {
	_, err := e.store.mark(ctx, req, [32]byte{}, protectedClosePlan{}, 0, candidate.Hash, state)
	if err != nil {
		return candidate, err
	}
	return e.store.recover(ctx, req)
}

func closeCandidateContext(req protectedCloseRequest, hash common.Hash) []byte {
	return []byte("nu54-rental-close-v1:" + req.Tenant + ":" + req.Branch + ":" + req.Actor + ":" + req.Device + ":" +
		strconv.FormatInt(req.Generation, 10) + ":" + req.Key + ":" + req.Review + ":" + hash.Hex())
}

type postgresProtectedCloseStore struct{ db *pgxpool.Pool }

func (s *postgresProtectedCloseStore) recover(ctx context.Context, r protectedCloseRequest) (protectedCloseCandidate, error) {
	var c protectedCloseCandidate
	var hash []byte
	var nonce, account, signer string
	err := s.db.QueryRow(ctx, `SELECT state,tx_hash,sealed_transaction,nonce::text,account_ref,signer
		FROM ledger.rental_close_recover($1,$2,$3,$4,$5,$6,$7)`, r.arguments(r.Key, r.Review)...).Scan(&c.State, &hash, &c.sealed, &nonce, &account, &signer)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	c.Nonce, err = strconv.ParseUint(nonce, 10, 64)
	if err != nil || len(hash) != 32 {
		return protectedCloseCandidate{}, service.Unknown
	}
	c.Hash = common.Hash(hash)
	c.Account = common.HexToAddress(account)
	c.Signer = common.HexToAddress(signer)
	return c, nil
}

func (s *postgresProtectedCloseStore) plan(ctx context.Context, r protectedCloseRequest, writer [32]byte) (protectedClosePlan, error) {
	var p protectedClosePlan
	var raw []byte
	err := s.db.QueryRow(ctx, `SELECT ledger.rental_close_live_scope($1,$2,$3,$4,$5,$6)`, r.arguments(writer[:])...).Scan(&raw)
	if err != nil {
		return p, err
	}
	if json.Unmarshal(raw, &p) != nil {
		return protectedClosePlan{}, service.Unknown
	}
	return p, nil
}

func (s *postgresProtectedCloseStore) admit(ctx context.Context, r protectedCloseRequest, writer [32]byte, p protectedClosePlan,
	lease int64, signer common.Address, c protectedCloseCandidate) (bool, error) {
	var saved bool
	digest, err := hex.DecodeString(p.ScopeDigest)
	if err != nil || len(digest) != 32 {
		return false, service.Conflict
	}
	err = s.db.QueryRow(ctx, `SELECT ledger.rental_close_admit($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::text::numeric,$13,$14)`,
		r.arguments(r.Key, r.Review, writer[:], digest, lease, strings.ToLower(signer.Hex()), strconv.FormatUint(c.Nonce, 10), c.Hash[:], c.sealed)...).Scan(&saved)
	return saved, err
}

func (s *postgresProtectedCloseStore) mark(ctx context.Context, r protectedCloseRequest, writer [32]byte, p protectedClosePlan,
	lease int64, hash common.Hash, next string) (bool, error) {
	digest, err := hex.DecodeString(p.ScopeDigest)
	if err != nil {
		return false, service.Conflict
	}
	var saved bool
	err = s.db.QueryRow(ctx, `SELECT ledger.rental_close_mark($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		r.arguments(r.Key, r.Review, hash[:], next, writer[:], digest, lease)...).Scan(&saved)
	return saved, err
}

func (e *protectedCloseExecutor) confirm(ctx context.Context, req protectedCloseRequest, plan protectedClosePlan) (rentalv1.CloseAnchorConfirmation, error) {
	expected := rentalAnchorRequest(req, e.deployment, plan)
	confirmed, err := e.anchor.ConfirmCloseAnchor(ctx, expected)
	if err != nil || !confirmed.ValidFor(expected, e.now()) {
		return rentalv1.CloseAnchorConfirmation{}, service.Conflict
	}
	return confirmed, nil
}

func rentalAnchorRequest(req protectedCloseRequest, deployment core.Deployment, plan protectedClosePlan) rentalv1.CloseAnchorRequest {
	binding := rentalCollectorBinding(req.Branch, req.Device, req.Generation, deployment, plan.Account,
		common.FromHex(plan.Registration), common.FromHex(plan.Measurement), common.FromHex(plan.Deployment))
	return rentalv1.CloseAnchorRequest{Stream: plan.Stream, StreamGeneration: plan.StreamGeneration, Branch: req.Branch, Board: req.Device,
		Generation: strconv.FormatInt(req.Generation, 10), Challenge: plan.Challenge, RegistrationBinding: crypto.Keccak256Hash(binding[:]).Hex(),
		Previous: rentalv1.Anchor{Height: plan.Height, Hash: plan.Hash}, ReviewDigest: plan.Assessment, ValidUntil: plan.ExpiresAt}
}

func (s *postgresProtectedCloseStore) reauthorize(ctx context.Context, r protectedCloseRequest, writer [32]byte, p protectedClosePlan,
	lease int64, hash common.Hash) (bool, error) {
	digest, err := hex.DecodeString(p.ScopeDigest)
	if err != nil || len(digest) != 32 {
		return false, service.Conflict
	}
	var allowed bool
	err = s.db.QueryRow(ctx, "SELECT ledger.rental_close_reauthorize($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)",
		r.arguments(r.Key, r.Review, r.RecoveryReview, hash[:], writer[:], digest, lease)...).Scan(&allowed)
	return allowed, err
}
