package commerce

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestRentalSlotOpenMatchesP01(t *testing.T) {
	host := os.Getenv("P05_W12_SLOT_OPEN_HOST")
	if host == "" {
		t.Skip("set P05_W12_SLOT_OPEN_HOST to the P01 test_session executable")
	}
	deployment := core.Deployment{ChainID: 8283, Settlement: common.HexToAddress("0x51")}
	registration, measurement, manifest := common.HexToHash("0x54"), common.HexToHash("0x52"), common.HexToHash("0x55")
	account := common.HexToAddress("0x56")
	binding := rentalCollectorBinding("00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000003",
		1, deployment, account.Hex(), registration[:], measurement[:], manifest[:])
	operator, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	board, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	scope := rentalv1.AuthorityResponse{
		Operator:    crypto.PubkeyToAddress(operator.PublicKey).Hex(),
		BoardSigner: crypto.PubkeyToAddress(board.PublicKey).Hex(),
		Account:     account.Hex(),
	}
	now := time.Now()
	evidence := rentalCredentialEvidence{revision: 1, policy: 1, issuedAt: uint64(now.Unix() - 2),
		endsAt: uint64(now.Unix() + 60), challenge: [32]byte{0x72}}
	issuer, _ := evidence.receiptDigests(binding, slotOpenReceiptType)
	issuerSignature, err := crypto.Sign(issuer[:], operator)
	if err != nil {
		t.Fatal(err)
	}
	issuerSignature[64] += 27
	evidence.issuerSignature = [65]byte(issuerSignature)
	var first [32]byte
	var times [16]byte
	binary.BigEndian.PutUint64(times[:8], evidence.issuedAt)
	binary.BigEndian.PutUint64(times[8:], evidence.endsAt)
	input := bytes.Join([][]byte{binding[:], first[:], common.HexToAddress(scope.Operator).Bytes(), issuerSignature,
		crypto.FromECDSA(board), evidence.challenge[:], times[:], {0}}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	probe := func(mode string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, host, mode)
		cmd.Stdin = bytes.NewReader(input)
		return cmd.Output()
	}
	raw, err := probe("--w12-slot-open-fixture")
	if err != nil || len(raw) != 73 {
		t.Fatalf("P01 slot observation unavailable: %v", err)
	}
	evidence.checkedAt = binary.BigEndian.Uint64(raw[:8])
	evidence.boardSignature = [65]byte(raw[8:])
	if evidence.checkedAt < uint64(now.Unix()) || evidence.checkedAt > uint64(time.Now().Unix()) ||
		!evidence.signaturesValid(binding, scope, slotOpenReceiptType) ||
		evidence.signaturesValid(binding, scope, credentialReceiptType) {
		t.Fatal("P01 open observation disagrees with Go type or checked time")
	}
	for _, mode := range []string{"--w12-slot-open-fixture", "--w12-credential-fixture"} {
		input[len(input)-1] = 1
		raw, err = probe(mode)
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 2 || len(raw) != 0 {
			t.Fatalf("P01 frozen generation emitted evidence in %s", mode)
		}
	}
}

func TestRentalSlotOpenCustomerScope(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	actor := service.Principal{Role: "customer", Tenant: "tenant", Subject: "customer"}
	scope := renterCredentialScope{ScopeDigest: "verified", ValidUntil: now.Unix() + 15}
	customer := service.VerifiedClient{Fingerprint: [32]byte{1}, ExpiresAt: now.Add(10 * time.Second)}
	collector := service.VerifiedClient{Fingerprint: [32]byte{2}, ExpiresAt: now.Add(time.Minute)}
	writer := [32]byte{3}
	for _, name := range []string{"current", "absent", "revoked", "reassigned", "authority_rotated", "expired", "same_identity"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			peer := customer
			clock := now
			if name == "same_identity" {
				peer.Fingerprint = collector.Fingerprint
			}
			read := func(_ context.Context, principal service.Principal, assignment string, audience, issuedBy [32]byte) (renterCredentialScope, error) {
				calls++
				if principal.Tenant != actor.Tenant || principal.Subject != actor.Subject || assignment != "original" ||
					audience != collector.Fingerprint || issuedBy != writer {
					t.Fatal("lookup escaped authenticated assignment scope")
				}
				current := scope
				switch {
				case name == "absent", name == "revoked" && calls == 2:
					return renterCredentialScope{}, nil
				case name == "reassigned" && calls == 2:
					current.Generation = "2"
				case name == "authority_rotated" && calls == 2:
					current.ScopeDigest = "replacement"
				case name == "expired" && calls == 2:
					clock = now.Add(10 * time.Second)
				}
				return current, nil
			}
			result, ok := verifyRenterSlotOpen(context.Background(), actor, "original", peer, collector, writer, read,
				func(_ context.Context, actual renterCredentialScope) (rentalObservation, bool) {
					if actual != scope || calls != 1 {
						t.Fatal("requested evidence without a current scoped assignment")
					}
					return rentalObservation{rentalCredentialTerm{endsAt: now.Unix() + 80, validUntil: now.Unix() + 12}, now.Unix()}, true
				}, func() time.Time { return clock })
			if ok != (name == "current") || !ok && result != (renterSlotOpenObservation{}) {
				t.Fatal("invalid scope returned an open observation")
			}
			if ok && result != (renterSlotOpenObservation{now.Unix(), now.Unix() + 10, scope}) {
				t.Fatal("result escaped the customer certificate deadline")
			}
		})
	}
}

func TestProjectRenterSlotOpenKeepsOriginalUnlessCurrent(t *testing.T) {
	now := time.Unix(1000, 0)
	base := []byte(`{"slot":"unconfirmed","pendingCount":1,"amount":1000000000000000000000001}`)
	scope := renterCredentialScope{ScopeDigest: "current", ValidUntil: now.Unix() + 10}
	open := renterSlotOpenObservation{now.Unix(), now.Unix() + 10, scope}
	for _, rejected := range []struct {
		scope renterCredentialScope
		cap   int64
		time  time.Time
	}{
		{renterCredentialScope{}, now.Unix() + 10, now},
		{scope, now.Unix(), now},
		{scope, now.Unix() + 10, now.Add(10 * time.Second)},
		{scope, now.Unix() + 10, now.Add(-time.Second)},
	} {
		if got := projectRenterSlotOpen(base, open, rejected.scope, rejected.cap, rejected.time); !bytes.Equal(got, base) {
			t.Fatal("rejected slot observation changed the original wire")
		}
	}
	got := projectRenterSlotOpen(base, open, scope, now.Unix()+5, now)
	if !bytes.Contains(got, []byte(`"slot":"open"`)) ||
		!bytes.Contains(got, []byte(`"slotEvidenceUntil":"1970-01-01T00:16:45Z"`)) ||
		!bytes.Contains(got, []byte(`"amount":1000000000000000000000001`)) {
		t.Fatal("projection changed original values or exceeded final customer validity")
	}
}
