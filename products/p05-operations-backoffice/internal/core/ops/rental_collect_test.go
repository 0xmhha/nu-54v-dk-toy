package ops

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type collectorFixtureIssuer func(context.Context, ble.ProtectedRentalChallenge) (RentalCollectorGrant, error)

func (f collectorFixtureIssuer) Issue(ctx context.Context, c ble.ProtectedRentalChallenge) (RentalCollectorGrant, error) {
	return f(ctx, c)
}

type collectorFixtureLink struct {
	rx     protocol.Reassembler
	tx     protocol.FrameWriter
	notes  chan []byte
	handle func(protocol.Message) protocol.Message
}

func (l *collectorFixtureLink) MTU() int                     { return 157 }
func (l *collectorFixtureLink) Close() error                 { return nil }
func (l *collectorFixtureLink) Notifications() <-chan []byte { return l.notes }
func (l *collectorFixtureLink) Write(fragment []byte) error {
	wire, err := l.rx.Feed(fragment)
	if err != nil || wire == nil {
		return err
	}
	request, err := protocol.DecodeMessage(wire)
	if err != nil {
		return err
	}
	response := l.handle(request)
	if response == nil {
		return fmt.Errorf("fixture link disconnected")
	}
	response["v"], response["sessionId"] = 1, "0000000000000000"
	encoded, err := protocol.EncodeMessage(response)
	if err != nil {
		return err
	}
	frames, err := l.tx.Write(encoded)
	if err != nil {
		return err
	}
	for _, frame := range frames {
		l.notes <- frame
	}
	return nil
}

func readCollectionVector(t *testing.T, name string) [][]byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "packages", "protocol", "v2", "testdata", name+".vectors")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var values [][]byte
	for _, field := range strings.Fields(string(content)) {
		value, err := hex.DecodeString(field)
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	return values
}

func TestCollectRentalJournalUsesCanonicalBoardEvidence(t *testing.T) {
	projection := readCollectionVector(t, "rjp1")[0]
	receiptParts := readCollectionVector(t, "rfr1")
	receipt := receiptParts[0]
	operator, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	scope := RentalCollectionScope{
		Binding: [208]byte(projection[4:212]), FirstNonce: [32]byte(projection[212:244]),
		BoardSigner: common.Address(receiptParts[2]), Operator: crypto.PubkeyToAddress(operator.PublicKey),
		Stream: "9107253e-60a7-4f47-95d2-253b42868872", StreamGeneration: "97a43cba-9ed4-4db7-aff6-57fb281c7cab",
	}
	domain := core.Domain{ChainID: binary.BigEndian.Uint64(scope.Binding[160:168]), VerifyingContract: common.Address(scope.Binding[168:188])}
	expires := binary.BigEndian.Uint64(receipt[180:188])
	frozen := binary.BigEndian.Uint64(receipt[172:180])
	timeInt := func(n uint64) string { return strconv.FormatUint(n, 10) }
	h := func(value []byte) string { return "0x" + hex.EncodeToString(value) }
	number := func(value []byte) string { return new(big.Int).SetBytes(value).String() }
	for _, mode := range []string{"success", "wrong operator", "tampered row", "link lost", "expired", "fresh reconnect"} {
		t.Run(mode, func(t *testing.T) {
			current := int64(frozen)
			rows := 0
			channelNonce := [32]byte{0x13}
			if mode == "fresh reconnect" {
				channelNonce[0]++
			}
			issuer := collectorFixtureIssuer(func(_ context.Context, c ble.ProtectedRentalChallenge) (RentalCollectorGrant, error) {
				if c.Nonce != channelNonce {
					t.Fatal("stale connection nonce")
				}
				authorization := RentalFreezeRequestV2{Binding: common.Hash(receipt[4:36]), Challenge: common.Hash(receipt[36:68]),
					ChannelNonce: common.Hash(c.Nonce), ExpiresAt: expires}
				if mode == "wrong operator" {
					authorization.ChannelNonce[0]++
				}
				digest, err := authorization.Digest(domain)
				if err != nil {
					t.Fatal(err)
				}
				sig, err := core.SignDigest(digest, operator)
				if err != nil {
					t.Fatal(err)
				}
				return RentalCollectorGrant{Binding: authorization.Binding, Challenge: authorization.Challenge,
					ExpiresAt: expires, Signature: sig}, nil
			})
			l := &collectorFixtureLink{notes: make(chan []byte, 32), tx: protocol.FrameWriter{MTU: 157}}
			l.handle = func(request protocol.Message) protocol.Message {
				if request["type"] == "rental.freeze.challenge" {
					return protocol.Message{"type": "rental.freeze.challenge.result", "channelNonce": h(channelNonce[:]), "expiresAt": timeInt(expires)}
				}
				if request["channelNonce"] != h(channelNonce[:]) {
					t.Fatal("board used a different link")
				}
				if _, row := request["projectionIndex"]; !row {
					return protocol.Message{"type": "rental.freeze.result", "binding": h(receipt[4:36]),
						"challenge": h(receipt[36:68]), "projectionRoot": h(receipt[68:100]), "firstNonce": number(receipt[100:132]),
						"nextNonce": number(receipt[132:164]), "entryCount": number(receipt[164:172]), "frozenAt": timeInt(frozen),
						"expiresAt": timeInt(expires), "boardSignature": h(receipt[188:])}
				}
				if mode == "link lost" {
					return nil
				}
				if mode == "expired" {
					current = int64(expires)
				}
				if request["projectionIndex"] != strconv.Itoa(rows) {
					t.Fatal("projection rows out of order")
				}
				offset := 277 + rows*126
				entry := bytes.Clone(projection[offset : offset+126])
				rows++
				if mode == "tampered row" {
					entry[125] ^= 1
				}
				return protocol.Message{"type": "rental.freeze.entry", "projectionIndex": request["projectionIndex"],
					"projectionRoot": h(receipt[68:100]), "kind": number(entry[:1]), "state": number(entry[1:2]),
					"nonce": number(entry[2:34]), "expiry": number(entry[34:42]), "merchant": h(entry[42:62]),
					"orderId": h(entry[62:94]), "approvalDigest": h(entry[94:126])}
			}
			result, err := CollectRentalJournal(context.Background(), l, issuer, scope, func() time.Time { return time.Unix(current, 0) })
			if mode != "success" && mode != "fresh reconnect" {
				if err == nil || result.Request.Projection != nil || result.Request.Receipt != nil {
					t.Fatal("released incomplete evidence")
				}
				return
			}
			if err != nil || !bytes.Equal(result.Request.Projection, projection) || !bytes.Equal(result.Request.Receipt, receipt) ||
				result.Expected.FreezeReceipt != h(receiptParts[1]) || rows != 3 {
				t.Fatalf("shared vector mismatch: %v, rows %d", err, rows)
			}
		})
	}
}
