package commerce

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// A booking's deposit and balance are whole won at a won branch (owner decision 2026-11-27). A dollar branch keeps
// every digit it had.
func TestTravelHoldAmountsAreWholeWon(t *testing.T) {
	const wonBranch, coinBranch, dollarBranch = "d6d10219-1c27-492c-9bdf-ec14d7d092bd", "0a4c8f36-7d0b-4c93-9c38-3c1f5b3f6a11", "5eb86d05-907a-4602-ab91-f85dd71d845c"
	leaf := &x509.Certificate{Raw: []byte("whole-won"), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Minute)}
	digest := sha256.Sum256(leaf.Raw)
	coin := Token{Name: "WKRC", Symbol: "WKRC", Label: "KRWC", Decimals: 18, BoardDecimals: 2}
	mux := http.NewServeMux()
	HTTP{Credentials: map[string]service.Principal{hex.EncodeToString(digest[:]): {Tenant: "11111111-1111-4111-8111-111111111111",
		Subject: "22222222-2222-4222-8222-222222222222", Role: "customer", ExpiresAt: time.Now().Unix() + 60}},
		Profiles: map[string]Profile{wonBranch: {Token: testWon}, coinBranch: {Token: coin}, dollarBranch: {}}}.Register(mux)
	hold := func(branch, deposit, balance string) int {
		body := `{"travel":{"operation":"hold","branch":"` + branch + `","slot":"568299aa-19ef-492e-9bd3-eca7eec67d0b",` +
			`"requestKey":"d41513c7-8036-43e5-8339-724afb603b69","partySize":1,"termsVersion":1,"asset":"0x` + strings.Repeat("2", 40) +
			`","deposit":"` + deposit + `","balance":"` + balance + `"}}`
		request := httptest.NewRequest("POST", "/v2/commerce", bytes.NewBufferString(body))
		request.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
		reply := httptest.NewRecorder()
		mux.ServeHTTP(reply, request)
		return reply.Code
	}
	// Without a store an accepted request ends at 503; a refused amount ends at 400 before it.
	const accepted, refused = http.StatusServiceUnavailable, http.StatusBadRequest
	for _, c := range []struct {
		branch, deposit, balance string
		want                     int
	}{
		{wonBranch, "3333300", "0", accepted},       // 33,333 won
		{wonBranch, "3333300", "1000000", accepted}, // and 10,000 won on the day
		{wonBranch, "3333350", "0", refused},        // 33,333.50 won
		{wonBranch, "3333300", "1000001", refused},  // a balance of 10,000.01 won
		{coinBranch, "33333" + strings.Repeat("0", 18), "0", accepted},
		{coinBranch, "33333" + strings.Repeat("0", 17) + "1", "0", refused},
		{dollarBranch, "3333350", "1000001", accepted}, // dollars are unchanged
	} {
		if got := hold(c.branch, c.deposit, c.balance); got != c.want {
			t.Errorf("hold at %s of %s and %s: %d, want %d", c.branch, c.deposit, c.balance, got, c.want)
		}
	}
}

// What the ledger is told about a token: ten to the power of its decimals for won, nothing for dollars.
func TestWholeUnitOfATokenIsOneWon(t *testing.T) {
	coin := Token{Name: "WKRC", Symbol: "WKRC", Label: "KRWC", Decimals: 18, BoardDecimals: 2}
	for _, c := range []struct {
		token Token
		unit  string
	}{{testWon, "100"}, {coin, "1" + strings.Repeat("0", 18)}, {DollarToken, ""}, {Token{}, ""}} {
		if got := c.token.WholeUnit(); got != c.unit {
			t.Errorf("%s: unit %q, want %q", c.token.Symbol, got, c.unit)
		}
	}
	if !testWon.WholeAmount("3333300") || testWon.WholeAmount("3333350") || !DollarToken.WholeAmount("3333350") || !(Token{}).WholeAmount("1") {
		t.Error("whole amounts follow the price rule")
	}
}
