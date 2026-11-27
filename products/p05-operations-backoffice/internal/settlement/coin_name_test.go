package settlement

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/orderescrow"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
)

// Every signature below is over the coin's address. Only the name it was made for decides.
func TestCoinBranchTakesOnlyTheCoinsName(t *testing.T) {
	target := Target{Contract: common.HexToAddress("0x1002"), Asset: commerce.StableNetCoin, ABI: EscrowABIHash, Token: testCoin}
	quote, err := escrowQuote(t, target, 1)
	if err != nil {
		t.Fatal(err)
	}
	board, confirmer, home := fixtureSigner(102), fixtureSigner(120).Address(), common.HexToAddress("0x1005")
	amount := new(big.Int).Mul(big.NewInt(34_200), big.NewInt(1_000_000_000_000_000_000))
	nonce := common.HexToHash("0x77")
	const deadline = 1_800_000_500
	relay, err := commerce.NewCipher(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	forwarder := Forwarder{RelayCipher: relay}
	for name, ok := range map[string]bool{"WKRC": true, "KRWC": false, "NU54 Test KRW": false, "NU54 Test USD": false, "WKRC ": false, "wkrc": false} {
		lock, err := orderescrow.NewLockFor(name, quote.Unsigned, confirmer)
		if err != nil {
			if ok {
				t.Fatal(err)
			}
			continue // not a name at all
		}
		signature, err := board.SignDigest(context.Background(), lock.Digest)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := verifyLock(quote, target, confirmer, signature); (err == nil) != ok {
			t.Errorf("an order signed for %q: %v", name, err)
		}
		digest, err := orderescrow.ReturnDigestFor(name, target.Asset, board.Address(), home, amount, deadline, nonce)
		if err != nil {
			t.Fatal(err)
		}
		signature, err = board.SignDigest(context.Background(), digest)
		if err != nil {
			t.Fatal(err)
		}
		bound := service.Bound{IntentDigest: strings.Repeat("11", 32), Request: service.Request{Body: map[string]string{"payer": lower(board.Address()), "to": lower(home), "amount": amount.String(),
			"validBefore": "1800000500", "nonce": rawHex(nonce[:])}}}
		stored := 0
		store := func([]any) error { stored++; return nil }
		if err = forwarder.storeTransfer(context.Background(), bound, target, signature, store); (err == nil) != ok || (stored == 1) != ok {
			t.Errorf("a return signed for %q: stored %d, %v", name, stored, err)
		}
		stored = 0
		// The amount the board counted, without the unit, is another transfer.
		bound.Request.Body["amount"] = "3420000"
		if err := forwarder.storeTransfer(context.Background(), bound, target, signature, store); err != service.Mismatch || stored != 0 {
			t.Errorf("a return of %q in board units: %v", name, err)
		}
	}
}
