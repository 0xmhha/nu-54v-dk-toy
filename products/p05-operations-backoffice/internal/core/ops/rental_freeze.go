package ops

import (
	"crypto/ecdsa"
	"errors"
	"strconv"

	"github.com/ethereum/go-ethereum/common"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/ble"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/core"
)

// RentalFreezeRequestV2 binds the server's challenge to the board nonce from the same link.
type RentalFreezeRequestV2 struct {
	Binding      common.Hash
	Challenge    common.Hash
	ChannelNonce common.Hash
	ExpiresAt    uint64
}

func (request RentalFreezeRequestV2) Digest(domain core.Domain) ([]byte, error) {
	return core.Digest(domain, "RentalFreezeRequestV2", map[string]any{
		"binding": request.Binding.Hex(), "challenge": request.Challenge.Hex(),
		"channelNonce": request.ChannelNonce.Hex(), "expiresAt": strconv.FormatUint(request.ExpiresAt, 10),
	})
}

// IssueRentalFreezeV2 signs for a collector after the commissioning authority confirms the
// current generation, challenge and device scope. The resulting proof is connection-specific.
func IssueRentalFreezeV2(domain core.Domain, operator *ecdsa.PrivateKey, request RentalFreezeRequestV2,
	link ble.ProtectedRentalChallenge, now uint64, authorize func(core.Domain, RentalFreezeRequestV2) error) ([]byte, error) {
	if operator == nil || authorize == nil || domain.ChainID == 0 || domain.VerifyingContract == (common.Address{}) ||
		request.Binding == (common.Hash{}) || request.Challenge == (common.Hash{}) ||
		request.ChannelNonce == (common.Hash{}) || request.ChannelNonce != common.Hash(link.Nonce) ||
		now == 0 || request.ExpiresAt <= now || request.ExpiresAt > link.ExpiresAt || request.ExpiresAt-now > 120 {
		return nil, errors.New("protected rental freeze request does not match an active collector challenge")
	}
	if err := authorize(domain, request); err != nil {
		return nil, err
	}
	digest, err := request.Digest(domain)
	if err != nil {
		return nil, err
	}
	return core.SignDigest(digest, operator)
}
