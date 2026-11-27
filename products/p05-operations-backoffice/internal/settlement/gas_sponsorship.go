package settlement

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
)

// GasSponsorGrant is the deployment authority's explicit branch-specific gas commitment.
// The signing domain fixes the sponsor's responsibility to the merchant relay.
type GasSponsorGrant struct {
	Branch    string         `json:"branch"`
	Chain     uint64         `json:"chain"`
	Contract  common.Address `json:"contract"`
	Asset     common.Address `json:"asset"`
	Manifest  string         `json:"manifest"`
	Merchant  common.Hash    `json:"merchant"`
	Relay     common.Address `json:"relay"`
	StartsAt  int64          `json:"startsAt"`
	ExpiresAt int64          `json:"expiresAt"`
	Signature string         `json:"signature"`
}

func GasSponsorGrantMessage(grant GasSponsorGrant) ([]byte, bool) {
	branch, branchErr := hex.DecodeString(strings.ReplaceAll(grant.Branch, "-", ""))
	manifest, manifestErr := hex.DecodeString(grant.Manifest)
	if branchErr != nil || len(branch) != 16 || !financeRef.MatchString(grant.Branch) ||
		manifestErr != nil || len(manifest) != 32 || strings.ToLower(grant.Manifest) != grant.Manifest ||
		grant.Chain != 8283 || grant.Contract == (common.Address{}) || grant.Asset == (common.Address{}) ||
		grant.Merchant == (common.Hash{}) || grant.Relay == (common.Address{}) ||
		grant.StartsAt <= 0 || grant.ExpiresAt <= grant.StartsAt {
		return nil, false
	}
	body := append([]byte("NU54-merchant-gas-commitment-v1\x00"), branch...)
	body = binary.BigEndian.AppendUint64(body, grant.Chain)
	body = append(body, grant.Contract[:]...)
	body = append(body, grant.Asset[:]...)
	body = append(body, manifest...)
	body = append(body, grant.Merchant[:]...)
	body = append(body, grant.Relay[:]...)
	body = binary.BigEndian.AppendUint64(body, uint64(grant.StartsAt))
	return binary.BigEndian.AppendUint64(body, uint64(grant.ExpiresAt)), true
}

func VerifyMerchantGasSponsor(grant GasSponsorGrant, branch string, profile commerce.Profile, target Target,
	deployment DeploymentReview, payer common.Address, authority ed25519.PublicKey, now time.Time,
) (commerce.GasSponsorship, bool) {
	var absent commerce.GasSponsorship
	if grant.Branch != branch || grant.Chain != profile.Chain ||
		grant.Contract != profile.Contract || grant.Contract != target.Contract ||
		grant.Asset != profile.Asset || grant.Asset != target.Asset ||
		grant.Merchant != profile.MerchantID || grant.Manifest != target.Manifest ||
		grant.Relay == payer || now.Unix() < grant.StartsAt || now.Unix() >= grant.ExpiresAt ||
		deployment.Status != "verified" || deployment.Chain != grant.Chain ||
		deployment.Manifest != grant.Manifest || deployment.Contract != lower(grant.Contract) ||
		deployment.Asset != lower(grant.Asset) || deployment.GasPayer != lower(grant.Relay) ||
		len(authority) != ed25519.PublicKeySize {
		return absent, false
	}
	body, valid := GasSponsorGrantMessage(grant)
	signature, err := hex.DecodeString(grant.Signature)
	if !valid || err != nil || !ed25519.Verify(authority, body, signature) {
		return absent, false
	}
	return commerce.GasSponsorship{Account: grant.Relay, Grant: sha256.Sum256(body), ExpiresAt: grant.ExpiresAt}, true
}
