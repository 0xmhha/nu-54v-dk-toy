package ops

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

// The refusal demonstrations of the week-12 log (W12-05): requests the device must refuse,
// sent from the operator tool so that the kiosk's release build carries no way to make them
// (P05 design 4). Each case runs in a fresh payment session.

// RefusalSession is the part of a device session the demonstrations use (ble.Session).
type RefusalSession interface {
	Confirm(ctx context.Context) error
	Request(ctx context.Context, m protocol.Message, wait time.Duration) (protocol.Message, error)
	SendRaw(ctx context.Context, body []byte, wait time.Duration) (protocol.Message, error)
	SessionID() string
	Close() error
}

// RefusalCase is one request and what the device answered.
type RefusalCase struct {
	Case     string `json:"case"`
	Expected string `json:"expected"`
	Got      string `json:"got"`
	Reply    string `json:"reply"` // message type of the answer
	Pass     bool   `json:"pass"`
}

const refusalWait = 5 * time.Second

// RawMap encodes a flat map of text keys to uint64, string or []byte as deterministic CBOR
// (RFC 8949 4.2.1). It builds bodies the schema does not allow, which the shared encoder refuses.
func RawMap(fields map[string]any) []byte {
	head := func(major byte, n uint64) []byte {
		switch {
		case n < 24:
			return []byte{major<<5 | byte(n)}
		case n < 1<<8:
			return []byte{major<<5 | 24, byte(n)}
		default:
			return []byte{major<<5 | 25, byte(n >> 8), byte(n)}
		}
	}
	type kv struct{ k, v []byte }
	var items []kv
	for k, v := range fields {
		var enc []byte
		switch x := v.(type) {
		case int:
			enc = head(0, uint64(x))
		case string:
			enc = append(head(3, uint64(len(x))), x...)
		case []byte:
			enc = append(head(2, uint64(len(x))), x...)
		default:
			panic(fmt.Sprintf("RawMap: %T", v))
		}
		items = append(items, kv{append(head(3, uint64(len(k))), k...), enc})
	}
	sort.Slice(items, func(i, j int) bool { return bytes.Compare(items[i].k, items[j].k) < 0 })
	out := head(5, uint64(len(items)))
	for _, it := range items {
		out = append(append(out, it.k...), it.v...)
	}
	return out
}

// Outside-schema requests (payment-protocol.md 2: the device signs two EIP-712 types only).
func rawTransactionRequest(sid []byte) []byte {
	return RawMap(map[string]any{"v": 1, "type": "sign.transaction", "sessionId": sid,
		"tx": common.FromHex("0x02f86f82205b0180808094" + strings.Repeat("dd", 20) + "8080c0")})
}

func rawPermitRequest(sid []byte) []byte {
	return RawMap(map[string]any{"v": 1, "type": "sign.permit", "sessionId": sid,
		"spender": common.FromHex("0x" + strings.Repeat("ee", 20)), "value": 1000000})
}

func judge(c RefusalCase, m protocol.Message, err error) RefusalCase {
	switch {
	case err != nil:
		c.Got = err.Error()
	default:
		c.Reply, _ = m["type"].(string)
		c.Got = fmt.Sprint(m["reason"])
		c.Pass = c.Got == c.Expected
	}
	return c
}

// RefuseUnsupported sends a raw-transaction and a Permit signing request, each in a confirmed
// payment session; the device must answer error{UNSUPPORTED_TYPE} (W12-05 UNSUPPORTED_TYPE).
func RefuseUnsupported(ctx context.Context, open func(context.Context) (RefusalSession, error)) ([]RefusalCase, error) {
	var out []RefusalCase
	for _, rc := range []struct {
		name string
		body func([]byte) []byte
	}{{"raw transaction signing request", rawTransactionRequest}, {"Permit signing request", rawPermitRequest}} {
		s, err := open(ctx)
		if err != nil {
			return out, err
		}
		if err := s.Confirm(ctx); err != nil {
			s.Close()
			return out, err
		}
		m, err := s.SendRaw(ctx, rc.body(common.FromHex("0x"+s.SessionID())), refusalWait)
		out = append(out, judge(RefusalCase{Case: rc.name, Expected: "UNSUPPORTED_TYPE"}, m, err))
		s.Close()
	}
	return out, nil
}

// ForgedInputs are the pieces of the MERCHANT_FORGED demonstration.
type ForgedInputs struct {
	Domain core.Domain
	// Genuine is an operator-signed attestation for the kiosk's merchant (opsctl attestation issue).
	Genuine Attestation
	// Forger signs the fake attestation in place of the operator. OrderSigner signs the order
	// (default Forger: opsctl does not hold the merchant key, and the device refuses either way).
	Forger      *ecdsa.PrivateKey
	OrderSigner *ecdsa.PrivateKey
	// The order: its payout is changed from the attestation's.
	OrderID      common.Hash
	Token        common.Address
	Amount       *big.Int
	Expiry       uint64
	ForgedPayout common.Address
}

func attestationMessage(a Attestation) map[string]any {
	return map[string]any{"merchant": strings.ToLower(a.Merchant.Hex()), "payout": strings.ToLower(a.Payout.Hex()), "name": a.Name,
		"validFrom": a.ValidFrom, "validUntil": a.ValidUntil, "operatorSignature": hexutil.Encode(a.OperatorSignature)}
}

// RefuseForged shows the device refusing a merchant the operator did not sign, and an order
// whose payout differs from the attestation (W12-05 MERCHANT_FORGED, device layer).
func RefuseForged(ctx context.Context, open func(context.Context) (RefusalSession, error), in ForgedInputs) ([]RefusalCase, error) {
	var out []RefusalCase
	// 1. The genuine attestation's fields, signed by the forger instead of the operator.
	fake := in.Genuine
	digest, err := core.Digest(in.Domain, "MerchantAttestation", map[string]any{"merchant": fake.Merchant.Hex(), "payout": fake.Payout.Hex(),
		"name": fake.Name, "validFrom": fake.ValidFrom, "validUntil": fake.ValidUntil})
	if err != nil {
		return nil, err
	}
	if fake.OperatorSignature, err = core.SignDigest(digest, in.Forger); err != nil {
		return nil, err
	}
	s, err := open(ctx)
	if err != nil {
		return out, err
	}
	if err := s.Confirm(ctx); err != nil {
		s.Close()
		return out, err
	}
	m, err := s.Request(ctx, protocol.Message{"type": "payment.identify", "attestation": protocol.Message(attestationMessage(fake))}, refusalWait)
	out = append(out, judge(RefusalCase{Case: "attestation not signed by the operator", Expected: "MERCHANT_FORGED"}, m, err))
	s.Close()

	// 2. The genuine attestation, then an order to another payout than the attestation's.
	if s, err = open(ctx); err != nil {
		return out, err
	}
	defer s.Close()
	if err := s.Confirm(ctx); err != nil {
		return out, err
	}
	if m, err := s.Request(ctx, protocol.Message{"type": "payment.identify", "attestation": protocol.Message(attestationMessage(in.Genuine))}, 800*time.Millisecond); err == nil {
		return append(out, judge(RefusalCase{Case: "genuine attestation accepted", Expected: "(no reply)"}, m, nil)), nil
	}
	order := MerchantOrder{OrderID: in.OrderID, Token: in.Token, Amount: in.Amount, Payout: in.ForgedPayout, Expiry: in.Expiry}
	signer := in.OrderSigner
	if signer == nil {
		signer = in.Forger
	}
	sig, err := SignMerchantOrder(in.Domain, signer, order)
	if err != nil {
		return out, err
	}
	auth := protocol.Message{"chainId": fmt.Sprint(in.Domain.ChainID), "contract": strings.ToLower(in.Domain.VerifyingContract.Hex()),
		"merchant": strings.ToLower(in.Genuine.Merchant.Hex()), "payout": strings.ToLower(in.ForgedPayout.Hex()), "token": strings.ToLower(in.Token.Hex()),
		"amount": in.Amount.String(), "orderId": in.OrderID.Hex(), "expiry": fmt.Sprint(in.Expiry)}
	m, err = s.Request(ctx, protocol.Message{"type": "payment.prepare", "authorization": auth, "merchantSignature": hexutil.Encode(sig)}, refusalWait)
	return append(out, judge(RefusalCase{Case: "order to another payout than the attestation's", Expected: "MERCHANT_FORGED"}, m, err)), nil
}
