package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestEncodeTypeMatchesVectors checks that the generated encodeType strings are
// the ones the committed EIP-712 test vectors were built from.
func TestEncodeTypeMatchesVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "content", "specifications", "protocol", "eip712-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Vectors []struct {
			ID          string `json:"id"`
			PrimaryType string `json:"primaryType"`
			EncodeType  string `json:"encodeType"`
		} `json:"vectors"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, v := range doc.Vectors {
		if got := EncodeType[v.PrimaryType]; got != v.EncodeType {
			t.Errorf("%s: encodeType %q, vector has %q", v.ID, got, v.EncodeType)
		}
	}
}
