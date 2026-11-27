package quotev2

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
)

type CommitmentPurpose byte

const (
	TenantCommitment CommitmentPurpose = 1
	BranchCommitment CommitmentPurpose = 2
	ClaimCommitment  CommitmentPurpose = 3
	IntentCommitment CommitmentPurpose = 4
)

// CommitPrivateContext binds issuer-only context without putting private identifiers or
// capabilities into calldata. The key must be dedicated to the quote commitment purpose.
func CommitPrivateContext(key, quoteID [32]byte, purpose CommitmentPurpose, value []byte) ([32]byte, error) {
	var validLength bool
	switch purpose {
	case TenantCommitment, BranchCommitment:
		validLength = len(value) == 16
	case ClaimCommitment:
		validLength = len(value) == 32
	case IntentCommitment:
		validLength = len(value) > 0 && len(value) <= 65535
	}
	if !validLength || zero(value) || zero(key[:]) || zero(quoteID[:]) {
		return [32]byte{}, errors.New("invalid private quote context")
	}
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte("NU54-QUOTE-COMMIT-V2\x00"))
	mac.Write([]byte{byte(purpose)})
	mac.Write(quoteID[:])
	mac.Write(binary.BigEndian.AppendUint32(nil, uint32(len(value))))
	mac.Write(value)
	return [32]byte(mac.Sum(nil)), nil
}
