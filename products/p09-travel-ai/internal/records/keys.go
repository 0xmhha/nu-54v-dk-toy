package records

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
)

// LoadKey reads deployer-provisioned bytes from a private secret file.
func LoadKey(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("private key file required")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("private key unavailable")
	}
	defer clear(body)
	key, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(string(body)))
	if err != nil || len(key) != 32 {
		clear(key)
		return nil, errors.New("private key invalid")
	}
	return key, nil
}
