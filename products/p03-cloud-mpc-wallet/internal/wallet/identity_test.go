package wallet

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func identityToken(t *testing.T, key ed25519.PrivateKey, claims Claims) string {
	t.Helper()
	encoding := base64.RawURLEncoding
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signed := encoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"JWT"}`)) + "." + encoding.EncodeToString(payload)
	return signed + "." + encoding.EncodeToString(ed25519.Sign(key, []byte(signed)))
}

func TestP03IdentityScope(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now()
	verifier := IdentityVerifier{Issuer: "https://wallet-id.example", Key: pub}
	base := Claims{Issuer: verifier.Issuer, Audience: Audience, Subject: uuid.NewString(), Credential: uuid.NewString(),
		Scope: "wallet", Issued: now.Unix(), Expires: now.Add(time.Minute).Unix()}
	token := identityToken(t, key, base)
	if got, err := verifier.Verify(token, "wallet", now); err != nil || got.Subject != base.Subject {
		t.Fatalf("valid P03 wallet login rejected: %v", err)
	}
	if _, err := verifier.Verify(token, "wallet:enroll", now); err == nil {
		t.Fatal("ordinary bearer enrolled credential")
	}
	for name, edit := range map[string]func(*Claims){
		"P05 audience":         func(c *Claims) { c.Audience = "nu54-p05-commerce" },
		"wrong issuer":         func(c *Claims) { c.Issuer = "https://commerce-id.example" },
		"expired":              func(c *Claims) { c.Expires = now.Unix() },
		"unbounded expiration": func(c *Claims) { c.Expires = now.Add(time.Hour).Unix() },
		"provider identifier":  func(c *Claims) { c.Subject = "raw-social-subject" },
	} {
		t.Run(name, func(t *testing.T) {
			c := base
			edit(&c)
			if _, err := verifier.Verify(identityToken(t, key, c), "wallet", now); err == nil {
				t.Fatal("accepted out-of-scope token")
			}
		})
	}
	api := API{Identity: verifier, ProfileSubject: uuid.NewString()}.Handler()
	request := httptest.NewRequest(http.MethodGet, "/v1/wallet", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	got := httptest.NewRecorder()
	api.ServeHTTP(got, request)
	if got.Code != http.StatusForbidden {
		t.Fatalf("unbound pilot subject reached wallet API: %d", got.Code)
	}
}
