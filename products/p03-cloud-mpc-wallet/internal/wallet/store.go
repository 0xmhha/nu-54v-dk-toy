package wallet

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/marketlab"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConflict  = errors.New("wallet state changed")
	ErrForbidden = errors.New("wallet credential unavailable")
	ErrMissing   = errors.New("wallet request unavailable")
)

type Wallet struct {
	ID                string `json:"id"`
	State             string `json:"state"`
	ProfileName       string `json:"profile"`
	Epoch             uint64 `json:"epoch"`
	Address           string `json:"address,omitempty"`
	PublicKey         []byte `json:"publicKey,omitempty"`
	RecoveryAvailable bool   `json:"recoveryAvailable"`
}

type Operation struct {
	RequestID        string           `json:"requestId,omitempty"`
	Session          ceremony.Session `json:"session"`
	State            string           `json:"state"`
	Fingerprint      [32]byte         `json:"fingerprint"`
	Transfer         *Transfer        `json:"transfer,omitempty"`
	NotBefore        time.Time        `json:"notBefore"`
	Acknowledged     int              `json:"acknowledged"`
	Signature        []byte           `json:"signature,omitempty"`
	RecoveryID       *byte            `json:"recoveryId,omitempty"`
	Refusal          *RecoveryReceipt `json:"refusal,omitempty"`
	ReviewAvailable  bool             `json:"reviewAvailable,omitempty"`
	SuccessorRequest string           `json:"successorRequest,omitempty"`
	proof            []byte
}

type Profile struct {
	First         []ceremony.Peer
	Replacement   []ceremony.Peer
	Future        [][]ceremony.Peer
	RecoveryDelay time.Duration
}

type Store struct {
	Pool         *pgxpool.Pool
	Bindings     *Bindings
	Profile      Profile
	MarketPolicy *marketlab.Policy
}

func withTx[T any](ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) (T, error)) (T, error) {
	var zero T
	tx, err := pool.Begin(ctx)
	if err != nil {
		return zero, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	v, err := fn(tx)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, err
	}
	return v, nil
}

func claimsCurrent(now time.Time, actors ...Claims) error {
	for _, actor := range actors {
		if now.Unix() >= actor.Expires {
			return ErrForbidden
		}
	}
	return nil
}

// Check authenticated claims after any lock or insert the SQL callback can wait
// on and before its transaction commits or releases a private result.
func withClaimsTx[T any](ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) (T, error), actors ...Claims) (T, error) {
	return withTx(ctx, pool, func(tx pgx.Tx) (T, error) {
		var zero T
		if err := claimsCurrent(time.Now(), actors...); err != nil {
			return zero, err
		}
		result, err := fn(tx)
		if expiry := claimsCurrent(time.Now(), actors...); expiry != nil {
			return zero, expiry
		}
		return result, err
	})
}

func (s Store) Enroll(ctx context.Context, actor Claims) error {
	_, err := withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (bool, error) {
		if _, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.identity(subject_ref) VALUES ($1) ON CONFLICT DO NOTHING`, actor.Subject); err != nil {
			return false, err
		}
		if err := lockIdentity(ctx, tx, actor.Subject); err != nil {
			return false, err
		}
		if err := claimsCurrent(time.Now(), actor); err != nil {
			return false, err
		}
		if err := s.bind(ctx, tx, actor); err != nil {
			return false, err
		}
		var first, alreadyLinked bool
		err := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT FROM wallet_mpc.auth_link WHERE subject_ref=$1),
			EXISTS(SELECT FROM wallet_mpc.auth_link WHERE subject_ref=$1 AND credential_ref=$2 AND unlinked_at IS NULL)`,
			actor.Subject, actor.Credential).Scan(&first, &alreadyLinked)
		if err != nil {
			return false, err
		}
		if alreadyLinked {
			return true, nil
		}
		if !first {
			return false, ErrForbidden
		}
		return true, linkCredential(ctx, tx, actor)
	}, actor)
	return err
}

func (s Store) Link(ctx context.Context, actor, enrollment Claims) error {
	if actor.Subject != enrollment.Subject || actor.Issuer != enrollment.Issuer || actor.Credential == enrollment.Credential {
		return ErrForbidden
	}
	_, err := withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (bool, error) {
		if err := s.authenticate(ctx, tx, actor); err != nil {
			return false, err
		}
		if err := claimsCurrent(time.Now(), actor, enrollment); err != nil {
			return false, err
		}
		return true, linkCredential(ctx, tx, enrollment)
	}, actor, enrollment)
	return err
}

func linkCredential(ctx context.Context, tx pgx.Tx, actor Claims) error {
	tag, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.auth_link(subject_ref,credential_ref) VALUES ($1,$2)
		ON CONFLICT (subject_ref,credential_ref) DO UPDATE SET
			linked_at=CASE WHEN wallet_mpc.auth_link.unlinked_at IS NULL THEN wallet_mpc.auth_link.linked_at ELSE clock_timestamp() END,
			unlinked_at=NULL
		WHERE wallet_mpc.auth_link.unlinked_at IS NULL OR wallet_mpc.auth_link.unlinked_at<$3`,
		actor.Subject, actor.Credential, time.Unix(actor.Issued, 0))
	if err == nil && tag.RowsAffected() != 1 {
		return ErrForbidden
	}
	return err
}

func lockIdentity(ctx context.Context, tx pgx.Tx, subject string) error {
	var ref string
	err := tx.QueryRow(ctx, `SELECT subject_ref FROM wallet_mpc.identity WHERE subject_ref=$1 FOR UPDATE`, subject).Scan(&ref)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrForbidden
	}
	return err
}

func (s Store) authenticate(ctx context.Context, tx pgx.Tx, actor Claims) error {
	if err := lockIdentity(ctx, tx, actor.Subject); err != nil {
		return err
	}
	if err := claimsCurrent(time.Now(), actor); err != nil {
		return err
	}
	if s.Bindings != nil && actor.Subject != s.Bindings.LegacySubject {
		p, err := readBinding(ctx, tx, actor.Subject)
		if errors.Is(err, ErrMissing) {
			return ErrForbidden
		}
		if err != nil {
			return err
		}
		if p.Issuer != actor.Issuer {
			return ErrForbidden
		}
	}

	var linked bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wallet_mpc.auth_link WHERE subject_ref=$1 AND credential_ref=$2 AND unlinked_at IS NULL)`, actor.Subject, actor.Credential).Scan(&linked)
	if err != nil {
		return err
	}
	if !linked {
		return ErrForbidden
	}
	return nil
}

func (s Store) Subject(ctx context.Context, actor Claims) (string, error) {
	return withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (string, error) {
		if err := s.authenticate(ctx, tx, actor); err != nil {
			return "", err
		}
		return actor.Subject, nil
	}, actor)
}

type Credential struct {
	ID     string    `json:"id"`
	Linked time.Time `json:"linked"`
}

func (s Store) Credentials(ctx context.Context, actor Claims) ([]Credential, error) {
	return withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) ([]Credential, error) {
		if err := s.authenticate(ctx, tx, actor); err != nil {
			return nil, err
		}
		rows, err := tx.Query(ctx, `SELECT credential_ref,linked_at FROM wallet_mpc.auth_link
			WHERE subject_ref=$1 AND unlinked_at IS NULL ORDER BY linked_at,credential_ref`, actor.Subject)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var items []Credential
		for rows.Next() {
			var item Credential
			if err := rows.Scan(&item.ID, &item.Linked); err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, rows.Err()
	}, actor)
}

func (s Store) Unlink(ctx context.Context, actor Claims, credential string) error {
	_, err := withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (bool, error) {
		if err := s.authenticate(ctx, tx, actor); err != nil {
			return false, err
		}
		var count int
		err := tx.QueryRow(ctx, `SELECT count(*) FROM wallet_mpc.auth_link WHERE subject_ref=$1 AND unlinked_at IS NULL`, actor.Subject).Scan(&count)
		if err != nil {
			return false, err
		}
		if err := claimsCurrent(time.Now(), actor); err != nil {
			return false, err
		}
		if count <= 1 {
			return false, ErrConflict
		}
		tag, err := tx.Exec(ctx, `UPDATE wallet_mpc.auth_link SET unlinked_at=clock_timestamp() WHERE subject_ref=$1 AND credential_ref=$2 AND unlinked_at IS NULL`, actor.Subject, credential)
		if err == nil && tag.RowsAffected() != 1 {
			return false, ErrMissing
		}
		return err == nil, err
	}, actor)
	return err
}

func readWallet(ctx context.Context, tx pgx.Tx, subject string) (Wallet, error) {
	var w Wallet
	err := tx.QueryRow(ctx, `SELECT wallet_id,state,coalesce(active_epoch,0),public_key,profile FROM wallet_mpc.wallet
		WHERE subject_ref=$1 FOR UPDATE`, subject).Scan(&w.ID, &w.State, &w.Epoch, &w.PublicKey, &w.ProfileName)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrMissing
	}
	if w.State == "active" {
		w.Address = address(w.PublicKey)
	}
	return w, err
}

func (s Store) Wallet(ctx context.Context, actor Claims) (Wallet, error) {
	return withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (Wallet, error) {
		if err := s.authenticate(ctx, tx, actor); err != nil {
			return Wallet{}, err
		}
		w, err := readWallet(ctx, tx, actor.Subject)
		if w.State == "closed" {
			w.PublicKey = nil
		}
		profile, profileErr := s.profileFor(ctx, tx, actor)
		if err == nil && profileErr != nil {
			return Wallet{}, profileErr
		}
		w.RecoveryAvailable = (w.State == "active" || w.State == "blocked") && len(profile.Committee(w.Epoch+1)) == 3
		if err == nil && w.RecoveryAvailable && w.State == "blocked" {
			w.RecoveryAvailable, err = ordinaryRecoveryRetryAvailable(ctx, tx, w.ID)
		}
		return w, err
	}, actor)
}

const operationColumns = `o.session,o.state,o.fingerprint,o.intent,o.approval_proof,o.not_before,o.request_ref`

func readOperation(row pgx.Row) (Operation, error) {
	var op Operation
	var session, fp, intent []byte
	err := row.Scan(&session, &op.State, &fp, &intent, &op.proof, &op.NotBefore, &op.RequestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return op, ErrMissing
	}
	if err != nil {
		return op, err
	}
	if len(fp) != 32 || json.Unmarshal(session, &op.Session) != nil {
		return op, errors.New("invalid stored operation")
	}
	copy(op.Fingerprint[:], fp)
	if len(intent) > 0 {
		op.Transfer = new(Transfer)
		if err := json.Unmarshal(intent, op.Transfer); err != nil {
			return op, err
		}
	}
	return op, nil
}

func existingOperation(ctx context.Context, tx pgx.Tx, wallet, id string) (Operation, error) {
	return readOperation(tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM wallet_mpc.operation o
	WHERE o.wallet_id=$1 AND o.operation_id=$2 FOR UPDATE`, wallet, id))
}

func requestOperation(ctx context.Context, tx pgx.Tx, wallet, id string) (Operation, error) {
	return readOperation(tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM wallet_mpc.operation o
	WHERE o.wallet_id=$1 AND o.request_ref=$2 FOR UPDATE`, wallet, id))
}

func (s Store) Reserve(ctx context.Context, actor Claims, id string, kind ceremony.Kind, transfer *Transfer) (Operation, error) {
	return s.reserve(ctx, actor, id, kind, transfer, "")
}

func (s Store) reserve(ctx context.Context, actor Claims, id string, kind ceremony.Kind, transfer *Transfer, previousRequest string) (Operation, error) {
	var digest, intent [32]byte
	profileName := TestProfile
	if s.MarketPolicy != nil {
		profileName = MarketTestProfile
	}
	walletProfile := profileName
	if s.MarketPolicy != nil {
		if !s.MarketPolicy.Valid() {
			return Operation{}, ErrConflict
		}
		walletProfile += ":" + s.MarketPolicy.Deployment.Hex()[2:]
	}
	if kind == ceremony.Sign {
		if transfer == nil || transfer.Profile != profileName {
			return Operation{}, ErrConflict
		}
		var err error
		if s.MarketPolicy != nil {
			if transfer.Market == nil {
				return Operation{}, ErrConflict
			}
			intent, err = marketTransferIntent(*transfer)
		} else {
			digest, intent, err = transfer.hashes()
		}
		if err != nil {
			return Operation{}, ErrConflict
		}
	} else if kind != ceremony.DKG && kind != ceremony.Refresh || transfer != nil {
		return Operation{}, ErrConflict
	}
	return withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (Operation, error) {
		if err := s.authenticate(ctx, tx, actor); err != nil {
			return Operation{}, err
		}
		profile, err := s.profileFor(ctx, tx, actor)
		if err != nil {
			return Operation{}, err
		}
		w, err := readWallet(ctx, tx, actor.Subject)
		if errors.Is(err, ErrMissing) && kind == ceremony.DKG {
			w = Wallet{ID: uuid.NewString(), State: "creating", ProfileName: walletProfile}
			if _, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.wallet(wallet_id,subject_ref,profile,state) VALUES ($1,$2,$3,'creating')`, w.ID, actor.Subject, walletProfile); err != nil {
				return Operation{}, err
			}
		} else if err != nil {
			return Operation{}, err
		}
		if w.ProfileName != walletProfile || (kind == ceremony.Sign && s.MarketPolicy != nil && strings.ToLower(transfer.Market.User.Hex()) != w.Address) {
			return Operation{}, ErrForbidden
		}
		var previousSession string
		var linkedSuccessor *string
		if previousRequest != "" {
			previousSession, linkedSuccessor, err = checkPriorReview(ctx, tx, w, previousRequest)
			if err != nil {
				return Operation{}, err
			}
		}
		old, err := requestOperation(ctx, tx, w.ID, id)
		if err == nil {
			if old.Session.Kind != kind || old.Session.Intent != intent ||
				(previousRequest != "" && (linkedSuccessor == nil || old.Session.ID != *linkedSuccessor)) {
				return Operation{}, ErrConflict
			}
			if err := attachRecoveryDisposition(ctx, tx, w, &old); err != nil {
				return Operation{}, err
			}
			return old, nil
		}
		if !errors.Is(err, ErrMissing) {
			return Operation{}, err
		}
		if linkedSuccessor != nil {
			return Operation{}, ErrConflict
		}
		now := time.Now().UTC()
		if err := claimsCurrent(now, actor); err != nil {
			return Operation{}, err
		}
		var deadline int64
		if kind == ceremony.Sign && s.MarketPolicy != nil {
			reviewed, _, err := ReviewMarketTransfer(s.MarketPolicy, *transfer, now)
			if err != nil {
				return Operation{}, ErrConflict
			}
			digest, deadline = reviewed.Digest, int64(reviewed.Deadline)
		}
		op := Operation{State: "reserved", Transfer: transfer, NotBefore: now}
		sessionID := id
		if s.Bindings != nil && actor.Subject != s.Bindings.LegacySubject {
			sessionID = uuid.NewString()
		}
		op.Session = ceremony.Session{ID: sessionID, Wallet: w.ID, Kind: kind, Intent: intent, Digest: digest}
		switch kind {
		case ceremony.DKG:
			if w.State != "creating" && w.State != "failed" {
				return Operation{}, ErrConflict
			}
			var pending bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wallet_mpc.operation WHERE wallet_id=$1 AND state IN ('reserved','running'))`, w.ID).Scan(&pending); err != nil {
				return Operation{}, err
			}
			if pending {
				return Operation{}, ErrConflict
			}
			if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.wallet SET state='creating' WHERE wallet_id=$1`, w.ID); err != nil {
				return Operation{}, err
			}
			op.Session.Epoch, op.Session.Peers, op.Session.Expires = 1, profile.First, now.Add(20*time.Minute).Unix()
		case ceremony.Sign, ceremony.Refresh:
			if w.State != "active" {
				if kind != ceremony.Refresh || w.State != "blocked" {
					return Operation{}, ErrConflict
				}
				if previousSession == "" {
					available, err := ordinaryRecoveryRetryAvailable(ctx, tx, w.ID)
					if err != nil {
						return Operation{}, err
					}
					if !available {
						return Operation{}, ErrConflict
					}
				}
			}
			var source string
			var savedSession []byte
			err = tx.QueryRow(ctx, `SELECT k.operation_id,o.session FROM wallet_mpc.key_epoch k JOIN wallet_mpc.operation o USING(operation_id)
			    WHERE k.wallet_id=$1 AND k.epoch=$2 AND k.retired_at IS NULL`, w.ID, w.Epoch).Scan(&source, &savedSession)
			if err != nil {
				return Operation{}, err
			}
			var previous ceremony.Session
			if err := json.Unmarshal(savedSession, &previous); err != nil {
				return Operation{}, err
			}
			op.Session.KeySession = source
			if kind == ceremony.Sign {
				op.Session.Epoch, op.Session.Peers, op.Session.Expires = w.Epoch, signers(previous.Peers, w.Epoch), now.Add(3*time.Minute).Unix()
				if deadline > 0 {
					op.Session.Expires = min(deadline, op.Session.Expires)
				}
			} else {
				next := profile.Committee(w.Epoch + 1)
				if len(next) != 3 {
					return Operation{}, ErrConflict
				}
				var reused bool
				ids := []string{next[0].ID, next[1].ID, next[2].ID}
				err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wallet_mpc.key_epoch k JOIN wallet_mpc.operation o USING(operation_id),
				    jsonb_array_elements(o.session->'Peers') AS p WHERE k.wallet_id=$1 AND p->>'ID'=ANY($2))`, w.ID, ids).Scan(&reused)
				if err != nil {
					return Operation{}, err
				}
				if reused {
					return Operation{}, ErrConflict
				}
				op.Session.Epoch = w.Epoch + 1
				for _, p := range previous.Peers {
					if p.Epoch == w.Epoch && p.Role != ceremony.A {
						op.Session.Peers = append(op.Session.Peers, p)
					}
				}
				op.Session.Peers = append(op.Session.Peers, next...)
				if err := claimsCurrent(time.Now(), actor); err != nil {
					return Operation{}, err
				}
				if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.wallet SET state='blocked' WHERE wallet_id=$1`, w.ID); err != nil {
					return Operation{}, err
				}
				if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.operation SET state='failed',approval_proof=NULL
					WHERE wallet_id=$1 AND kind='sign' AND state IN ('reserved','running')`, w.ID); err != nil {
					return Operation{}, err
				}
				var frozenAt time.Time
				if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&frozenAt); err != nil {
					return Operation{}, err
				}
				op.NotBefore = frozenAt.Add(profile.RecoveryDelay)
				op.Session.Expires = frozenAt.Add(29 * time.Minute).Unix()
			}
		}
		now = time.Now()
		if err := claimsCurrent(now, actor); err != nil {
			return Operation{}, err
		}
		if deadline > 0 && deadline <= now.Unix() {
			return Operation{}, ErrConflict
		}
		op.Fingerprint, err = op.Session.Fingerprint(now)
		if err != nil {
			return Operation{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.operation
			(operation_id,wallet_id,kind,epoch,session,fingerprint,state,intent,not_before,expires_at,request_ref)
			VALUES ($1,$2,$3,$4,$5,$6,'reserved',$7,$8,$9,$10) ON CONFLICT DO NOTHING`, sessionID, w.ID, kind, op.Session.Epoch, op.Session, op.Fingerprint[:], op.Transfer, op.NotBefore, time.Unix(op.Session.Expires, 0), id); err != nil {
			return Operation{}, err
		}
		saved, err := requestOperation(ctx, tx, w.ID, id)
		if err != nil {
			return Operation{}, ErrConflict
		}
		if previousSession != "" {
			result, err := tx.Exec(ctx, `UPDATE wallet_mpc.recovery_review SET successor_operation=$2
				WHERE operation_id=$1 AND successor_operation IS NULL`, previousSession, sessionID)
			if err != nil {
				return Operation{}, err
			}
			if result.RowsAffected() != 1 {
				return Operation{}, ErrConflict
			}
		}
		if kind == ceremony.Refresh {
			if err := audit(ctx, tx, w.ID, sessionID, "recovery_started"); err != nil {
				return Operation{}, err
			}
		}
		if deadline > 0 && deadline <= time.Now().Unix() {
			return Operation{}, ErrConflict
		}
		return saved, nil
	}, actor)
}

func (p Profile) Committee(epoch uint64) []ceremony.Peer {
	switch epoch {
	case 0:
		return nil
	case 1:
		return p.First
	case 2:
		return p.Replacement
	default:
		if epoch-3 < uint64(len(p.Future)) {
			return p.Future[epoch-3]
		}
		return nil
	}
}

func signers(group []ceremony.Peer, epoch uint64) []ceremony.Peer {
	result := make([]ceremony.Peer, 0, 2)
	for _, p := range group {
		if p.Epoch == epoch && (p.Role == ceremony.A || p.Role == ceremony.B) {
			result = append(result, p)
		}
	}
	return result
}

func audit(ctx context.Context, tx pgx.Tx, wallet, id, event string) error {
	_, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.audit_outbox(wallet_id,operation_id,event_kind)
	VALUES ($1,nullif($2,'')::uuid,$3)`, wallet, id, event)
	return err
}

func (s Store) Approve(ctx context.Context, actor Claims, id string, proof []byte, approval, recovery ed25519.PublicKey) (Operation, error) {
	return withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (Operation, error) {
		if err := s.authenticate(ctx, tx, actor); err != nil {
			return Operation{}, err
		}
		w, err := readWallet(ctx, tx, actor.Subject)
		if err != nil {
			return Operation{}, err
		}
		op, err := requestOperation(ctx, tx, w.ID, id)
		if err != nil {
			return Operation{}, err
		}
		approval, recovery, err = s.approvalFor(ctx, tx, actor, approval, recovery)
		if err != nil {
			return Operation{}, err
		}
		key := approval
		allowedState := "active"
		switch op.Session.Kind {
		case ceremony.DKG:
			allowedState = "creating"
		case ceremony.Refresh:
			allowedState = "blocked"
			key = recovery
		}
		now := time.Now()
		if claimsCurrent(now, actor) != nil || !ed25519.Verify(key, ceremony.ApprovalMessage(op.Session.Kind, op.Fingerprint), proof) || w.State != allowedState ||
			now.Before(op.NotBefore) || now.Unix() >= op.Session.Expires || op.Session.Kind == ceremony.Sign && op.Session.Epoch != w.Epoch {
			return Operation{}, ErrForbidden
		}
		if op.State == "running" {
			return op, nil
		}
		if op.State != "reserved" {
			return Operation{}, ErrConflict
		}
		if _, err := tx.Exec(ctx, `UPDATE wallet_mpc.operation SET state='running',approval_proof=$2 WHERE operation_id=$1`, op.Session.ID, proof); err != nil {
			return Operation{}, err
		}
		if time.Now().Unix() >= op.Session.Expires {
			return Operation{}, ErrForbidden
		}
		op.State = "running"
		return op, nil
	}, actor)
}

func (s Store) Operation(ctx context.Context, actor Claims, id string) (Operation, error) {
	return withClaimsTx(ctx, s.Pool, func(tx pgx.Tx) (Operation, error) {
		if err := s.authenticate(ctx, tx, actor); err != nil {
			return Operation{}, err
		}
		w, err := readWallet(ctx, tx, actor.Subject)
		if err != nil {
			return Operation{}, err
		}
		op, err := requestOperation(ctx, tx, w.ID, id)
		if err != nil {
			return Operation{}, err
		}
		err = tx.QueryRow(ctx, `SELECT count(*) FROM wallet_mpc.participant_ack WHERE operation_id=$1 AND evidence_kind <>'fenced'`, op.Session.ID).Scan(&op.Acknowledged)
		if err != nil {
			return Operation{}, err
		}
		if err := attachRecoveryDisposition(ctx, tx, w, &op); err != nil {
			return Operation{}, err
		}

		if op.Session.Kind == ceremony.Sign && op.State == "succeeded" && w.State == "active" && w.Epoch == op.Session.Epoch {
			var recoveryID int16
			err = tx.QueryRow(ctx, `SELECT signature,recovery_id FROM wallet_mpc.signature_result
				WHERE operation_id=$1 AND expires_at>clock_timestamp()`, op.Session.ID).Scan(&op.Signature, &recoveryID)
			if err == nil {
				parity := byte(recoveryID)
				op.RecoveryID = &parity
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return Operation{}, err
			}
		}
		return op, nil
	}, actor)
}

func AppendClose(wallet string, epoch uint64, requests ...string) []byte {
	var request string
	if len(requests) != 0 {
		request = requests[0]
	}
	raw, _ := json.Marshal(struct {
		Wallet  string
		Epoch   uint64
		Request string `json:",omitempty"`
	}{wallet, epoch, request})
	return raw
}
