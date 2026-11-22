package chainview

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func checkExplorerBrowser(t *testing.T, ctx context.Context, database, phase, stream, legacy, merchant string) {
	t.Helper()
	command := os.Getenv("P07_EXPLORER_BROWSER_SCRIPT")
	if command == "" {
		return
	}
	open := func(role string) *pgxpool.Pool {
		config, err := pgxpool.ParseConfig(database)
		if err != nil {
			t.Fatal(err)
		}
		config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			_, err := conn.Exec(ctx, "SET ROLE "+pgx.Identifier{role}.Sanitize())
			return err
		}
		pool, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			t.Fatal(err)
		}
		return pool
	}
	public := &PublicStore{db: open("nu54_p07_public"), LegacyLedgerURL: "https://operator.invalid/?view=v1"}
	defer public.Close()
	operator := &OperatorStore{db: open("nu54_p07_explorer")}
	defer operator.Close()

	root := newExplorerClient(t)
	publicServer := httptest.NewServer(public.PublicHandler())
	defer publicServer.Close()
	streams := []string{stream, legacy}
	if phase == "current" {
		streams = append(streams, heldStream)
	}
	ops := httptest.NewUnstartedServer(operator.Handler(map[string]OperatorGrant{root.fingerprint: {
		Streams: streams, Merchants: map[string][]string{stream: {merchant}},
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}}))
	ops.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: root.ca}
	ops.StartTLS()
	defer ops.Close()

	var transaction, recipient, legacyMerchant, legacyOrder string
	var logIndex int
	if phase == "current" {
		if err := public.db.QueryRow(ctx, `SELECT r.tx_hash,r.log_index FROM chainview.explorer_receipt_detail r
			WHERE r.stream_id=$1 AND r.merchant_key=$2 AND r.order_key=$3 AND r.kind='paid'`, stream, merchant, (common.Hash{31: 4}).Hex()).
			Scan(&transaction, &logIndex); err != nil {
			t.Fatal(err)
		}
		if err := public.db.QueryRow(ctx, `SELECT merchant_key,order_key FROM chainview.explorer_receipt_detail
			WHERE stream_id=$1 AND kind='paid' LIMIT 1`, legacy).Scan(&legacyMerchant, &legacyOrder); err != nil {
			t.Fatal(err)
		}

		// The privileged test oracle compares a browser-entered expected value
		// against the public digest without passing the value to the server.
		owner, err := pgxpool.New(ctx, database)
		if err != nil {
			t.Fatal(err)
		}
		defer owner.Close()
		if err := owner.QueryRow(ctx, `SELECT recipient_ref FROM chainview.settlement_receipt
			WHERE stream_id=$1 AND tx_hash=$2 AND log_index=$3`, stream, transaction, logIndex).Scan(&recipient); err != nil {
			t.Fatal(err)
		}
	}

	browser := exec.CommandContext(ctx, "node", command)
	browser.Env = append(os.Environ(), "EXPLORER_PHASE="+phase, "EXPLORER_PUBLIC="+publicServer.URL,
		"EXPLORER_OPS="+ops.URL, "EXPLORER_CERT="+root.cert, "EXPLORER_KEY="+root.key, "EXPLORER_STREAM="+stream,
		"EXPLORER_LEGACY="+legacy, "EXPLORER_MERCHANT="+merchant,
		"EXPLORER_LEGACY_MERCHANT="+legacyMerchant, "EXPLORER_LEGACY_ORDER="+legacyOrder,
		"EXPLORER_ORDER="+(common.Hash{31: 4}).Hex(), "EXPLORER_RECIPIENT="+recipient, "EXPLORER_TX="+transaction,
		fmt.Sprintf("EXPLORER_LOG=%d", logIndex), "EXPLORER_ABSENCE="+(common.Hash{31: 99}).Hex())
	out, err := browser.CombinedOutput()
	if err != nil {
		t.Fatalf("browser verification (%s): %v\n%s", phase, err, out)
	}
	t.Logf("browser verification (%s): %s", phase, out)
}

type explorerClient struct {
	ca                            *x509.CertPool
	fingerprint, cert, key        string
	caFile, serverCert, serverKey string
}

func newExplorerClient(t *testing.T) explorerClient {
	t.Helper()
	rootPub, rootKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	parent := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageCertSign}
	authority, err := x509.CreateCertificate(rand.Reader, parent, parent, rootPub, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(authority)
	if err != nil {
		t.Fatal(err)
	}
	leafPub, leafKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2),
		NotBefore: parent.NotBefore, NotAfter: parent.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	certificate, err := x509.CreateCertificate(rand.Reader, leaf, ca, leafPub, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name, kind string, data []byte) string {
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: data}), 0600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	serverPub, serverKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	server := &x509.Certificate{SerialNumber: big.NewInt(3), NotBefore: parent.NotBefore, NotAfter: parent.NotAfter,
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	serverDER, err := x509.CreateCertificate(rand.Reader, server, ca, serverPub, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	serverPKCS8, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	sum := sha256.Sum256(certificate)
	return explorerClient{ca: roots, fingerprint: hex.EncodeToString(sum[:]),
		cert: write("test.crt", "CERTIFICATE", certificate), key: write("test.key", "PRIVATE KEY", pkcs8),
		caFile: write("ca.crt", "CERTIFICATE", authority), serverCert: write("server.crt", "CERTIFICATE", serverDER),
		serverKey: write("server.key", "PRIVATE KEY", serverPKCS8)}
}
