package core

import (
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var halfOrder = new(big.Int).Rsh(crypto.S256().Params().N, 1)

// SignDigest signs a 32-byte digest in the protocol form: r || s || v, low-s, v = 27 or 28.
// go-ethereum signs deterministically (RFC 6979) with low-s and a 0/1 recovery id.
func SignDigest(digest []byte, key *ecdsa.PrivateKey) ([]byte, error) {
	sig, err := crypto.Sign(digest, key)
	if err != nil {
		return nil, err
	}
	sig[64] += 27
	return sig, nil
}

// RecoverSigner returns the signer of a protocol signature, refusing other forms.
func RecoverSigner(digest, sig []byte) (common.Address, error) {
	if len(sig) != 65 {
		return common.Address{}, errors.New("signature must be 65 bytes")
	}
	if sig[64] != 27 && sig[64] != 28 {
		return common.Address{}, errors.New("v must be 27 or 28")
	}
	if new(big.Int).SetBytes(sig[32:64]).Cmp(halfOrder) > 0 {
		return common.Address{}, errors.New("high-s signature")
	}
	rsv := append([]byte{}, sig...)
	rsv[64] -= 27
	pub, err := crypto.SigToPub(digest, rsv)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*pub), nil
}
