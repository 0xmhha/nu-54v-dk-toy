package commerce

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core/ops"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RentalCollectorIssuer is enabled with a dedicated database login and the
// operator key for an independently pinned settlement deployment.
type RentalCollectorIssuer struct {
	db         *pgxpool.Pool
	operator   *ecdsa.PrivateKey
	deployment core.Deployment
	manifest   [32]byte
}

func OpenRentalCollectorIssuer(ctx context.Context, dsn string, operator *ecdsa.PrivateKey, deployment core.Deployment, manifest [32]byte) (*RentalCollectorIssuer, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil || operator == nil || deployment.ChainID == 0 || deployment.Settlement == (common.Address{}) || manifest == [32]byte{} {
		return nil, errors.New("invalid protected rental issuer settings")
	}
	config.ConnConfig.RuntimeParams["lock_timeout"] = "2s"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "7s"
	config.ConnConfig.RuntimeParams["jit"] = "off" // see commerce.Open
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("protected rental authority connection failed")
	}
	var allowed bool
	err = db.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p05_rental_authority','USAGE')
		AND NOT pg_has_role(current_user,'nu54_p05_commerce','MEMBER')
		AND NOT pg_has_role(current_user,'nu54_p05_rental_evidence','MEMBER')
		AND NOT (SELECT rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls FROM pg_roles WHERE rolname=current_user)
		AND NOT has_schema_privilege('ledger','CREATE')
		AND NOT has_table_privilege('ledger.rental_collector_registration','SELECT,INSERT,UPDATE,DELETE')
		AND NOT has_table_privilege('ledger.rental_generation_source','SELECT,INSERT,UPDATE,DELETE')`).Scan(&allowed)
	if err != nil || !allowed {
		db.Close()
		return nil, errors.New("dedicated protected rental authority login and migration required")
	}
	return &RentalCollectorIssuer{db: db, operator: operator, deployment: deployment, manifest: manifest}, nil
}

func (s *RentalCollectorIssuer) Close() { s.db.Close() }

func putCollectorUUID(dst []byte, id string) {
	_, _ = hex.Decode(dst, []byte(strings.ReplaceAll(id, "-", "")))
}

func rentalCollectorBinding(branch, device string, generation int64, deployment core.Deployment, account string,
	registration, measurement, deploymentDigest []byte) [208]byte {
	var binding [208]byte
	binary.BigEndian.PutUint64(binding[:8], uint64(generation))
	putCollectorUUID(binding[8:24], branch)
	putCollectorUUID(binding[24:40], device)
	copy(binding[40:72], registration)
	copy(binding[72:104], measurement)
	copy(binding[104:136], deploymentDigest)
	binary.BigEndian.PutUint64(binding[160:168], deployment.ChainID)
	copy(binding[168:188], deployment.Settlement[:])
	copy(binding[188:208], common.HexToAddress(account).Bytes())
	return binding
}

func (s *RentalCollectorIssuer) Register(mux *http.ServeMux, credentials map[string]service.Principal) {
	mux.HandleFunc("GET /v2/private/rental-registration/{branch}/{device}/{generation}", s.registerDevice(credentials, false))
	mux.HandleFunc("POST /v2/private/rental-registration/{branch}/{device}/{generation}", s.registerDevice(credentials, true))
	mux.HandleFunc("POST /v2/private/rental-collector/{branch}/{device}/{generation}/freeze-challenge", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		now := time.Now()
		actor, client, ok := service.CurrentClientPrincipal(r.TLS, credentials, now)
		branch, device := r.PathValue("branch"), r.PathValue("device")
		generation, err := strconv.ParseInt(r.PathValue("generation"), 10, 64)
		if !ok || actor.Role != "rental-collector" || !operatorRef.MatchString(actor.Tenant) ||
			!operatorRef.MatchString(branch) || !operatorRef.MatchString(device) || err != nil ||
			generation < 1 || strconv.FormatInt(generation, 10) != r.PathValue("generation") {
			http.NotFound(w, r)
			return
		}

		var input struct {
			ChannelNonce string `json:"channelNonce"`
			ExpiresAt    uint64 `json:"expiresAt"`
		}
		reader := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512))
		reader.DisallowUnknownFields()
		if r.Header.Get("Content-Type") != "application/json" || reader.Decode(&input) != nil || reader.Decode(new(any)) != io.EOF {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		nonce, err := hex.DecodeString(input.ChannelNonce)
		if err != nil || len(nonce) != 32 || common.Hash(nonce) == (common.Hash{}) || input.ExpiresAt <= uint64(now.Unix()) ||
			input.ExpiresAt-uint64(now.Unix()) > 120 {
			operatorResponse(w, nil, service.BadRequest)
			return
		}
		reply, err := s.issue(r.Context(), actor.Tenant, branch, device, generation, client, ble.ProtectedRentalChallenge{
			Nonce: [32]byte(nonce), ExpiresAt: input.ExpiresAt,
		})
		if err != nil {
			http.NotFound(w, r)
			return
		}
		operatorResponse(w, reply, nil)
	})
}

func (s *RentalCollectorIssuer) issue(ctx context.Context, tenant, branch, device string, generation int64,
	client service.VerifiedClient, link ble.ProtectedRentalChallenge) (json.RawMessage, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	var registration, measurement, deployment, challenge, identity, board, collector, peer, first, confirmed []byte
	var settlement, account, signer string
	var deadline time.Time
	err = tx.QueryRow(ctx, `SELECT * FROM ledger.rental_collector_authority($1,$2,$3,$4,$5)`,
		tenant, branch, device, generation, client.Fingerprint[:]).Scan(&registration, &measurement, &deployment,
		&settlement, &account, &challenge, &deadline, &identity, &board, &signer, &collector, &peer, &first, &confirmed)
	if err != nil {
		return nil, err
	}
	if [32]byte(deployment) != s.manifest || common.HexToAddress(settlement) != s.deployment.Settlement {
		return nil, errors.New("protected rental authority deployment differs")
	}
	binding := rentalCollectorBinding(branch, device, generation, s.deployment, account, registration, measurement, deployment)
	scope := ops.RentalRegistrationScope{
		Challenge: [32]byte(challenge), Board: [32]byte(board), FreezeSigner: common.HexToAddress(signer),
		Binding: binding, CollectorCert: [32]byte(collector), CollectorPeer: [7]byte(peer), FirstNonce: [32]byte(first),
		ExpiresAt: uint64(deadline.Unix()),
	}
	if confirmedRegistrationDigest(scope, identity) != [32]byte(confirmed) {
		return nil, errors.New("protected collector registration changed")
	}
	expiry := min(link.ExpiresAt, uint64(deadline.Unix()), uint64(client.ExpiresAt.Unix()))
	request := ops.RentalFreezeRequestV2{Binding: crypto.Keccak256Hash(binding[:]), Challenge: common.Hash(challenge),
		ChannelNonce: common.Hash(link.Nonce), ExpiresAt: expiry}
	now := time.Now()
	signature, err := ops.IssueRentalFreezeV2(s.deployment.Domain(), s.operator, request, link, uint64(now.Unix()),
		func(core.Domain, ops.RentalFreezeRequestV2) error {
			// The authority function holds the generation, grant and source
			// locks until this transaction commits.
			return nil
		})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if time.Now().Unix() >= int64(expiry) {
		return nil, errors.New("collector authority expired before disclosure")
	}
	return json.Marshal(map[string]any{"binding": hex.EncodeToString(request.Binding[:]),
		"challenge": hex.EncodeToString(request.Challenge[:]), "expiresAt": expiry, "signature": hex.EncodeToString(signature)})
}

// Authenticate the exact staged statement and the installation public key
// that confirmed it. An independent valid re-signature has the same identity.
func confirmedRegistrationDigest(scope ops.RentalRegistrationScope, identity []byte) [32]byte {
	return sha256.Sum256(append(scope.Body(), identity...))
}

func (s *RentalCollectorIssuer) registerDevice(credentials map[string]service.Principal, confirm bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		actor, client, ok := service.CurrentClientPrincipal(r.TLS, credentials, time.Now())
		branch, device := r.PathValue("branch"), r.PathValue("device")
		generation, err := strconv.ParseInt(r.PathValue("generation"), 10, 64)
		if !ok || actor.Role != "operator" || !operatorRef.MatchString(actor.Tenant) || !operatorRef.MatchString(actor.Subject) ||
			!operatorRef.MatchString(branch) || !operatorRef.MatchString(device) || err != nil ||
			generation < 1 || strconv.FormatInt(generation, 10) != r.PathValue("generation") {
			http.NotFound(w, r)
			return
		}
		var proof []byte
		if confirm {
			// Device evidence is binary, and this endpoint does not accept browser forms.
			if r.Header.Get("Content-Type") != "application/octet-stream" {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
			proof, err = io.ReadAll(http.MaxBytesReader(w, r.Body, ops.RentalRegistrationWireSize))
			if err != nil || len(proof) != ops.RentalRegistrationWireSize {
				operatorResponse(w, nil, service.BadRequest)
				return
			}
		}
		result, err := s.registration(r.Context(), actor, client, branch, device, generation, proof)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		operatorResponse(w, result, nil)
	}
}

func (s *RentalCollectorIssuer) registration(ctx context.Context, actor service.Principal, client service.VerifiedClient,
	branch, device string, generation int64, proof []byte) (json.RawMessage, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	var registration, measurement, deployment, challenge, identity, board, collector, peer, first, source []byte
	var settlement, account, signer string
	var deadline time.Time
	err = tx.QueryRow(ctx, `SELECT * FROM ledger.rental_collector_registration_scope($1,$2,$3,$4,$5)`,
		actor.Tenant, branch, actor.Subject, device, generation).Scan(&registration, &measurement, &deployment,
		&settlement, &account, &challenge, &deadline, &identity, &board, &signer, &collector, &peer, &first, &source)
	if err != nil {
		return nil, err
	}
	if [32]byte(deployment) != s.manifest || common.HexToAddress(settlement) != s.deployment.Settlement {
		return nil, errors.New("protected rental registration deployment differs")
	}
	if !client.ExpiresAt.After(time.Now()) {
		return nil, errors.New("protected rental registration operator expired")
	}
	// The signed proof uses the exact staged timestamp. It cannot be extended
	// by a new authenticated request or a later enrollment.
	scope := ops.RentalRegistrationScope{
		Challenge: [32]byte(challenge), Board: [32]byte(board), FreezeSigner: common.HexToAddress(signer),
		Binding:       rentalCollectorBinding(branch, device, generation, s.deployment, account, registration, measurement, deployment),
		CollectorCert: [32]byte(collector), CollectorPeer: [7]byte(peer), FirstNonce: [32]byte(first),
		ExpiresAt: uint64(deadline.Unix()),
	}
	if proof != nil {
		if err := ops.VerifyRentalRegistration(proof, identity, scope, uint64(time.Now().Unix())); err != nil {
			return nil, err
		}
		// A genuine board can return a fresh P-256 signature after a lost
		// response. Deduplicate the already authenticated statement.
		digest := confirmedRegistrationDigest(scope, identity)
		if _, err := tx.Exec(ctx, `SELECT ledger.record_rental_collector_registration($1,$2,$3,$4,$5,$6,$7)`,
			actor.Tenant, branch, actor.Subject, device, generation, source, digest[:]); err != nil {
			return nil, err
		}
	}
	if !client.ExpiresAt.After(time.Now()) || !deadline.After(time.Now()) {
		return nil, errors.New("protected rental registration expired")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if proof != nil {
		return json.RawMessage(`{"confirmed":true}`), nil
	}
	return json.Marshal(map[string]any{"challenge": hex.EncodeToString(challenge), "expiresAt": deadline.Unix(),
		"installationRequest": hex.EncodeToString(scope.Body())})
}
