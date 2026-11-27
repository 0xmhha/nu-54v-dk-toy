package wallet

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

const Audience = "nu54-p03-wallet"

type Claims struct {
	Issuer     string `json:"iss"`
	Audience   string `json:"aud"`
	Subject    string `json:"sub"`
	Credential string `json:"credential_ref"`
	Scope      string `json:"scope"`
	Issued     int64  `json:"iat"`
	Expires    int64  `json:"exp"`
}

type IdentityVerifier struct {
	Issuer string
	Key    ed25519.PublicKey
}

func (v IdentityVerifier) Verify(token, scope string, now time.Time) (Claims, error) {
	var claims Claims
	invalid := errors.New("invalid wallet login")
	if len(v.Key) != ed25519.PublicKeySize || v.Issuer == "" || len(token) > 8192 {
		return claims, invalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, invalid
	}
	for _, part := range parts {
		if len(part) == 0 || strings.Contains(part, "=") {
			return claims, invalid
		}
	}
	header, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return claims, invalid
	}
	var h struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if decodeStrict(header, &h) != nil || h.Algorithm != "EdDSA" || h.Type != "JWT" {
		return claims, invalid
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || !ed25519.Verify(v.Key, []byte(parts[0]+"."+parts[1]), signature) {
		return claims, invalid
	}
	body, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || decodeStrict(body, &claims) != nil || claims.Issuer != v.Issuer || claims.Audience != Audience ||
		claims.Scope != scope || !canonicalUUID(claims.Subject) || !canonicalUUID(claims.Credential) ||
		claims.Issued > now.Unix()+30 || claims.Issued < now.Add(-5*time.Minute).Unix() ||
		claims.Expires <= now.Unix() || claims.Expires > claims.Issued+300 {
		return Claims{}, invalid
	}
	return claims, nil
}

func canonicalUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

func decodeStrict(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
