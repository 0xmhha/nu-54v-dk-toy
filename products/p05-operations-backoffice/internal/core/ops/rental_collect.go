package ops

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var ErrRentalCollection = errors.New("registered rental journal could not be collected")

type RentalCollectionScope struct {
	Binding                  [208]byte
	FirstNonce               [32]byte
	BoardSigner, Operator    common.Address
	Stream, StreamGeneration string
}

type RentalCollectorGrant struct {
	Binding, Challenge common.Hash
	ExpiresAt          uint64
	Signature          []byte
}

type RentalGrantIssuer interface {
	Issue(context.Context, ble.ProtectedRentalChallenge) (RentalCollectorGrant, error)
}

// CollectedRentalJournal holds only a public projection and board attestation.
type CollectedRentalJournal struct {
	Request             rentalv1.Request
	Expected            rentalv1.Binding
	Count               int
	FrozenAt, ExpiresAt time.Time
}

// CollectRentalJournal runs exactly one attempt on one assigned transport. The
// caller must discard that transport after a failure and start a new attempt
// with a fresh board nonce. No partial journal is returned.
func CollectRentalJournal(ctx context.Context, link ble.Transport, issuer RentalGrantIssuer, scope RentalCollectionScope,
	clock func() time.Time) (CollectedRentalJournal, error) {
	var empty CollectedRentalJournal
	chain := binary.BigEndian.Uint64(scope.Binding[160:168])
	settlement := common.Address(scope.Binding[168:188])
	account := common.Address(scope.Binding[188:208])
	if link == nil || issuer == nil || clock == nil || chain != 8283 || settlement == (common.Address{}) ||
		account == (common.Address{}) || scope.BoardSigner == (common.Address{}) || scope.Operator == (common.Address{}) ||
		scope.BoardSigner == account || scope.BoardSigner == scope.Operator || scope.FirstNonce[31] != 0 ||
		[24]byte(scope.Binding[136:160]) != [24]byte{} || [16]byte(scope.Binding[8:24]) == [16]byte{} ||
		[16]byte(scope.Binding[24:40]) == [16]byte{} || [32]byte(scope.Binding[40:72]) == [32]byte{} ||
		[32]byte(scope.Binding[72:104]) == [32]byte{} || [32]byte(scope.Binding[104:136]) == [32]byte{} ||
		binary.BigEndian.Uint64(scope.Binding[:8]) == 0 || scope.Stream == "" || scope.StreamGeneration == "" {
		return empty, ErrRentalCollection
	}
	domain := core.Domain{ChainID: chain, VerifyingContract: settlement}
	binding := crypto.Keccak256Hash(scope.Binding[:])
	channel, err := ble.GetProtectedRentalChallenge(ctx, link)
	if err != nil {
		return empty, ErrRentalCollection
	}
	grant, err := issuer.Issue(ctx, channel)
	now := clock().Unix()
	if err != nil || grant.Binding != binding || grant.Challenge == (common.Hash{}) || now <= 0 ||
		grant.ExpiresAt <= uint64(now) || grant.ExpiresAt-uint64(now) > 120 || grant.ExpiresAt > channel.ExpiresAt {
		return empty, ErrRentalCollection
	}
	authorization := RentalFreezeRequestV2{Binding: binding, Challenge: grant.Challenge,
		ChannelNonce: common.Hash(channel.Nonce), ExpiresAt: grant.ExpiresAt}
	digest, err := authorization.Digest(domain)
	if err != nil {
		return empty, ErrRentalCollection
	}
	operator, err := core.RecoverSigner(digest, grant.Signature)
	if err != nil || operator != scope.Operator {
		return empty, ErrRentalCollection
	}

	wire, err := ble.FreezeProtectedRental(ctx, link, grant.Challenge, channel.Nonce, grant.ExpiresAt, grant.Signature)
	if err != nil {
		return empty, ErrRentalCollection
	}
	receipt, receiptDigest, count, frozenAt, ok := readCollectionReceipt(wire, domain, scope, grant, clock().Unix())
	if !ok {
		return empty, ErrRentalCollection
	}
	root := common.Hash(receipt.Root)
	projection := make([]byte, 277, 277+126*count)
	copy(projection, "RJP1")
	copy(projection[4:], scope.Binding[:])
	copy(projection[212:244], receipt.FirstNonce[:])
	copy(projection[244:276], receipt.NextNonce[:])
	projection[276] = byte(count)
	nonce := new(big.Int).SetBytes(receipt.FirstNonce[:])
	for i := 0; i < count; i++ {
		if ctx.Err() != nil || clock().Unix() >= int64(grant.ExpiresAt) {
			return empty, ErrRentalCollection
		}
		row, err := ble.ReadProtectedRentalEntry(ctx, link, grant.Challenge, channel.Nonce, grant.ExpiresAt,
			grant.Signature, uint64(i), root)
		if err != nil {
			return empty, ErrRentalCollection
		}
		encoded, ok := encodeCollectionEntry(row, nonce)
		if !ok {
			return empty, ErrRentalCollection
		}
		projection = append(projection, encoded[:]...)
		nonce.Add(nonce, big.NewInt(1))
	}
	if ctx.Err() != nil || clock().Unix() >= int64(grant.ExpiresAt) || crypto.Keccak256Hash(projection) != root {
		return empty, ErrRentalCollection
	}
	return CollectedRentalJournal{
		Request: rentalv1.Request{Projection: projection, Receipt: receipt.Bytes()},
		Expected: rentalv1.Binding{Stream: scope.Stream, StreamGeneration: scope.StreamGeneration,
			Deployment: common.Hash(scope.Binding[104:136]).Hex(), FreezeReceipt: receiptDigest.Hex(), JournalRoot: root.Hex()},
		Count: count, FrozenAt: time.Unix(int64(frozenAt), 0), ExpiresAt: time.Unix(int64(grant.ExpiresAt), 0),
	}, nil
}

func fixedHex(value any, dst []byte) bool {
	v, ok := value.(string)
	if !ok || !strings.HasPrefix(v, "0x") || len(v) != 2+2*len(dst) || v != strings.ToLower(v) {
		return false
	}
	_, err := hex.Decode(dst, []byte(v[2:]))
	return err == nil
}

func collectionUint64(value any) (uint64, bool) {
	v, ok := value.(string)
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseUint(v, 10, 64)
	return n, err == nil && strconv.FormatUint(n, 10) == v
}

func collectionUint256(value any, dst []byte) bool {
	v, ok := value.(string)
	if !ok {
		return false
	}
	n, ok := new(big.Int).SetString(v, 10)
	if !ok || n.Sign() < 0 || n.BitLen() > 256 || n.String() != v {
		return false
	}
	n.FillBytes(dst)
	return true
}

func readCollectionReceipt(m protocol.Message, domain core.Domain, scope RentalCollectionScope, grant RentalCollectorGrant,
	now int64) (rentalv1.FreezeReceipt, common.Hash, int, uint64, bool) {
	var empty rentalv1.FreezeReceipt
	var receipt rentalv1.FreezeReceipt
	count, countOK := collectionUint64(m["entryCount"])
	frozen, frozenOK := collectionUint64(m["frozenAt"])
	expiry, expiryOK := collectionUint64(m["expiresAt"])
	if !countOK || count > 8 || !frozenOK || frozen == 0 || now <= 0 || frozen > uint64(now) ||
		!expiryOK || expiry != grant.ExpiresAt || expiry <= uint64(now) ||
		!fixedHex(m["binding"], receipt.Binding[:]) || common.Hash(receipt.Binding) != grant.Binding ||
		!fixedHex(m["challenge"], receipt.Challenge[:]) || common.Hash(receipt.Challenge) != grant.Challenge ||
		!fixedHex(m["projectionRoot"], receipt.Root[:]) || receipt.Root == [32]byte{} ||
		!collectionUint256(m["firstNonce"], receipt.FirstNonce[:]) || receipt.FirstNonce != scope.FirstNonce ||
		!collectionUint256(m["nextNonce"], receipt.NextNonce[:]) || !fixedHex(m["boardSignature"], receipt.BoardSignature[:]) {
		return empty, common.Hash{}, 0, 0, false
	}
	start, end := new(big.Int).SetBytes(receipt.FirstNonce[:]), new(big.Int).SetBytes(receipt.NextNonce[:])
	if end.Sub(end, start).Cmp(new(big.Int).SetUint64(count)) != 0 {
		return empty, common.Hash{}, 0, 0, false
	}
	receipt.EntryCount, receipt.FrozenAt, receipt.ExpiresAt = count, frozen, expiry
	digest := common.Hash(receipt.Digest(domain.ChainID, domain.VerifyingContract))
	signer, err := core.RecoverSigner(digest[:], receipt.BoardSignature[:])
	return receipt, digest, int(count), frozen, err == nil && signer == scope.BoardSigner
}

func encodeCollectionEntry(m protocol.Message, nonce *big.Int) ([126]byte, bool) {
	var row [126]byte
	kind, kindOK := collectionUint64(m["kind"])
	state, stateOK := collectionUint64(m["state"])
	expiry, expiryOK := collectionUint64(m["expiry"])
	if !kindOK || (kind != 1 && kind != 2) || !stateOK || (state != 1 && state != 2) ||
		!expiryOK || expiry == 0 || !collectionUint256(m["nonce"], row[2:34]) || new(big.Int).SetBytes(row[2:34]).Cmp(nonce) != 0 ||
		!fixedHex(m["merchant"], row[42:62]) || !fixedHex(m["orderId"], row[62:94]) ||
		!fixedHex(m["approvalDigest"], row[94:126]) || common.Hash(row[94:126]) == (common.Hash{}) {
		return row, false
	}
	merchantEmpty := common.Address(row[42:62]) == (common.Address{})
	orderEmpty := common.Hash(row[62:94]) == (common.Hash{})
	if merchantEmpty != orderEmpty || merchantEmpty != (kind == 2) {
		return row, false
	}
	row[0], row[1] = byte(kind), byte(state)
	binary.BigEndian.PutUint64(row[34:], expiry)
	return row, true
}
