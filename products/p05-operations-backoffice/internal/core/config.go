package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
)

// Deployment is the part of products/p06-stablenet-contracts/deployments/<chainId>.json
// the operations tool needs. Addresses are never typed by hand.
type Deployment struct {
	ChainID    uint64
	Registry   common.Address
	Settlement common.Address
	Token      common.Address
}

func LoadDeployment(path string) (Deployment, error) {
	var raw struct {
		ChainID   uint64 `json:"chainId"`
		Contracts map[string]struct {
			Address string `json:"address"`
		} `json:"contracts"`
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Deployment{}, err
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return Deployment{}, err
	}
	d := Deployment{
		ChainID:    raw.ChainID,
		Registry:   common.HexToAddress(raw.Contracts["MerchantRegistry"].Address),
		Settlement: common.HexToAddress(raw.Contracts["PaymentSettlement"].Address),
		Token:      common.HexToAddress(raw.Contracts["TestUSDC"].Address),
	}
	if d.ChainID == 0 || d.Registry == (common.Address{}) || d.Settlement == (common.Address{}) {
		return Deployment{}, fmt.Errorf("%s: chainId or contract addresses missing", path)
	}
	return d, nil
}

// Domain is the EIP-712 domain of the deployment.
func (d Deployment) Domain() Domain {
	return Domain{ChainID: d.ChainID, VerifyingContract: d.Settlement}
}

// RegisterParameter reads a value from the design register
// (docs/content/planning/design-freeze-checkpoint-02.json, `.parameters.<name>.value`).
func RegisterParameter(repoRoot, name string) (uint64, error) {
	var raw struct {
		Parameters map[string]struct {
			Value json.Number `json:"value"`
		} `json:"parameters"`
	}
	b, err := os.ReadFile(filepath.Join(repoRoot, "docs/content/planning/design-freeze-checkpoint-02.json"))
	if err != nil {
		return 0, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(&raw); err != nil {
		return 0, err
	}
	p, ok := raw.Parameters[name]
	if !ok {
		return 0, fmt.Errorf("register has no parameter %s", name)
	}
	v, err := p.Value.Int64()
	if err != nil || v < 0 {
		return 0, fmt.Errorf("parameter %s is not a non-negative integer", name)
	}
	return uint64(v), nil
}
