package ceremony

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/asn1"
	"errors"
	"log"
	"math/big"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/bnb-chain/tss-lib/v4/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v4/tss"
)

const ResultTTL = 24 * time.Hour

type Authorization struct {
	Session Session
	// Proofs are signatures from the coordinator and the independent device
	// or recovery authority over the domain-separated session fingerprint.
	Coordinator []byte
	Independent []byte
}

func ApprovalMessage(kind Kind, fingerprint [32]byte) []byte {
	message := []byte("nu54/p03/ceremony-approval/v1/" + string(kind) + "\x00")
	return append(message, fingerprint[:]...)
}

type activeRun struct {
	session     Session
	fingerprint [32]byte
	cancel      context.CancelFunc
	in          chan Frame
	secret      [32]byte
}

type Node struct {
	lifetime    context.Context
	id          string
	vault       *vault
	coordinator ed25519.PublicKey
	approval    ed25519.PublicKey
	recovery    ed25519.PublicKey
	send        func(context.Context, Frame) error
	fetchNonce  func(context.Context, string, [32]byte) ([32]byte, error)
	mu          sync.Mutex
	runs        map[[32]byte]*activeRun
	workers     sync.WaitGroup
	closing     bool
}

func newNode(lifetime context.Context, id string, v *vault, coordinator, approval, recovery ed25519.PublicKey, send func(context.Context, Frame) error, fetchNonce func(context.Context, string, [32]byte) ([32]byte, error)) (*Node, error) {
	if !peerID.MatchString(id) || len(coordinator) != 32 || len(approval) != 32 || len(recovery) != 32 ||
		coordinator.Equal(approval) || coordinator.Equal(recovery) || approval.Equal(recovery) {
		return nil, errors.New("independent participant and authorization keys are required")
	}
	return &Node{lifetime: lifetime, id: id, vault: v, coordinator: coordinator, approval: approval, recovery: recovery, send: send, fetchNonce: fetchNonce,
		runs: make(map[[32]byte]*activeRun)}, nil
}

func (n *Node) Close() error {
	n.mu.Lock()
	n.closing = true
	for _, run := range n.runs {
		run.cancel()
	}
	n.mu.Unlock()
	n.workers.Wait()
	return n.vault.close()
}

func (n *Node) Start(auth Authorization) (Result, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	var zero Result
	s := auth.Session
	fingerprint, err := s.Fingerprint(time.Now())
	if n.closing || n.lifetime.Err() != nil || err != nil {
		return zero, errors.New("session unavailable")
	}
	authority := n.approval
	if s.Kind == Refresh {
		authority = n.recovery
	}
	message := ApprovalMessage(s.Kind, fingerprint)
	if !ed25519.Verify(n.coordinator, message, auth.Coordinator) || !ed25519.Verify(authority, message, auth.Independent) {
		return zero, errors.New("approval required")
	}
	index := slices.IndexFunc(s.Peers, func(p Peer) bool { return p.ID == n.id })
	if index < 0 {
		return zero, errors.New("participant is outside the committee")
	}
	row, err := n.vault.record(s.ID)
	if err == nil {
		if row.Result.Session != fingerprint {
			return zero, errors.New("session already reserved")
		}
		return n.result(row)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return zero, err
	}
	fence, err := n.vault.fence(s.Wallet)
	if err != nil {
		return zero, err
	}
	if (s.Kind != Refresh && s.Epoch <= fence) || s.Kind == Refresh && n.isOld(s) && s.Epoch-1 != fence {
		return zero, errors.New("epoch is unavailable")
	}
	var share *keygen.LocalPartySaveData
	if s.Kind == Sign || n.isOld(s) {
		base, err := n.vault.record(s.KeySession)
		if err != nil || base.Session.Wallet != s.Wallet || base.Session.Epoch != s.Peers[index].Epoch ||
			base.Result.State != "ready" || base.Share == nil {
			return zero, errors.New("key share unavailable")
		}
		if base.Share.ShareID == nil || !slices.ContainsFunc(partyIDs(s.Peers, s.Peers[index].Epoch), func(id *tss.PartyID) bool {
			return id.Id == n.id && id.KeyInt().Cmp(base.Share.ShareID) == 0
		}) {
			return zero, errors.New("share belongs to a different participant")
		}
		share = base.Share
	}
	if len(n.runs) >= 3 {
		return zero, errors.New("participant capacity reached")
	}
	for _, run := range n.runs {
		if run.session.Wallet == s.Wallet {
			return zero, errors.New("wallet has a pending ceremony")
		}
	}
	row = record{Session: s, Result: Result{Session: fingerprint, Peer: n.id, State: "running"}}
	if err := n.vault.putRecord(row); err != nil {
		return zero, err
	}
	ctx, cancel := context.WithDeadline(n.lifetime, time.Unix(s.Expires, 0))
	run := &activeRun{session: s, fingerprint: fingerprint, cancel: cancel, in: make(chan Frame, 128)}
	if s.nonceLeader() == n.id {
		if _, err := rand.Read(run.secret[:]); err != nil {
			cancel()
			return zero, err
		}
	}
	n.runs[fingerprint] = run
	n.workers.Go(func() { n.execute(ctx, run, row, share) })
	return row.Result, nil
}

func (n *Node) isOld(s Session) bool {
	return s.Kind == Refresh && slices.ContainsFunc(s.Peers, func(p Peer) bool { return p.ID == n.id && p.Epoch == s.Epoch-1 })
}

func (n *Node) result(row record) (Result, error) {
	if row.Session.Kind == Sign {
		fence, err := n.vault.fence(row.Session.Wallet)
		if err != nil {
			return Result{}, err
		}
		if fence >= row.Session.Epoch {
			row.Result.State, row.Result.Signature, row.Result.RecoveryID = "fenced", nil, nil
			return row.Result, nil
		}
	}
	if row.Result.State == "running" && n.runs[row.Result.Session] == nil {
		row.Result.State = "interrupted"
	}
	if row.Session.Kind == Sign && time.Now().After(time.Unix(row.Session.Expires, 0).Add(ResultTTL)) {
		if row.Result.State == "signed" {
			row.Result.State, row.Result.Signature, row.Result.RecoveryID = "expired", nil, nil
			if err := n.vault.putRecord(row); err != nil {
				return Result{}, err
			}
		}
	}
	return row.Result, nil
}

func (n *Node) Status(id string) (Result, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	row, err := n.vault.record(id)
	if err != nil {
		return Result{}, err
	}
	return n.result(row)
}

func (n *Node) Fence(wallet string, epoch uint64) error {
	if !uuid.MatchString(wallet) || epoch == 0 {
		return errors.New("invalid epoch")
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	have, err := n.vault.fence(wallet)
	if err != nil || have >= epoch {
		return err
	}
	if err := n.vault.putFence(wallet, epoch); err != nil {
		return err
	}
	for _, run := range n.runs {
		if run.session.Wallet == wallet && run.session.Epoch <= epoch {
			run.cancel()
		}
	}
	return nil
}

func (n *Node) Receive(peer string, frame Frame) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	run := n.runs[frame.Session]
	if run == nil {
		return os.ErrNotExist
	}
	if len(frame.Data) > 1<<18 || frame.To != n.id || peer != frame.From || peer == n.id ||
		!slices.ContainsFunc(run.session.Peers, func(p Peer) bool { return p.ID == peer }) {
		return errors.New("invalid peer frame")
	}
	select {
	case run.in <- frame:
		return nil
	default:
		return errors.New("round inbox is full")
	}
}

func (n *Node) execute(ctx context.Context, run *activeRun, row record, share *keygen.LocalPartySaveData) {
	defer func() {
		run.cancel()
		n.mu.Lock()
		clear(run.secret[:])
		delete(n.runs, run.fingerprint)
		n.mu.Unlock()
	}()
	s := row.Session
	var pre *keygen.LocalPreParams
	if s.Kind == DKG || s.Kind == Refresh && !n.isOld(s) {
		var err error
		pre, err = prepare(ctx)
		if err != nil {
			log.Print("participant precomputation failed")
			return
		}
	}

	secret := run.secret
	if leader := s.nonceLeader(); leader != n.id {
		var err error
		secret, err = n.fetchNonce(ctx, leader, run.fingerprint)
		if err != nil {
			return
		}
	}
	e, err := newEngine(s, n.id, run.fingerprint, secret, pre, share)
	clear(secret[:])

	if err != nil {
		log.Print("participant threshold setup failed")
		return
	}
	result, err := n.rounds(ctx, run, e, share)
	if err != nil {
		log.Print("participant threshold round stopped")
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	row.Result = result.Result
	row.Share = result.Share
	// If publication is interrupted the durable running marker prevents rerun.
	_ = n.vault.putRecord(row)
}

func (n *Node) rounds(ctx context.Context, run *activeRun, e *engine, existing *keygen.LocalPartySaveData) (record, error) {
	row := record{Result: Result{Session: run.fingerprint, Peer: n.id}}
	var ids []string
	for _, p := range run.session.Peers {
		ids = append(ids, p.ID)
	}
	inbox := NewInbox(n.id, ids)
	errorsOut := make(chan error, 1)
	var outgoing sync.WaitGroup
	report := func(err error) {
		select {
		case errorsOut <- err:
		default:
		}
	}
	send := func(frame Frame) {
		outgoing.Go(func() {
			if err := n.send(ctx, frame); err != nil {
				report(err)
			}
		})
	}
	transmit := func(message tss.Message) error {
		frames, err := e.encode(message)
		if err != nil {
			return err
		}
		for _, frame := range frames {
			send(frame)
		}
		return nil
	}
	if err := e.party.Start(); err != nil {
		return row, errors.New("cannot begin threshold round")
	}
	echoes := make(map[messageKey]bool)
	var completed bool
	for !completed {
		select {
		case <-ctx.Done():
			return row, ctx.Err()
		case err := <-errorsOut:
			return row, err
		case message := <-e.out:
			if err := transmit(message); err != nil {
				return row, err
			}
		case frame := <-run.in:
			msg, err := e.parse(frame)
			if err != nil {
				return row, err
			}
			source := frame.From
			if frame.Source != "" {
				source = frame.Source
			}
			recipients, _ := e.route(source, frame.Round, frame.Broadcast)
			deliver, ready, err := inbox.receive(frame.From, frame, recipients)
			if err != nil {
				return row, err
			}
			key := messageKey{frame.From, frame.Round}
			if frame.Broadcast && frame.Source == "" && !echoes[key] {
				echoes[key] = true
				for _, id := range recipients {
					if id != n.id && id != frame.From {
						send(Frame{Session: frame.Session, From: n.id, To: id, Round: frame.Round, Broadcast: true, Source: frame.From, Digest: frame.Hash()})
					}
				}
			}
			if ready {
				if msg == nil {
					msg, err = e.parse(deliver)
				}
				if err != nil {
					return row, err
				}
				if _, err := e.party.Update(msg); err != nil {
					return row, errors.New("threshold round verification failed")
				}
			}
		case share := <-e.shares:
			if n.isOld(run.session) {
				row.Result.State, row.Result.PublicKey = "retired", publicKey(existing)
			} else {
				if share == nil || share.Xi == nil || share.ShareID == nil || share.ECDSAPub == nil || !share.ECDSAPub.ValidateBasic() {
					return row, errors.New("invalid generated share")
				}
				row.Share, row.Result.State, row.Result.PublicKey = share, "ready", publicKey(share)
			}
			completed = true
		case signature := <-e.signatures:
			r, s := new(big.Int).SetBytes(signature.R), new(big.Int).SetBytes(signature.S)
			der, err := asn1.Marshal(struct{ R, S *big.Int }{r, s})
			if err != nil || !VerifySignature(publicKey(existing), run.session.Digest, der) {
				return row, errors.New("invalid threshold signature")
			}
			row.Result.State, row.Result.Signature, row.Result.RecoveryID = "signed", der, signature.SignatureRecovery
			completed = true
		}
	}
	// A party may report completion after emitting its last required message.
	for len(e.out) > 0 {
		if err := transmit(<-e.out); err != nil {
			return row, err
		}
	}
	outgoing.Wait()
	select {
	case err := <-errorsOut:
		return row, err
	default:
		return row, nil
	}
}

func (s Session) nonceLeader() string {
	for _, p := range s.Peers {
		if (s.Kind == Refresh && p.Epoch == s.Epoch-1 && p.Role == C) ||
			(s.Kind != Refresh && p.Role == A) {
			return p.ID
		}
	}
	return ""
}

func (n *Node) Nonce(peer string, fingerprint [32]byte) ([32]byte, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	run := n.runs[fingerprint]
	if run == nil || time.Now().Unix() >= run.session.Expires {
		return [32]byte{}, os.ErrNotExist
	}
	if run.session.nonceLeader() != n.id || peer == n.id || !slices.ContainsFunc(run.session.Peers, func(p Peer) bool { return p.ID == peer }) {
		return [32]byte{}, errors.New("participant nonce unavailable")
	}
	return run.secret, nil
}
