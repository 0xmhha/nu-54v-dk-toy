package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// TestDigestMatchesVectors recomputes every shared vector's digest ([N21]).
func TestDigestMatchesVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "content", "specifications", "protocol", "eip712-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Domain struct {
			ChainID           uint64 `json:"chainId"`
			VerifyingContract string `json:"verifyingContract"`
		} `json:"domain"`
		Vectors []struct {
			ID          string         `json:"id"`
			PrimaryType string         `json:"primaryType"`
			Message     map[string]any `json:"message"`
			Digest      string         `json:"digest"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	d := Domain{ChainID: doc.Domain.ChainID, VerifyingContract: common.HexToAddress(doc.Domain.VerifyingContract)}
	for _, v := range doc.Vectors {
		got, err := Digest(d, v.PrimaryType, v.Message)
		if err != nil {
			t.Fatalf("%s: %v", v.ID, err)
		}
		if hexutil.Encode(got) != v.Digest {
			t.Errorf("%s: digest %s, vector %s", v.ID, hexutil.Encode(got), v.Digest)
		}
	}
}
