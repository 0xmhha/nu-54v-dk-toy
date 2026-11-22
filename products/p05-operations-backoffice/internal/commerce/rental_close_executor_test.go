package commerce

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type stagedCloseStore struct {
	original     protectedCloseCandidate
	current      protectedClosePlan
	admissionErr error
	blockNew     bool
	events       *[]string
}

func (s *stagedCloseStore) recover(context.Context, protectedCloseRequest) (protectedCloseCandidate, error) {
	*s.events = append(*s.events, "recover")
	return s.original, nil
}
func (s *stagedCloseStore) plan(context.Context, protectedCloseRequest, [32]byte) (protectedClosePlan, error) {
	*s.events = append(*s.events, "plan")
	if s.blockNew {
		return protectedClosePlan{}, service.Conflict
	}
	return s.current, nil
}
func (s *stagedCloseStore) admit(_ context.Context, _ protectedCloseRequest, _ [32]byte, _ protectedClosePlan,
	_ int64, _ common.Address, c protectedCloseCandidate) (bool, error) {
	*s.events = append(*s.events, "admit")
	if s.admissionErr != nil {
		return false, s.admissionErr
	}
	s.original = c
	return true, nil
}
func (s *stagedCloseStore) mark(_ context.Context, _ protectedCloseRequest, _ [32]byte, _ protectedClosePlan, _ int64, hash common.Hash, state string) (bool, error) {
	*s.events = append(*s.events, state)
	if hash != s.original.Hash {
		return false, service.Conflict
	}
	s.original.State = state
	if state == "confirmed" || state == "reverted" || state == "superseded" {
		s.original.sealed = nil
	}
	return true, nil
}

type stagedCloseChain struct {
	destination, originalSigner common.Address
	hash                        common.Hash
	observed                    core.RentalCloseChainState
	publishErr                  error
	events                      *[]string
	sent                        []byte
}

func (c *stagedCloseChain) AccountOf(context.Context, common.Address) (core.Account, error) {
	*c.events = append(*c.events, "account")
	return core.Account{Exists: true, WithdrawAddress: c.destination}, nil
}
func (c *stagedCloseChain) PrepareRentalClose(context.Context, *ecdsa.PrivateKey, common.Address) (core.RentalCloseTransaction, error) {
	*c.events = append(*c.events, "sign")
	return core.RentalCloseTransaction{Hash: c.hash, Nonce: 7, Signed: bytes.Repeat([]byte{7}, 100)}, nil
}
func (c *stagedCloseChain) InspectRentalClose(_ context.Context, signer, _ common.Address, tx core.RentalCloseTransaction) (core.RentalCloseChainState, error) {
	*c.events = append(*c.events, "lookup")
	if tx.Hash != c.hash || tx.Nonce != 7 || signer != c.originalSigner {
		return "", service.Conflict
	}
	return c.observed, nil
}
func (c *stagedCloseChain) PublishRentalClose(_ context.Context, _ common.Address, _ common.Address, tx core.RentalCloseTransaction) error {
	*c.events = append(*c.events, "publish")
	if tx.Hash != c.hash || tx.Nonce != 7 {
		return service.Conflict
	}
	c.sent = bytes.Clone(tx.Signed)
	return c.publishErr
}

type stagedCloseAnchor struct {
	calls, deniedCall int
	events            *[]string
}

func (a *stagedCloseAnchor) CallerFingerprint() [32]byte { return [32]byte{8} }
func (a *stagedCloseAnchor) ConfirmCloseAnchor(_ context.Context, r rentalv1.CloseAnchorRequest) (rentalv1.CloseAnchorConfirmation, error) {
	*a.events = append(*a.events, "p07")
	a.calls++
	if a.calls == a.deniedCall {
		return rentalv1.CloseAnchorConfirmation{}, service.Conflict
	}
	return rentalv1.CloseAnchorConfirmation{CloseAnchorRequest: r, Finalized: r.Previous, ExpiresAt: time.Now().Unix() + 4}, nil
}

func TestProtectedCloseExecutorPreservesExactAmbiguousTransaction(t *testing.T) {
	for _, scenario := range []string{"authority-change", "post-sign-scope-change", "lost-send-and-restart"} {
		t.Run(scenario, func(t *testing.T) {
			var events []string
			x := "10921a6c-4415-4f76-9852-bdf55395f5d8"
			y := "20921a6c-4415-4f76-9852-bdf55395f5d8"
			a := "30921a6c-4415-4f76-9852-bdf55395f5d8"
			b := "40921a6c-4415-4f76-9852-bdf55395f5d8"
			req := protectedCloseRequest{Tenant: x, Branch: y, Actor: a, Device: b, Generation: 1, Key: x, Review: strings.Repeat("a", 64)}
			digest := func(c string) string { return "0x" + strings.Repeat(c, 64) }
			dep := core.Deployment{ChainID: 8283, Settlement: common.Address{19: 4}}
			manifest := common.Hash{31: 5}
			store := &stagedCloseStore{events: &events, current: protectedClosePlan{ReviewRef: req.Review, ScopeDigest: strings.Repeat("b", 64),
				Account: common.Address{19: 6}.Hex(), Destination: common.Address{19: 7}.Hex(), Deployment: manifest.Hex(), Settlement: strings.ToLower(dep.Settlement.Hex()),
				Registration: digest("a"), Measurement: digest("b"), Challenge: digest("c"), Stream: x, StreamGeneration: y,
				Assessment: "sha256:" + strings.Repeat("d", 64), Height: 5, Hash: digest("e"), ExpiresAt: time.Now().Unix() + 60}}
			key, err := crypto.GenerateKey()
			if err != nil {
				t.Fatal(err)
			}
			cipher, err := NewCipher(bytes.Repeat([]byte{1}, 32))
			if err != nil {
				t.Fatal(err)
			}
			anchor := &stagedCloseAnchor{events: &events}
			chain := &stagedCloseChain{events: &events, destination: common.Address{19: 7}, hash: common.Hash{31: 8}, originalSigner: crypto.PubkeyToAddress(key.PublicKey)}
			runner := &protectedCloseExecutor{store: store, chain: chain, anchor: anchor, cipher: cipher, operator: key, deployment: dep, manifest: manifest, now: time.Now}
			ctx := t.Context()
			switch scenario {
			case "authority-change":
				anchor.deniedCall = 2
			case "post-sign-scope-change":
				store.admissionErr = service.Conflict
			case "lost-send-and-restart":
				chain.publishErr = context.DeadlineExceeded
			}
			got, err := runner.execute(ctx, req)
			if scenario != "lost-send-and-restart" {
				if !errors.Is(err, service.Conflict) || store.original.State != "" || chain.sent != nil || anchor.calls != 2 {
					t.Fatalf("unverified signed candidate escaped: %+v %v %v", got, err, events)
				}
				return
			}
			if err != nil || got.State != "attempting" || got.Nonce != 7 || !bytes.Equal(chain.sent, bytes.Repeat([]byte{7}, 100)) ||
				bytes.Equal(store.original.sealed, chain.sent) {
				t.Fatalf("ambiguous outcome lost original or exposed clear bytes: %+v %v %v", got, err, events)
			}
			events = nil
			store.blockNew = true
			chain.observed = core.RentalCloseSubmitted
			restarted := *runner
			got, err = restarted.execute(ctx, req)
			if err != nil || got.State != "submitted" || got.Hash != chain.hash || len(got.sealed) == 0 ||
				strings.Join(events, ",") != "recover,lookup,submitted,recover" {
				t.Fatalf("accepted replay did not reconcile hash first: %+v %v %v", got, err, events)
			}
			events = nil
			chain.observed = core.RentalCloseNotObserved
			rotated, err := crypto.GenerateKey()
			if err != nil {
				t.Fatal(err)
			}
			restarted.operator = rotated
			got, err = restarted.execute(ctx, req)
			if !errors.Is(err, service.Conflict) || got.State != "submitted" || len(got.sealed) == 0 ||
				strings.Join(events, ",") != "recover,lookup" {
				t.Fatalf("rotated signer lost or reissued original: %+v %v %v", got, err, events)
			}
			restarted.operator = key
			events = nil
			got, err = restarted.execute(ctx, req)
			if !errors.Is(err, service.Conflict) || got.State != "submitted" || len(got.sealed) == 0 ||
				strings.Join(events, ",") != "recover,lookup,plan" {
				t.Fatalf("evicted candidate was resent or lost after authority revocation: %+v %v %v", got, err, events)
			}
			chain.observed = core.RentalCloseConfirmed
			got, err = restarted.execute(ctx, req)
			if err != nil || got.State != "confirmed" || got.Hash != chain.hash || got.sealed != nil {
				t.Fatalf("final canonical original was not retained: %+v %v", got, err)
			}
			events = nil
			if _, err := restarted.execute(ctx, req); err != nil || strings.Join(events, ",") != "recover" {
				t.Fatalf("terminal replay called external services: %v %v", err, events)
			}
		})
	}
}
