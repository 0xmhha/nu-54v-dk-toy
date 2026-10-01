package core

import (
	"crypto/ecdsa"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// LoadKey opens an encrypted keystore whose password comes from a secretRef ([N32]):
//
//	keychain:<service>  macOS Keychain item (account "nu54"), read into memory only
//	file:<path>         a password file readable only by the user
//
// The password and the key never touch the repository or the command line.
func LoadKey(keystorePath, passwordRef string) (*ecdsa.PrivateKey, error) {
	blob, err := os.ReadFile(keystorePath)
	if err != nil {
		return nil, err
	}
	password, err := resolveSecret(passwordRef)
	if err != nil {
		return nil, err
	}
	k, err := keystore.DecryptKey(blob, password)
	if err != nil {
		return nil, fmt.Errorf("decrypt %s: %w", keystorePath, err)
	}
	return k.PrivateKey, nil
}

func resolveSecret(ref string) (string, error) {
	switch {
	case strings.HasPrefix(ref, "keychain:"):
		out, err := exec.Command("security", "find-generic-password", "-a", "nu54", "-s", strings.TrimPrefix(ref, "keychain:"), "-w").Output()
		if err != nil {
			return "", fmt.Errorf("keychain item %s: %w", ref, err)
		}
		return strings.TrimRight(string(out), "\n"), nil
	case strings.HasPrefix(ref, "file:"):
		b, err := os.ReadFile(strings.TrimPrefix(ref, "file:"))
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\n"), nil
	}
	return "", fmt.Errorf("secretRef %q: use keychain:<service> or file:<path>", ref)
}

// Address returns the account address of a private key.
func Address(k *ecdsa.PrivateKey) common.Address {
	return crypto.PubkeyToAddress(k.PublicKey)
}

// DistinctRoles fails when two role keys share an address (P05 design 1).
func DistinctRoles(roles map[string]common.Address) error {
	seen := map[common.Address]string{}
	for name, a := range roles {
		if other, ok := seen[a]; ok {
			return fmt.Errorf("roles %s and %s share address %s", other, name, a.Hex())
		}
		seen[a] = name
	}
	return nil
}
