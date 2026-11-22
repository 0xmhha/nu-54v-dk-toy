package ops

import (
	"bytes"
	"encoding/binary"
	"math/big"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type VerifiedRentalJournal struct {
	Receipt                                     rentalv1.FreezeReceipt
	Expected                                    rentalv1.Binding
	PaymentEntries, LimitEntries, BurnedEntries int
}

func VerifyRentalJournal(body rentalv1.Request, scope RentalCollectionScope, challenge [32]byte,
	challengeUntil, now time.Time) (VerifiedRentalJournal, error) {
	var out VerifiedRentalJournal
	receipt, err := rentalv1.ParseFreezeReceipt(body.Receipt)
	projection := body.Projection
	chain := binary.BigEndian.Uint64(scope.Binding[160:168])
	contract := common.Address(scope.Binding[168:188])
	account := common.Address(scope.Binding[188:208])
	if err != nil || chain != 8283 || contract == (common.Address{}) ||
		scope.BoardSigner == (common.Address{}) || scope.BoardSigner == scope.Operator || scope.BoardSigner == account ||
		receipt.Binding != crypto.Keccak256Hash(scope.Binding[:]) ||
		challenge == [32]byte{} || receipt.Challenge != challenge || receipt.Root == [32]byte{} ||
		receipt.EntryCount > 8 || receipt.FirstNonce != scope.FirstNonce || receipt.FirstNonce[31] != 0 ||
		now.Unix() <= 0 || receipt.FrozenAt == 0 || receipt.FrozenAt > uint64(now.Unix()) ||
		receipt.ExpiresAt <= uint64(now.Unix()) || receipt.ExpiresAt-uint64(now.Unix()) > 120 ||
		challengeUntil.Unix() <= 0 || receipt.ExpiresAt > uint64(challengeUntil.Unix()) ||
		len(projection) != 277+126*int(receipt.EntryCount) || string(projection[:4]) != "RJP1" ||
		!bytes.Equal(projection[4:212], scope.Binding[:]) || !bytes.Equal(projection[212:244], receipt.FirstNonce[:]) ||
		!bytes.Equal(projection[244:276], receipt.NextNonce[:]) || uint64(projection[276]) != receipt.EntryCount ||
		crypto.Keccak256Hash(projection) != receipt.Root {
		return out, ErrRentalCollection
	}
	digest := common.Hash(receipt.Digest(chain, contract))
	recovered, err := core.RecoverSigner(digest[:], receipt.BoardSignature[:])
	if err != nil || recovered != scope.BoardSigner {
		return out, ErrRentalCollection
	}
	nonce := new(big.Int).SetBytes(receipt.FirstNonce[:])
	for row := projection[277:]; len(row) != 0; row = row[126:] {
		kind, state := row[0], row[1]
		merchant, order := common.Address(row[42:62]), common.Hash(row[62:94])
		if (kind != 1 && kind != 2) || (state != 1 && state != 2) ||
			(merchant == (common.Address{})) != (kind == 2) || (order == (common.Hash{})) != (kind == 2) ||
			binary.BigEndian.Uint64(row[34:42]) == 0 || common.Hash(row[94:126]) == (common.Hash{}) ||
			new(big.Int).SetBytes(row[2:34]).Cmp(nonce) != 0 {
			return VerifiedRentalJournal{}, ErrRentalCollection
		}
		switch {
		case state == 2:
			out.BurnedEntries++
		case kind == 1:
			out.PaymentEntries++
		case kind == 2:
			out.LimitEntries++
		}
		nonce.Add(nonce, big.NewInt(1))
	}
	if new(big.Int).SetBytes(receipt.NextNonce[:]).Cmp(nonce) != 0 {
		return VerifiedRentalJournal{}, ErrRentalCollection
	}
	out.Receipt = receipt
	out.Expected = rentalv1.Binding{Stream: scope.Stream, StreamGeneration: scope.StreamGeneration,
		Deployment: common.Hash(scope.Binding[104:136]).Hex(), FreezeReceipt: digest.Hex(), JournalRoot: common.Hash(receipt.Root).Hex()}
	return out, nil
}
