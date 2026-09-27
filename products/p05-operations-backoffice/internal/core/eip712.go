// Package core is the P05 operations core ([N20]): chain calls, EIP-712
// signing, keystore and audit. opsctl uses it now; opsd will use it next cycle.
package core

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
)

// Domain is the EIP-712 domain shared by every protocol struct ([N04]).
type Domain struct {
	ChainID           uint64
	VerifyingContract common.Address
}

func (d Domain) typed() apitypes.TypedDataDomain {
	return apitypes.TypedDataDomain{
		Name:              "NU54 Payment Settlement",
		Version:           "1",
		ChainId:           math.NewHexOrDecimal256(int64(d.ChainID)),
		VerifyingContract: d.VerifyingContract.Hex(),
	}
}

var domainType = []apitypes.Type{
	{Name: "name", Type: "string"},
	{Name: "version", Type: "string"},
	{Name: "chainId", Type: "uint256"},
	{Name: "verifyingContract", Type: "address"},
}

// fieldTypes turns the generated encodeType into apitypes fields.
func fieldTypes(primary string) ([]apitypes.Type, error) {
	enc, ok := protocol.EncodeType[primary]
	if !ok {
		return nil, fmt.Errorf("unknown primary type %q", primary)
	}
	var fields []apitypes.Type
	inner := enc[len(primary)+1 : len(enc)-1]
	start := 0
	for i := 0; i <= len(inner); i++ {
		if i == len(inner) || inner[i] == ',' {
			var typ, name string
			if _, err := fmt.Sscanf(inner[start:i], "%s %s", &typ, &name); err != nil {
				return nil, fmt.Errorf("parse %q: %w", inner[start:i], err)
			}
			fields = append(fields, apitypes.Type{Name: name, Type: typ})
			start = i + 1
		}
	}
	return fields, nil
}

// Digest returns the EIP-712 digest of message for the given primary type.
// message values use decimal strings for integers and 0x-hex for addresses and bytes32.
func Digest(d Domain, primary string, message map[string]any) ([]byte, error) {
	fields, err := fieldTypes(primary)
	if err != nil {
		return nil, err
	}
	td := apitypes.TypedData{
		Types:       apitypes.Types{"EIP712Domain": domainType, primary: fields},
		PrimaryType: primary,
		Domain:      d.typed(),
		Message:     message,
	}
	hash, _, err := apitypes.TypedDataAndHash(td)
	return hash, err
}

// TimeAnchorMessage builds the operator-signed TimeAnchor ([N06]).
func TimeAnchorMessage(device common.Address, timestamp uint64) map[string]any {
	return map[string]any{"device": device.Hex(), "timestamp": new(big.Int).SetUint64(timestamp).String()}
}

// DeviceResetMessage builds the operator-signed DeviceReset ([N23]).
func DeviceResetMessage(device common.Address, nonce *big.Int) map[string]any {
	return map[string]any{"device": device.Hex(), "nonce": nonce.String()}
}
