// Package deployment defines the signed deployment identity shared by the producer and the desk.
package deployment

import (
	"crypto/ed25519"
	"encoding/json"

	"github.com/ethereum/go-ethereum/common"
)

type Profile struct {
	StreamID      string         `json:"streamId"`
	GenerationID  string         `json:"generationId"`
	Version       uint8          `json:"version"`
	ChainID       uint64         `json:"chainId"`
	Contract      common.Address `json:"contract"`
	Asset         common.Address `json:"asset"`
	AssetDecimals *uint8         `json:"assetDecimals,omitempty"`
	ABIHash       string         `json:"abiSha256"`
	Manifest      string         `json:"manifestSha256"`
	CodeHash      common.Hash    `json:"codeHash"`
	StartHeight   int64          `json:"startHeight"`
	StartHash     common.Hash    `json:"startHash"`
}

func Verify(profile Profile, key ed25519.PublicKey, signature []byte) bool {
	body, err := json.Marshal(profile)
	return err == nil && len(key) == ed25519.PublicKeySize &&
		ed25519.Verify(key, append([]byte("NU54-chainview-profile-v1\x00"), body...), signature)
}
