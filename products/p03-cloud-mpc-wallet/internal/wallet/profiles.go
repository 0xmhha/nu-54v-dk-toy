package wallet

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/jackc/pgx/v5"
)

const ProfileDomain = "nu54/p03/account-profile/v1\x00"

type AccountProfile struct {
	Subject, Issuer, Operator string
	ApprovalKey, RecoveryKey  ed25519.PublicKey
	Committees                [][]ceremony.Peer
	Endpoints                 map[string]ceremony.Endpoint
	DelaySeconds              uint64
}

type Bindings struct {
	LegacyEndpoints                                     map[string]ceremony.Endpoint
	LegacyApproval, LegacyRecovery                      ed25519.PublicKey
	Directory                                           string
	Authority, IdentityKey, CoordinatorKey, OperatorKey ed25519.PublicKey
	LegacySubject                                       string
}

type profileEnvelope struct{ Payload, Signature []byte }

var pinPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (p AccountProfile) profile() Profile {
	r := Profile{First: p.Committees[0], RecoveryDelay: time.Duration(p.DelaySeconds) * time.Second}
	if len(p.Committees) > 1 {
		r.Replacement = p.Committees[1]
		r.Future = p.Committees[2:]
	}
	return r
}

func (b *Bindings) load(actor Claims) (AccountProfile, [32]byte, error) {
	var p AccountProfile
	var digest [32]byte
	if !canonicalUUID(actor.Subject) || len(b.Authority) != 32 {
		return p, digest, ErrForbidden
	}
	root, err := os.OpenRoot(b.Directory)
	if err != nil {
		return p, digest, err
	}
	defer root.Close()
	f, err := root.Open(actor.Subject + ".json")
	if errors.Is(err, os.ErrNotExist) {
		return p, digest, ErrForbidden
	}
	if err != nil {
		return p, digest, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 65537))
	var e profileEnvelope
	if err != nil || len(raw) > 65536 || decodeStrict(raw, &e) != nil || !ed25519.Verify(b.Authority, append([]byte(ProfileDomain), e.Payload...), e.Signature) ||
		decodeStrict(e.Payload, &p) != nil || p.Subject != actor.Subject || p.Issuer != actor.Issuer || !b.valid(p) {
		return p, digest, ErrForbidden
	}
	return p, sha256.Sum256(e.Payload), nil
}

func (b *Bindings) valid(p AccountProfile) bool {
	if len(p.ApprovalKey) != 32 || len(p.RecoveryKey) != 32 || p.ApprovalKey.Equal(p.RecoveryKey) ||
		len(p.Committees) == 0 || len(p.Committees) > 16 || p.DelaySeconds < 60 || p.DelaySeconds > 600 ||
		p.Operator != "" && (!canonicalUUID(p.Operator) || p.Operator == p.Subject) {
		return false
	}
	for _, k := range []ed25519.PublicKey{b.Authority, b.IdentityKey, b.CoordinatorKey, b.OperatorKey, b.LegacyApproval, b.LegacyRecovery} {
		if p.ApprovalKey.Equal(k) || p.RecoveryKey.Equal(k) {
			return false
		}
	}
	ids, certs, keys := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i, peers := range p.Committees {
		epoch := uint64(i + 1)
		if len(peers) != 3 {
			return false
		}
		now := time.Now()
		s := ceremony.Session{ID: "55da9223-d9e7-432d-8861-d0f96d788a18", Wallet: "dca934da-ed05-4db2-b4bd-2b095e14ab80", Kind: ceremony.DKG, Epoch: epoch, Expires: now.Add(time.Minute).Unix(), Peers: peers}
		if _, err := s.Fingerprint(now); err != nil {
			return false
		}
		for _, peer := range peers {
			e, ok := p.Endpoints[peer.ID]
			if !ok || ids[peer.ID] || !ceremony.ValidEndpoint(e) || !pinPattern.MatchString(e.CertificateSHA256) || !pinPattern.MatchString(e.SPKISHA256) || certs[e.CertificateSHA256] || keys[e.SPKISHA256] {
				return false
			}
			for legacyID, legacy := range b.LegacyEndpoints {
				if peer.ID == legacyID || e.CertificateSHA256 == legacy.CertificateSHA256 || e.SPKISHA256 == legacy.SPKISHA256 || endpointOrigin(e) == endpointOrigin(legacy) {
					return false
				}
			}
			ids[peer.ID], certs[e.CertificateSHA256], keys[e.SPKISHA256] = true, true, true
		}
	}
	return len(ids) == len(p.Endpoints)
}

func endpointOrigin(e ceremony.Endpoint) string {
	u, err := url.Parse(e.URL)
	if err != nil {
		return ""
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	return "https://" + net.JoinHostPort(strings.TrimSuffix(strings.ToLower(u.Hostname()), "."), port)
}

func readBinding(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, subject string) (AccountProfile, error) {
	var p AccountProfile
	var raw []byte
	err := q.QueryRow(ctx, `SELECT profile FROM wallet_mpc.profile_binding WHERE subject_ref=$1`, subject).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrMissing
	}
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, err
	}
	return p, nil
}

func (s Store) bind(ctx context.Context, tx pgx.Tx, actor Claims) error {
	if s.Bindings == nil || actor.Subject == s.Bindings.LegacySubject {
		return nil
	}
	p, err := readBinding(ctx, tx, actor.Subject)
	if err == nil {
		if p.Issuer != actor.Issuer {
			return ErrForbidden
		}
		return nil
	}
	if !errors.Is(err, ErrMissing) {
		return err
	}
	var legacyWallet bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM wallet_mpc.wallet WHERE subject_ref=$1)`, actor.Subject).Scan(&legacyWallet); err != nil {
		return err
	}
	if legacyWallet {
		return ErrConflict
	}
	p, digest, err := s.Bindings.load(actor)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.profile_binding(subject_ref,issuer,profile,profile_digest) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, actor.Subject, actor.Issuer, p, digest[:])
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrConflict
	}
	for _, authority := range []ed25519.PublicKey{p.ApprovalKey, p.RecoveryKey} {
		pin := sha256.Sum256(authority)
		tag, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.authority_binding(subject_ref,authority_pin) VALUES($1,$2) ON CONFLICT DO NOTHING`, actor.Subject, pin[:])
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
	}
	for id, endpoint := range p.Endpoints {
		tag, err := tx.Exec(ctx, `INSERT INTO wallet_mpc.participant_binding(subject_ref,participant_ref,certificate_pin,public_key_pin,endpoint_origin) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, actor.Subject, id, endpoint.CertificateSHA256, endpoint.SPKISHA256, endpointOrigin(endpoint))
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrConflict
		}
	}
	return nil
}

func (s Store) profileFor(ctx context.Context, tx pgx.Tx, actor Claims) (Profile, error) {
	if s.Bindings == nil || actor.Subject == s.Bindings.LegacySubject {
		return s.Profile, nil
	}
	p, err := readBinding(ctx, tx, actor.Subject)
	if err != nil {
		return Profile{}, err
	}
	if p.Issuer != actor.Issuer {
		return Profile{}, ErrForbidden
	}
	return p.profile(), nil
}

func (s Store) approvalFor(ctx context.Context, tx pgx.Tx, actor Claims, approval, recovery ed25519.PublicKey) (ed25519.PublicKey, ed25519.PublicKey, error) {
	if s.Bindings == nil || actor.Subject == s.Bindings.LegacySubject {
		return approval, recovery, nil
	}
	p, err := readBinding(ctx, tx, actor.Subject)
	if err != nil {
		return nil, nil, err
	}
	if p.Issuer != actor.Issuer {
		return nil, nil, ErrForbidden
	}
	return p.ApprovalKey, p.RecoveryKey, nil
}

// BoundParticipants resolves an immutable endpoint set for the requested peer.
// The startup controller remains available to in-flight legacy wallet operations.
type BoundParticipants struct {
	Store                     Store
	Legacy                    Participants
	CertPath, KeyPath, CAPath string
	mu                        sync.Mutex
	controllers               map[string]*ceremony.Controller
}

func (c *BoundParticipants) resolve(ctx context.Context, id string) (Participants, error) {
	var subject string
	err := c.Store.Pool.QueryRow(ctx, `SELECT subject_ref FROM wallet_mpc.participant_binding WHERE participant_ref=$1`, id).Scan(&subject)
	if errors.Is(err, pgx.ErrNoRows) {
		return c.Legacy, nil
	}
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing := c.controllers[subject]; existing != nil {
		return existing, nil
	}
	p, err := readBinding(ctx, c.Store.Pool, subject)
	if err != nil {
		return nil, err
	}
	next, err := ceremony.NewController(c.CertPath, c.KeyPath, c.CAPath, p.Endpoints)
	if err != nil {
		return nil, err
	}
	if c.controllers == nil {
		c.controllers = make(map[string]*ceremony.Controller)
	}
	c.controllers[subject] = next
	return next, nil
}

func (c *BoundParticipants) Status(ctx context.Context, id, session string) (ceremony.Result, error) {
	client, err := c.resolve(ctx, id)
	if err != nil {
		return ceremony.Result{}, err
	}
	return client.Status(ctx, id, session)
}
func (c *BoundParticipants) Start(ctx context.Context, id string, auth ceremony.Authorization) (ceremony.Result, error) {
	client, err := c.resolve(ctx, id)
	if err != nil {
		return ceremony.Result{}, err
	}
	return client.Start(ctx, id, auth)
}
func (c *BoundParticipants) Fence(ctx context.Context, id, wallet string, epoch uint64) error {
	client, err := c.resolve(ctx, id)
	if err != nil {
		return err
	}
	return client.Fence(ctx, id, wallet, epoch)
}

// ProfilePending grants only the public CSR bootstrap, before a new profile and
// its first linked credential can be committed. A revoked member cannot reenter it.
func (s Store) ProfilePending(ctx context.Context, actor Claims) (bool, error) {
	if err := claimsCurrent(time.Now(), actor); err != nil {
		return false, err
	}
	if s.Bindings == nil || actor.Subject == s.Bindings.LegacySubject {
		return false, nil
	}
	var pending bool
	err := s.Pool.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM wallet_mpc.identity WHERE subject_ref=$1)`, actor.Subject).Scan(&pending)
	if expiry := claimsCurrent(time.Now(), actor); expiry != nil {
		return false, expiry
	}
	return pending, err
}
