package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
)

func TestOperatorGrantIsReopenedAndBoundToCredential(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operator.json")
	actor := wallet.Claims{Subject: "6e8a535f-ac65-482e-bfe1-0a7fe2469eff", Credential: "d81e4b58-785c-47fd-b866-7f452d7c1f56"}
	grant := grantSource(path)
	if grant(actor) {
		t.Fatal("missing grants authorized")
	}
	ref := sha256.Sum256([]byte("nu54/p03/recovery-ops/v1\x00" + actor.Subject + "\x00" + actor.Credential))
	record := recoveryOperatorGrant{Subject: actor.Subject, Credential: actor.Credential, Expires: time.Now().Add(time.Minute)}
	publish := func(value any) {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path+".next", raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".next", path); err != nil {
			t.Fatal(err)
		}
	}
	publish(map[string]recoveryOperatorGrant{hex.EncodeToString(ref[:]): record})
	if !grant(actor) {
		t.Fatal("current grant denied")
	}
	other := actor
	other.Credential = "9c8ddca5-fd81-4104-ab61-37465918e6d7"
	if grant(other) {
		t.Fatal("unassigned credential accepted")
	}
	record.Expires = time.Now().Add(-time.Second)
	publish(map[string]recoveryOperatorGrant{hex.EncodeToString(ref[:]): record})
	if grant(actor) {
		t.Fatal("expired grant accepted")
	}
	publish(map[string]any{})
	if grant(actor) {
		t.Fatal("removed grant accepted")
	}
	publish([]string{"malformed"})
	if grant(actor) {
		t.Fatal("malformed authority fell back to an older snapshot")
	}
}
