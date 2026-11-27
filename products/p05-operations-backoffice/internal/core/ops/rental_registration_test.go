package ops

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestProtectedRentalRegistrationAuthenticatesInstalledBoardAndCurrentCollector(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public := elliptic.Marshal(key.Curve, key.X, key.Y)
	scope := RentalRegistrationScope{
		Challenge: [32]byte{1}, Board: [32]byte{2}, FreezeSigner: common.Address{3},
		CollectorCert: [32]byte{4}, CollectorPeer: [7]byte{1, 5}, FirstNonce: [32]byte{6},
		ExpiresAt: 130,
	}
	binary.BigEndian.PutUint64(scope.Binding[:8], 9)
	for _, offset := range []int{8, 24, 40, 72, 104, 136, 168, 188} {
		scope.Binding[offset] = byte(offset)
	}
	wire := make([]byte, 439)
	copy(wire, "N54G")
	copy(wire[4:], scope.Challenge[:])
	copy(wire[36:], scope.Board[:])
	copy(wire[68:], scope.Binding[:])
	copy(wire[276:], scope.FreezeSigner[:])
	copy(wire[296:], scope.CollectorCert[:])
	copy(wire[328:], scope.CollectorPeer[:])
	copy(wire[335:], scope.FirstNonce[:])
	binary.BigEndian.PutUint64(wire[367:], scope.ExpiresAt)
	digest := sha256.Sum256(wire[:375])
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	r.FillBytes(wire[375:407])
	s.FillBytes(wire[407:439])
	if err := VerifyRentalRegistration(wire, public, scope, 129); err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*RentalRegistrationScope){
		"challenge":             func(s *RentalRegistrationScope) { s.Challenge[0] ^= 1 },
		"board":                 func(s *RentalRegistrationScope) { s.Board[0] ^= 1 },
		"generation":            func(s *RentalRegistrationScope) { s.Binding[7]++ },
		"measurement":           func(s *RentalRegistrationScope) { s.Binding[72] ^= 1 },
		"freeze signer":         func(s *RentalRegistrationScope) { s.FreezeSigner[0] ^= 1 },
		"collector certificate": func(s *RentalRegistrationScope) { s.CollectorCert[0] ^= 1 },
		"collector peer":        func(s *RentalRegistrationScope) { s.CollectorPeer[1] ^= 1 },
		"first nonce":           func(s *RentalRegistrationScope) { s.FirstNonce[0] ^= 1 },
	} {
		t.Run(name, func(t *testing.T) {
			expected := scope
			mutate(&expected)
			if VerifyRentalRegistration(wire, public, expected, 129) == nil {
				t.Fatal("accepted a different enrolled scope")
			}
		})
	}
	if VerifyRentalRegistration(wire, public, scope, 130) == nil {
		t.Fatal("accepted expired registration")
	}
	wire[len(wire)-1] ^= 1
	if VerifyRentalRegistration(wire, public, scope, 129) == nil {
		t.Fatal("accepted changed installation signature")
	}
}
