package settlement

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

func TestFinanceCredentialExpiresDuringBody(t *testing.T) {
	credential := &x509.Certificate{Raw: []byte("finance fixture"), NotAfter: time.Now().Add(time.Hour)}
	fingerprint := sha256.Sum256(credential.Raw)
	expiry := time.Now().Unix() + 1
	h := FinanceHTTP{Credentials: map[string]service.Principal{
		hex.EncodeToString(fingerprint[:]): {Role: "finance", ExpiresAt: expiry},
	}}
	reader, writer := io.Pipe()
	defer reader.Close()
	req := httptest.NewRequest("POST", "/v2/finance/day-close", reader)
	req.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{credential}, VerifiedChains: [][]*x509.Certificate{{credential}}}
	req.SetPathValue("operation", "day-close")
	go func() {
		defer writer.Close()
		time.Sleep(time.Until(time.Unix(expiry, 0)) + time.Millisecond)
		_, _ = io.Copy(writer, strings.NewReader(`{"branchId":"838403f1-2888-41f2-b7bb-766a14110001","businessDay":"2026-11-20","asset":"0x1111111111111111111111111111111111111111","gross":"0","refunds":"0","evidenceRef":"838403f1-2888-41f2-b7bb-766a14110002"}`))
	}()
	recorder := httptest.NewRecorder()
	h.serve(recorder, req)
	if recorder.Code != 404 {
		t.Fatalf("expired finance credential returned status %d", recorder.Code)
	}
}
