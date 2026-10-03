package ops

// The refusal demonstrations against the shared session vectors: the bodies opsctl sends equal
// the vectors' bytes (SV-23 outside-schema requests, SV-03 forged attestation, SV-05 changed
// payout), and the device's replies are judged as the week-12 log needs them.

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

// replaySessions hands out one replayed session per open, each over its own vector steps.
func replaySessions(t *testing.T, runs ...[]vectorStep) (func(context.Context) (RefusalSession, error), []*replay) {
	var used []*replay
	i := 0
	return func(context.Context) (RefusalSession, error) {
		tr := newReplay(t, runs[i])
		i++
		used = append(used, tr)
		return paymentSession(t, tr), nil
	}, used
}

func TestRefuseUnsupportedSendsTheVectorBodies(t *testing.T) {
	sv := scenario(t, "SV-23")
	// SV-23: [2] open, [3] confirm, [4] raw transaction; [6] open, [7] confirm, [8] raw Permit.
	open, _ := replaySessions(t, []vectorStep{sv[2], sv[3], sv[4]}, []vectorStep{sv[6], sv[7], sv[8]})
	cases, err := RefuseUnsupported(context.Background(), open)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || !cases[0].Pass || !cases[1].Pass || cases[0].Reply != "error" {
		t.Fatalf("%+v", cases)
	}
}

func TestRefuseForgedSendsTheVectorBodies(t *testing.T) {
	sv03, sv05 := scenario(t, "SV-03"), scenario(t, "SV-05")
	op, _ := crypto.HexToECDSA(operatorKey)
	stranger, _ := crypto.HexToECDSA("7c852118294e51e653712a81e05800f419141751be58f605c371e15141b007a6")
	merchant, _ := crypto.HexToECDSA(merchantKey)
	domain := core.Domain{ChainID: 8283, VerifyingContract: common.HexToAddress("0xc0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0")}
	genuine, err := IssueAttestation(fixedRegistry{payout: common.HexToAddress("0x" + strings.Repeat("b1", 20))}, domain, op,
		crypto.PubkeyToAddress(merchant.PublicKey), common.HexToAddress("0x"+strings.Repeat("b1", 20)), "Cafe Test 01", 1_790_000_000, 86400)
	if err != nil {
		t.Fatal(err)
	}
	// SV-03: [2] open, [3] confirm, [4] identify with an attestation signed by the stranger.
	// SV-05: [2] open, [3] confirm, [4] identify (genuine), [5] prepare with payout b2..b2.
	open, _ := replaySessions(t, []vectorStep{sv03[2], sv03[3], sv03[4]}, []vectorStep{sv05[2], sv05[3], sv05[4], sv05[5]})
	cases, err := RefuseForged(context.Background(), open, ForgedInputs{
		Domain: domain, Genuine: genuine, Forger: stranger, OrderSigner: merchant,
		OrderID: common.HexToHash("0x" + strings.Repeat("01", 32)), Token: common.HexToAddress("0x" + strings.Repeat("d1", 20)),
		Amount: big.NewInt(4_500_000), Expiry: 1_790_000_060, ForgedPayout: common.HexToAddress("0x" + strings.Repeat("b2", 20)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 || !cases[0].Pass || !cases[1].Pass || cases[1].Reply != "payment.result" {
		t.Fatalf("%+v", cases)
	}
}

type fixedRegistry struct{ payout common.Address }

func (r fixedRegistry) MerchantStatus(common.Address) (bool, common.Address, error) { return true, r.payout, nil }
func (r fixedRegistry) PendingPayoutOf(common.Address) (common.Address, uint64, error) {
	return common.Address{}, 0, nil
}
