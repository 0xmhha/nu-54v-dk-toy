package wallet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/jackc/pgx/v5"
)

type RecoveryStatus struct {
	Wallet              string                `json:"wallet"`
	State               string                `json:"state"`
	Epoch               uint64                `json:"epoch"`
	Policy              string                `json:"policy"`
	IndependentSignPath bool                  `json:"independentSignPath"`
	QuorumExposure      bool                  `json:"quorumExposure"`
	Evidence            []RecoveryEvidence    `json:"evidence"`
	Participants        []RecoveryParticipant `json:"participants,omitempty"`
	IncidentRef         string                `json:"incidentRef,omitempty"`
	LatestSession       string                `json:"latestSession,omitempty"`
	AssetMoves          int                   `json:"assetMoves"`
	session             string
	peers               []ceremony.Peer
}

type RecoveryParticipant struct {
	Role      ceremony.Role `json:"role"`
	State     string        `json:"state"`
	CheckedAt time.Time     `json:"checkedAt"`
}

type RecoveryEvidence struct {
	Reference string    `json:"reference"`
	Digest    string    `json:"digest,omitempty"`
	Operation string    `json:"operation"`
	Kind      string    `json:"kind"`
	At        time.Time `json:"at"`
}

func quorumExposure(ctx context.Context, tx pgx.Tx, wallet string) (bool, error) {
	var exposed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wallet_mpc.audit_outbox
		WHERE wallet_id=$1 AND event_kind='incident_hold')`, wallet).Scan(&exposed)
	return exposed, err
}

func (s Store) recoveryStatus(ctx context.Context, subject string, freeze bool, actor Claims, options ...operatorScope) (RecoveryStatus, error) {
	return withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (RecoveryStatus, error) {
		var result RecoveryStatus
		w, err := readWallet(ctx, tx, subject)
		if err != nil {
			return result, err
		}
		if len(options) > 0 {
			scope := options[0]
			if scope.wallet != "" && w.ID != scope.wallet {
				return result, ErrForbidden
			}
			if s.Bindings != nil && subject != s.Bindings.LegacySubject {
				var assigned bool
				err = tx.QueryRow(ctx, `SELECT profile->>'Operator'=$2 FROM wallet_mpc.profile_binding WHERE subject_ref=$1`, subject, actor.Subject).Scan(&assigned)
				if errors.Is(err, pgx.ErrNoRows) || err == nil && !assigned {
					return result, ErrForbidden
				}
				if err != nil {
					return result, err
				}
			}
		}
		result.Wallet, result.State, result.Epoch, result.Policy = w.ID, w.State, w.Epoch, w.ProfileName
		result.QuorumExposure, err = quorumExposure(ctx, tx, w.ID)
		if err != nil {
			return result, err
		}
		if err := claimsCurrent(time.Now(), actor); err != nil {
			return result, err
		}
		if len(options) > 0 && !options[0].current(actor) {
			return result, ErrForbidden
		}
		if freeze && !result.QuorumExposure {
			if w.State == "closed" || w.Epoch == 0 {
				return result, ErrConflict
			}
			if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.wallet SET state='blocked' WHERE wallet_id=$1`, w.ID); err != nil {
				return result, err
			}
			if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.operation SET state='failed',approval_proof=NULL
				WHERE wallet_id=$1 AND state IN ('reserved','running','held')`, w.ID); err != nil {
				return result, err
			}
			if err := audit(ctx, tx, w.ID, "", "incident_hold"); err != nil {
				return result, err
			}
			result.State, result.QuorumExposure = "blocked", true
		}
		if w.Epoch > 0 {
			operation, err := readOperation(tx.QueryRow(ctx, `SELECT `+operationColumns+`
				FROM wallet_mpc.key_epoch k JOIN wallet_mpc.operation o ON o.operation_id=k.operation_id
				WHERE k.wallet_id=$1 AND k.epoch=$2`, w.ID, w.Epoch))
			if err != nil {
				return result, err
			}
			result.session = operation.Session.ID
			for _, peer := range operation.Session.Peers {
				if peer.Epoch == w.Epoch {
					result.peers = append(result.peers, peer)
				}
			}
		}
		err = tx.QueryRow(ctx, `SELECT coalesce((SELECT request_ref::text FROM wallet_mpc.operation
			WHERE wallet_id=$1 ORDER BY created_at DESC,operation_id DESC LIMIT 1),'')`, w.ID).Scan(&result.LatestSession)
		if err != nil {
			return result, err
		}
		rows, err := tx.Query(ctx, `SELECT a.event_id,coalesce(o.request_ref::text,''),a.event_kind,a.created_at,
			coalesce((SELECT encode(p.evidence_digest,'hex') FROM wallet_mpc.participant_ack p
			  WHERE p.operation_id=a.operation_id AND p.received_at<=a.created_at
			  ORDER BY p.received_at DESC,p.participant_ref,p.evidence_kind LIMIT 1),'')
			FROM wallet_mpc.audit_outbox a LEFT JOIN wallet_mpc.operation o USING(operation_id)
			WHERE a.wallet_id=$1 ORDER BY a.event_id DESC LIMIT 24`, w.ID)
		if err != nil {
			return result, err
		}
		defer rows.Close()
		for rows.Next() {
			var event RecoveryEvidence
			var sequence int64
			if err := rows.Scan(&sequence, &event.Operation, &event.Kind, &event.At, &event.Digest); err != nil {
				return result, err
			}
			ref := sha256.Sum256([]byte("nu54/p03/operator-event/v1\x00" + w.ID + "\x00" + strconv.FormatInt(sequence, 10)))
			event.Reference = hex.EncodeToString(ref[:16])
			if event.Kind == "incident_hold" {
				result.IncidentRef = event.Reference
			}
			result.Evidence = append(result.Evidence, event)
		}
		return result, rows.Err()
	}, actor)
}

func (a API) operatorTarget(ctx context.Context, actor, requested string) (string, error) {
	if requested == "" {
		if a.ProfileSubject != "" && actor == a.RecoveryOperatorSubject {
			return a.ProfileSubject, nil
		}
		return "", ErrForbidden
	}
	if !canonicalUUID(requested) {
		return "", ErrForbidden
	}
	if a.ProfileSubject != "" && actor == a.RecoveryOperatorSubject {
		var legacyOwn bool
		err := a.Store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wallet_mpc.wallet WHERE wallet_id=$1 AND subject_ref=$2)`,
			requested, a.ProfileSubject).Scan(&legacyOwn)
		if err != nil {
			return "", err
		}
		if legacyOwn {
			return a.ProfileSubject, nil
		}
	}
	var subject string
	err := a.Store.Pool.QueryRow(ctx, `SELECT w.subject_ref FROM wallet_mpc.wallet w JOIN wallet_mpc.profile_binding p USING(subject_ref)
		WHERE w.wallet_id=$1 AND p.profile->>'Operator'=$2`, requested, actor).Scan(&subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	return subject, err
}

func (a API) VerifyRecoveryOperator(token string, now time.Time) (Claims, error) {
	actor, err := a.RecoveryOperator.Verify(token, "wallet:recovery-ops", now)
	if err != nil || a.Store.Bindings == nil && actor.Subject != a.RecoveryOperatorSubject || !a.operatorCurrent(actor) {
		return Claims{}, ErrForbidden
	}
	return actor, nil
}

func (a API) operatorCurrent(actor Claims) bool {
	return claimsCurrent(time.Now(), actor) == nil && (a.OperatorGrant == nil || a.OperatorGrant(actor))
}

type operatorScope struct {
	wallet  string
	current func(Claims) bool
}

func (a API) operatorServiceStatus(ctx context.Context, actor Claims, requested string, freeze bool) (RecoveryStatus, error) {
	subject, err := a.operatorTarget(ctx, actor.Subject, requested)
	if err != nil {
		return RecoveryStatus{}, err
	}
	result, err := a.Store.recoveryStatus(ctx, subject, freeze, actor, operatorScope{requested, a.operatorCurrent})
	if err == nil && !a.operatorCurrent(actor) {
		return RecoveryStatus{}, ErrForbidden
	}
	if err != nil || freeze || a.OperatorParticipants == nil || len(result.peers) == 0 {
		return result, err
	}
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	availability := make([]RecoveryParticipant, len(result.peers))
	var calls sync.WaitGroup
	for i, peer := range result.peers {
		calls.Go(func() {
			_, err := a.OperatorParticipants.Status(probe, peer.ID, result.session)
			state := "unavailable"
			if err == nil || errors.Is(err, os.ErrNotExist) {
				state = "reachable"
			}
			availability[i] = RecoveryParticipant{Role: peer.Role, State: state, CheckedAt: time.Now().UTC()}
		})
	}
	calls.Wait()
	fresh, err := a.Store.recoveryStatus(ctx, subject, false, actor, operatorScope{requested, a.operatorCurrent})
	if err != nil {
		return fresh, err
	}
	if !a.operatorCurrent(actor) {
		return RecoveryStatus{}, ErrForbidden
	}
	if fresh.session == result.session {
		fresh.Participants = availability
	}
	return fresh, nil
}

func (a API) recoveryHandlers(mux *http.ServeMux) {
	verify := func(w http.ResponseWriter, req *http.Request) (Claims, bool) {
		w.Header().Set("Cache-Control", "no-store")
		actor, err := a.VerifyRecoveryOperator(bearer(req), time.Now())
		if err != nil {
			http.Error(w, "recovery operator required", http.StatusUnauthorized)
			return Claims{}, false
		}
		return actor, true
	}
	mux.HandleFunc("GET /v1/recovery/status", func(w http.ResponseWriter, req *http.Request) {
		actor, ok := verify(w, req)
		if !ok {
			return
		}
		result, err := a.operatorServiceStatus(req.Context(), actor, req.URL.Query().Get("wallet"), false)
		writeClaimsResponse(w, result, err, actor)
	})
	mux.HandleFunc("POST /v1/recovery/reviews", func(w http.ResponseWriter, req *http.Request) {
		actor, ok := verify(w, req)
		if !ok {
			return
		}
		var input struct {
			Wallet  string `json:"wallet"`
			Request string `json:"request"`
		}
		if !requestJSON(w, req, &input) || !validID(w, input.Wallet) || !validID(w, input.Request) {
			return
		}
		subject, err := a.operatorTarget(req.Context(), actor.Subject, input.Wallet)
		if err == nil {
			err = a.Store.PermitRecoveryReview(req.Context(), subject, input.Request, actor)
		}
		writeClaimsResponse(w, nil, err, actor)
	})
	mux.HandleFunc("POST /v1/recovery/incidents", func(w http.ResponseWriter, req *http.Request) {
		actor, ok := verify(w, req)
		if !ok {
			return
		}
		var input struct {
			Kind   string `json:"kind"`
			Wallet string `json:"wallet,omitempty"`
		}
		if !requestJSON(w, req, &input) {
			return
		}
		if input.Kind != "quorum_exposure" {
			http.Error(w, "invalid incident kind", 400)
			return
		}
		result, err := a.operatorServiceStatus(req.Context(), actor, input.Wallet, true)
		writeClaimsResponse(w, result, err, actor)
	})
}
