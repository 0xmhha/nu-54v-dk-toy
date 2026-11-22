package ops

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// RentalRegistrationScope is resolved from the current protected enrollment.
// Binding contains the canonical board generation/branch/device/registration/
// measurement/deployment/chain/contract/account bytes in that order.
type RentalRegistrationScope struct {
	Challenge     [32]byte
	Board         [32]byte
	Binding       [208]byte
	FreezeSigner  common.Address
	CollectorCert [32]byte
	CollectorPeer [7]byte
	FirstNonce    [32]byte
	ExpiresAt     uint64
}

const rentalRegistrationBodySize = 375

const RentalRegistrationWireSize = rentalRegistrationBodySize + 64

// Body is presented to the separate installation authority, whose signature
// authorizes the board to return its own signature over exactly these bytes.
func (scope RentalRegistrationScope) Body() []byte {
	bound := make([]byte, 0, rentalRegistrationBodySize)
	bound = append(bound, "N54G"...)
	bound = append(bound, scope.Challenge[:]...)
	bound = append(bound, scope.Board[:]...)
	bound = append(bound, scope.Binding[:]...)
	bound = append(bound, scope.FreezeSigner[:]...)
	bound = append(bound, scope.CollectorCert[:]...)
	bound = append(bound, scope.CollectorPeer[:]...)
	bound = append(bound, scope.FirstNonce[:]...)
	bound = binary.BigEndian.AppendUint64(bound, scope.ExpiresAt)
	return bound
}

// VerifyRentalRegistration requires the enrolled installation key to attest
// the current scope, including the separate key that will sign RFR1 receipts.
func VerifyRentalRegistration(wire, enrolledInstallationKey []byte, scope RentalRegistrationScope, now uint64) error {
	if len(wire) != RentalRegistrationWireSize || now == 0 || scope.ExpiresAt <= now || scope.ExpiresAt-now > 120 ||
		scope.Challenge == [32]byte{} || scope.Board == [32]byte{} || scope.CollectorCert == [32]byte{} ||
		[6]byte(scope.CollectorPeer[1:]) == [6]byte{} || scope.CollectorPeer[0] > 1 ||
		scope.FreezeSigner == (common.Address{}) || bytes.Equal(scope.FreezeSigner[:], scope.Binding[188:208]) ||
		binary.BigEndian.Uint64(scope.Binding[0:8]) == 0 || scope.FirstNonce[31] != 0 {
		return errors.New("invalid protected rental registration scope")
	}
	bound := scope.Body()
	if !bytes.Equal(bound, wire[:rentalRegistrationBodySize]) {
		return errors.New("protected rental registration differs from enrolled scope")
	}

	x, y := elliptic.Unmarshal(elliptic.P256(), enrolledInstallationKey)
	if x == nil {
		return errors.New("protected rental installation key not enrolled")
	}
	digest := sha256.Sum256(bound)
	signature := wire[rentalRegistrationBodySize:]
	if !ecdsa.Verify(&ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, digest[:],
		new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])) {
		return errors.New("protected rental registration signature differs")
	}
	return nil
}
