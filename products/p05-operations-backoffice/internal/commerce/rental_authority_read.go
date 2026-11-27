package commerce

import (
	"context"
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
	"github.com/jackc/pgx/v5/pgxpool"
)

type RentalAuthorityReader struct {
	db         *pgxpool.Pool
	deployment core.Deployment
	manifest   [32]byte
	operator   common.Address
}

func OpenRentalAuthorityReader(ctx context.Context, dsn string, deployment core.Deployment, manifest [32]byte,
	operator common.Address) (*RentalAuthorityReader, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil || deployment.ChainID != 8283 || deployment.Settlement == (common.Address{}) ||
		manifest == [32]byte{} || operator == (common.Address{}) {
		return nil, errors.New("invalid protected rental authority reader settings")
	}
	config.ConnConfig.RuntimeParams["lock_timeout"] = "2s"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "7s"
	config.ConnConfig.RuntimeParams["jit"] = "off" // see commerce.Open
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("protected rental authority reader connection failed")
	}
	var allowed bool
	err = db.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p05_rental_authority_read','USAGE')
		AND NOT pg_has_role(current_user,'nu54_p05_commerce','MEMBER')
		AND NOT pg_has_role(current_user,'nu54_p05_rental_authority','MEMBER')
		AND NOT pg_has_role(current_user,'nu54_p05_rental_evidence','MEMBER')
		AND NOT (SELECT rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls FROM pg_roles WHERE rolname=current_user)
		AND NOT has_schema_privilege('ledger','CREATE')
		AND NOT has_table_privilege('ledger.rental_authority_audience','SELECT,INSERT,UPDATE,DELETE')
		AND NOT has_table_privilege('ledger.rental_generation_source','SELECT,INSERT,UPDATE,DELETE')
		AND NOT has_table_privilege('ledger.rental_collector_registration','SELECT,INSERT,UPDATE,DELETE')
		AND NOT has_table_privilege('ledger.rental_generation_authorized_proof','SELECT,INSERT,UPDATE,DELETE')
		AND NOT has_function_privilege('ledger.record_rental_proof_submission(uuid,uuid,bigint,uuid,bytea)','EXECUTE')`).Scan(&allowed)
	if err != nil || !allowed {
		db.Close()
		return nil, errors.New("dedicated protected rental read authority login required")
	}
	return &RentalAuthorityReader{db: db, deployment: deployment, manifest: manifest, operator: operator}, nil
}

func (r *RentalAuthorityReader) Close() { r.db.Close() }

func (r *RentalAuthorityReader) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST "+rentalv1.AuthorityPath, func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		client, ok := service.CurrentVerifiedClient(req.TLS, time.Now())
		if !ok {
			http.NotFound(w, req)
			return
		}
		var input rentalv1.AuthorityRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 1024))
		decoder.DisallowUnknownFields()
		if req.Header.Get("Content-Type") != "application/json" || decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
			http.NotFound(w, req)
			return
		}
		result, err := r.read(req.Context(), client, input)
		if err != nil {
			http.NotFound(w, req)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

func (r *RentalAuthorityReader) read(ctx context.Context, client service.VerifiedClient, input rentalv1.AuthorityRequest) (rentalv1.AuthorityResponse, error) {
	var empty rentalv1.AuthorityResponse
	generation, err := strconv.ParseInt(input.Generation, 10, 64)
	challenge, challengeErr := hex.DecodeString(strings.TrimPrefix(input.Challenge, "0x"))
	writer, writerErr := hex.DecodeString(input.CallerFingerprint)
	if err != nil || generation < 1 || strconv.FormatInt(generation, 10) != input.Generation ||
		!operatorRef.MatchString(input.Branch) || !operatorRef.MatchString(input.Board) || !operatorRef.MatchString(input.Stream) ||
		!operatorRef.MatchString(input.StreamGeneration) || challengeErr != nil || len(challenge) != 32 ||
		common.Hash(challenge) == (common.Hash{}) || input.Challenge != common.Hash(challenge).Hex() ||
		writerErr != nil || len(writer) != 32 || input.CallerFingerprint != hex.EncodeToString(writer) ||
		[32]byte(writer) == [32]byte{} || [32]byte(writer) == client.Fingerprint {
		return empty, service.Forbidden
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	var registration, measurement, deployment, currentChallenge, identity, board, collector, peer, first, confirmed []byte
	var settlement, account, signer string
	var registrationDeadline, registeredAt, challengeUntil, validUntil time.Time
	err = tx.QueryRow(ctx, `SELECT * FROM ledger.read_rental_generation_authority($1,$2,$3,$4,$5,$6,$7,$8)`,
		input.Branch, input.Board, generation, input.Stream, input.StreamGeneration, challenge, client.Fingerprint[:], writer).
		Scan(&registration, &measurement, &deployment, &settlement, &account, &currentChallenge, &registrationDeadline, &identity, &board,
			&signer, &collector, &peer, &first, &confirmed, &registeredAt, &challengeUntil, &validUntil)
	if err != nil {
		return empty, err
	}
	address, accountAddress := common.HexToAddress(signer), common.HexToAddress(account)
	if [32]byte(deployment) != r.manifest || common.HexToAddress(settlement) != r.deployment.Settlement ||
		address == (common.Address{}) || address == accountAddress || address == r.operator {
		return empty, service.Forbidden
	}
	binding := rentalCollectorBinding(input.Branch, input.Board, generation, r.deployment, account, registration, measurement, deployment)
	confirmedScope := ops.RentalRegistrationScope{Binding: binding, Challenge: [32]byte(currentChallenge), Board: [32]byte(board),
		FreezeSigner: address, CollectorCert: [32]byte(collector), CollectorPeer: [7]byte(peer), FirstNonce: [32]byte(first),
		ExpiresAt: uint64(registrationDeadline.Unix())}
	if confirmedRegistrationDigest(confirmedScope, identity) != [32]byte(confirmed) {
		return empty, service.Forbidden
	}
	valid := min(validUntil.Unix(), client.ExpiresAt.Unix())
	registered := registeredAt.Unix()
	if registeredAt.Nanosecond() != 0 {
		registered++
	}
	if now := time.Now().Unix(); valid <= now || registered <= 0 || registered > now || challengeUntil.Unix() <= now {
		return empty, service.Forbidden
	}
	if err := tx.Commit(ctx); err != nil {
		return empty, err
	}
	if time.Now().Unix() >= valid {
		return empty, service.Forbidden
	}
	return rentalv1.AuthorityResponse{
		AuthorityRequest: input, Registration: common.Hash(registration).Hex(), Measurement: common.Hash(measurement).Hex(),
		Deployment: common.Hash(deployment).Hex(), ChainID: r.deployment.ChainID, Settlement: strings.ToLower(settlement),
		Account: strings.ToLower(account), BoardSigner: strings.ToLower(signer), Operator: strings.ToLower(r.operator.Hex()),
		RegisteredAt: registered, ChallengeUntil: challengeUntil.Unix(), ValidUntil: valid,
	}, nil
}
