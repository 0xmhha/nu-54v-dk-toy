package market

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDeploymentArtifactRequiresCurrentCompiler(t *testing.T) {
	for _, test := range []struct {
		name, version, evm string
		viaIR, literal     bool
		want               bool
	}{
		{"verified settings", "0.8.39+commit.ce47f2c8", "prague", false, false, true},
		{"stale compiler", "0.8.30+commit.73712a01", "prague", false, false, false},
		{"wrong EVM", "0.8.39+commit.ce47f2c8", "osaka", false, false, false},
		{"different codegen", "0.8.39+commit.ce47f2c8", "prague", true, false, false},
		{"literal source metadata", "0.8.39+commit.ce47f2c8", "prague", false, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "Market.json")
			artifact := fmt.Sprintf(`{"bytecode":{"object":"0x6000"},"metadata":{"compiler":{"version":%q},"settings":{"evmVersion":%q,"viaIR":%t,"metadata":{"bytecodeHash":"ipfs","useLiteralContent":%t},"compilationTarget":{"src/Market.sol":"Market"},"remappings":["forge-std/=../p06-stablenet-contracts/lib/forge-std/src/"],"optimizer":{"enabled":true,"runs":200}}}}`,
				test.version, test.evm, test.viaIR, test.literal)
			if err := os.WriteFile(file, []byte(artifact), 0600); err != nil {
				t.Fatal(err)
			}
			bytecode, err := deploymentBytecode(file)
			if test.want {
				if err != nil || string(bytecode) != "\x60\x00" {
					t.Fatalf("expected creation bytecode, got %x: %v", bytecode, err)
				}
			} else if err == nil {
				t.Fatal("deployment accepted an artifact for a different toolchain")
			}
		})
	}
}
