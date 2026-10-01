package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestLoadKeyFromFileSecret(t *testing.T) {
	dir := t.TempDir()
	priv, _ := crypto.HexToECDSA("59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d")
	acct, err := keystore.NewKeyStore(dir, keystore.LightScryptN, keystore.LightScryptP).ImportECDSA(priv, "pw")
	if err != nil {
		t.Fatal(err)
	}
	ks, pw := acct.URL.Path, filepath.Join(t.TempDir(), "pw")
	_ = os.WriteFile(pw, []byte("pw\n"), 0o600)
	got, err := LoadKey(ks, "file:"+pw)
	if err != nil || Address(got) != acct.Address {
		t.Fatalf("LoadKey: %v", err)
	}
	if _, err := LoadKey(ks, pw); err == nil {
		t.Fatal("a bare path is not a secretRef")
	}
	_ = os.WriteFile(pw, []byte("wrong"), 0o600)
	if _, err := LoadKey(ks, "file:"+pw); err == nil {
		t.Fatal("wrong password accepted")
	}
}

func TestSignAndRecover(t *testing.T) {
	priv, _ := crypto.HexToECDSA("5de4111afa1a4b94908f83103eb1f1706367c2e68ca870fc3fb9a804cdab365a")
	digest := crypto.Keccak256([]byte("nu54"))
	sig, err := SignDigest(digest, priv)
	if err != nil || (sig[64] != 27 && sig[64] != 28) {
		t.Fatalf("sign: %v v=%d", err, sig[64])
	}
	if a, err := RecoverSigner(digest, sig); err != nil || a != Address(priv) {
		t.Fatalf("recover: %s %v", a.Hex(), err)
	}
	bad := append([]byte{}, sig...)
	bad[64] = 1
	if _, err := RecoverSigner(digest, bad); err == nil {
		t.Fatal("v = 1 accepted")
	}
	if _, err := RecoverSigner(digest, sig[:64]); err == nil {
		t.Fatal("64-byte signature accepted")
	}
}

func TestDistinctRoles(t *testing.T) {
	a, b := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	if err := DistinctRoles(map[string]common.Address{"operator": a, "registry-admin": b}); err != nil {
		t.Fatal(err)
	}
	if err := DistinctRoles(map[string]common.Address{"operator": a, "registry-admin": a}); err == nil {
		t.Fatal("shared address accepted")
	}
}

func TestDeploymentAndRegister(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	d, err := LoadDeployment(filepath.Join(root, "products", "p06-stablenet-contracts", "deployments", "8283.json"))
	if err != nil || d.ChainID != 8283 || d.Settlement == (common.Address{}) {
		t.Fatalf("deployment: %+v %v", d, err)
	}
	v, err := RegisterParameter(root, "attestationValidity")
	if err != nil || v != 86400 {
		t.Fatalf("attestationValidity: %d %v", v, err)
	}
}
