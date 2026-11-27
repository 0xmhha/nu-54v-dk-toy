package commerce

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RentalProofWriter struct {
	db                  *pgxpool.Pool
	reader              *core.RentalEvidenceReader
	deployment          core.Deployment
	manifest            [32]byte
	operator            common.Address
	pendingOrderOrigins bool
}

func OpenRentalProofWriter(ctx context.Context, dsn string, reader *core.RentalEvidenceReader, deployment core.Deployment,
	manifest [32]byte, operator common.Address) (*RentalProofWriter, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil || reader == nil || deployment.ChainID != 8283 || manifest == [32]byte{} || operator == (common.Address{}) {
		return nil, errors.New("invalid protected rental writer settings")
	}
	config.ConnConfig.RuntimeParams["lock_timeout"] = "2s"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "7s"
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("protected rental evidence connection failed")
	}
	var allowed bool
	err = db.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p05_rental_evidence','USAGE')
		AND NOT pg_has_role(current_user,'nu54_p05_commerce','MEMBER')
		AND NOT pg_has_role(current_user,'nu54_p05_rental_authority','MEMBER')
		AND NOT (SELECT rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls FROM pg_roles WHERE rolname=current_user)
		AND NOT has_schema_privilege('ledger','CREATE')
		AND NOT has_table_privilege('ledger.rental_generation_authorized_proof','SELECT,INSERT,UPDATE,DELETE')`).Scan(&allowed)
	if err != nil || !allowed {
		db.Close()
		return nil, errors.New("dedicated protected rental writer login required")
	}
	return &RentalProofWriter{db: db, reader: reader, deployment: deployment, manifest: manifest, operator: operator}, nil
}

func (w *RentalProofWriter) EnablePendingOrderOrigins() { w.pendingOrderOrigins = true }

func (w *RentalProofWriter) Close() { w.db.Close() }

type rentalSubmission struct {
	Key        string `json:"requestKey"`
	Projection []byte `json:"projection"`
	Receipt    []byte `json:"freezeReceipt"`
}

type rentalWriteScope struct {
	collection ops.RentalCollectionScope
	challenge  [32]byte
	deadline   time.Time
	previous   *rentalv1.Anchor
	replayed   bool
}

type rentalScopeQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (w *RentalProofWriter) scope(ctx context.Context, db rentalScopeQuery, tenant, branch, device string, generation int64,
	client service.VerifiedClient, key string, submitted [32]byte) (rentalWriteScope, error) {
	var out rentalWriteScope
	var registration, measurement, deployment, challenge, identity, board, collector, peer, first, confirmed, previousBlock []byte
	var settlement, account, signer string
	var previousHeight *int64
	err := db.QueryRow(ctx, `SELECT * FROM ledger.rental_writer_scope($1,$2,$3,$4,$5,$6,$7)`,
		tenant, branch, device, generation, client.Fingerprint[:], key, submitted[:]).Scan(&registration, &measurement,
		&deployment, &settlement, &account, &challenge, &out.deadline, &identity, &board, &signer, &collector, &peer,
		&first, &confirmed, &out.collection.Stream, &out.collection.StreamGeneration, &previousHeight, &previousBlock, &out.replayed)
	if err != nil {
		return out, err
	}
	if [32]byte(deployment) != w.manifest || common.HexToAddress(settlement) != w.deployment.Settlement ||
		!client.ExpiresAt.After(time.Now()) {
		return out, service.Forbidden
	}
	out.collection.Binding = rentalCollectorBinding(branch, device, generation, w.deployment, account, registration, measurement, deployment)
	out.collection.FirstNonce, out.collection.BoardSigner, out.collection.Operator = [32]byte(first), common.HexToAddress(signer), w.operator
	out.challenge = [32]byte(challenge)
	n54g := ops.RentalRegistrationScope{Binding: out.collection.Binding, Challenge: out.challenge, Board: [32]byte(board),
		FreezeSigner: out.collection.BoardSigner, FirstNonce: out.collection.FirstNonce, CollectorCert: [32]byte(collector),
		CollectorPeer: [7]byte(peer), ExpiresAt: uint64(out.deadline.Unix())}
	if confirmedRegistrationDigest(n54g, identity) != [32]byte(confirmed) {
		return out, service.Forbidden
	}
	if previousHeight != nil {
		out.previous = &rentalv1.Anchor{Height: *previousHeight, Hash: common.Hash(previousBlock).Hex()}
	}
	return out, nil
}

func (w *RentalProofWriter) Register(mux *http.ServeMux, credentials map[string]service.Principal) {
	mux.HandleFunc("POST /v2/private/rental-collector/{branch}/{device}/{generation}/record", func(reply http.ResponseWriter, req *http.Request) {
		reply.Header().Set("Cache-Control", "no-store")
		actor, client, ok := service.CurrentClientPrincipal(req.TLS, credentials, time.Now())
		branch, device := req.PathValue("branch"), req.PathValue("device")
		generation, err := strconv.ParseInt(req.PathValue("generation"), 10, 64)
		if !ok || actor.Role != "rental-collector" || !operatorRef.MatchString(actor.Tenant) ||
			!operatorRef.MatchString(branch) || !operatorRef.MatchString(device) || err != nil ||
			generation <= 0 || strconv.FormatInt(generation, 10) != req.PathValue("generation") {
			http.NotFound(reply, req)
			return
		}
		var body rentalSubmission
		decoder := json.NewDecoder(http.MaxBytesReader(reply, req.Body, 4096))
		decoder.DisallowUnknownFields()
		if req.Header.Get("Content-Type") != "application/json" || decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF ||
			!operatorRef.MatchString(body.Key) || len(body.Projection) < 277 || len(body.Projection) > 1285 ||
			len(body.Receipt) != rentalv1.FreezeReceiptSize {
			operatorResponse(reply, nil, service.BadRequest)
			return
		}
		if err := w.record(req.Context(), actor.Tenant, branch, device, generation, client, body); err != nil {
			var available *rentalProofUnavailable
			if errors.As(err, &available) {
				operatorResponse(reply, nil, service.Conflict)
			} else {
				http.NotFound(reply, req)
			}
			return
		}
		operatorResponse(reply, json.RawMessage(`{"recorded":true}`), nil)
	})
}

type rentalProofUnavailable struct{}

func (*rentalProofUnavailable) Error() string {
	return "conclusive current rental evidence unavailable"
}

func sameRentalAnchor(a, b *rentalv1.Anchor) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func (w *RentalProofWriter) record(ctx context.Context, tenant, branch, device string, generation int64, client service.VerifiedClient,
	input rentalSubmission) error {
	// Both wire types are length-checked, and RFR1 has a fixed length. The
	// commitment deduplicates the caller's exact board statement and journal.
	digest := sha256.Sum256(append(append([]byte("P05-RFR1-submission-v1"), input.Receipt...), input.Projection...))
	scope, err := w.scope(ctx, w.db, tenant, branch, device, generation, client, input.Key, digest)
	if err != nil || scope.replayed {
		return err
	}
	body := rentalv1.Request{Projection: input.Projection, Receipt: input.Receipt, Previous: scope.previous, IncludePendingPaymentEntries: w.pendingOrderOrigins}
	verified, err := ops.VerifyRentalJournal(body, scope.collection, scope.challenge, scope.deadline, time.Now())
	if err != nil {
		return err
	}
	frozen := time.Unix(int64(verified.Receipt.FrozenAt), 0)
	validUntil := time.Unix(int64(verified.Receipt.ExpiresAt), 0)
	assessment, err := w.reader.Read(ctx, body, verified.Expected, int(verified.Receipt.EntryCount), frozen, validUntil, time.Now())
	if err != nil || assessment.PendingCount == nil {
		return &rentalProofUnavailable{}
	}
	detail, err := hex.DecodeString(assessment.Digest[len("sha256:"):])
	if err != nil {
		return &rentalProofUnavailable{}
	}

	// No database lock is held during reconciliation. Under the final device
	// fence, compare the whole live scope and the previous finalized anchor.
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	current, err := w.scope(ctx, tx, tenant, branch, device, generation, client, input.Key, digest)
	if err != nil || current.replayed {
		return err
	}
	if current.collection != scope.collection || current.challenge != scope.challenge || !current.deadline.Equal(scope.deadline) ||
		!sameRentalAnchor(current.previous, scope.previous) || !validUntil.After(time.Now()) ||
		!time.Unix(assessment.ValidUntil, 0).After(time.Now()) {
		return &rentalProofUnavailable{}
	}
	freeze, root, anchor := common.HexToHash(verified.Expected.FreezeReceipt), common.Hash(verified.Receipt.Root), common.HexToHash(assessment.AnchorHash)
	var previousHeight, previousHash any
	if scope.previous != nil {
		previousHeight, previousHash = scope.previous.Height, common.HexToHash(scope.previous.Hash).Bytes()
	}
	binding := scope.collection.Binding
	var inserted bool
	err = tx.QueryRow(ctx, `SELECT ledger.record_rental_generation_proof($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
		$21,$22,$23,$24,$25,$26,$27,$28,$29,$30)`, branch, device, generation, input.Key, binding[40:72], binding[72:104],
		strings.ToLower(scope.collection.BoardSigner.Hex()), strings.ToLower(w.deployment.Settlement.Hex()), binding[104:136], scope.collection.Stream,
		scope.collection.StreamGeneration, scope.challenge[:], freeze[:], root[:], detail, verified.Receipt.FirstNonce[:], verified.Receipt.NextNonce[:],
		verified.PaymentEntries, verified.LimitEntries, verified.BurnedEntries, *assessment.PendingPayments, *assessment.PendingLimits,
		*assessment.AnchorHeight, anchor[:], frozen, validUntil, time.Unix(assessment.AnchorTime, 0), time.Unix(assessment.ValidUntil, 0),
		previousHeight, previousHash).Scan(&inserted)
	if err != nil {
		return err
	}
	if !inserted {
		return &rentalProofUnavailable{}
	}
	if _, err = tx.Exec(ctx, "SELECT ledger.record_rental_proof_submission($1,$2,$3,$4,$5)", branch, device, generation, input.Key, digest[:]); err != nil {
		return err
	}
	if assessment.PendingPaymentEntries != nil {
		for i := 0; i < int(verified.Receipt.EntryCount); i++ {
			if *assessment.PendingPaymentEntries&(1<<i) == 0 {
				continue
			}
			entry := input.Projection[277+126*i : 277+126*(i+1)]
			commitment := make([]byte, 0, 116)
			commitment = append(commitment, entry[2:34]...)
			commitment = append(commitment, entry[42:126]...)
			witness := sha256.Sum256(commitment)
			var recorded bool
			if err := tx.QueryRow(ctx, `SELECT ledger.add_renter_v1_pending_witness($1,$2,$3,$4,$5)`,
				branch, device, generation, input.Key, witness[:]).Scan(&recorded); err != nil || !recorded {
				return &rentalProofUnavailable{}
			}
		}
	}
	if !client.ExpiresAt.After(time.Now()) {
		return service.Forbidden
	}
	return tx.Commit(ctx)
}
