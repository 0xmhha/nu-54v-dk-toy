package ceremony_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"golang.org/x/sys/unix"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/google/uuid"
)

func TestParticipantHelper(t *testing.T) {
	path := os.Getenv("NU54_P03_TEST_PARTICIPANT")
	if path == "" {
		t.Skip("participant subprocess only")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg ceremony.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if err := ceremony.Serve(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
}

func TestRealCeremonies(t *testing.T) {
	if os.Getenv("NU54_P03_REAL_DKG") != "1" {
		t.Skip("set NU54_P03_REAL_DKG=1 to generate fresh safe primes in separate processes")
	}
	multi := os.Getenv("NU54_P03_MULTI") == "1"
	device := os.Getenv("NU54_P03_ANDROID_SERIAL")
	ios := os.Getenv("NU54_P03_IOS_SIMULATOR")
	if ios != "" {
		if device != "" || !regexp.MustCompile(`^[A-Fa-f0-9-]{36}$`).MatchString(ios) {
			t.Fatal("choose one dedicated P03 fixture device")
		}
		device = ios
	} else if device != "" && !regexp.MustCompile(`^emulator-[0-9]+$`).MatchString(device) {
		t.Fatal("dedicated emulator serial required")
	}
	if device != "" {
		path := filepath.Join("/tmp", fmt.Sprintf("nu54-p03-%d-%s.lock", os.Getuid(), device))
		fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
		if err != nil {
			t.Fatal(err)
		}
		lock := os.NewFile(uintptr(fd), path)
		if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
			lock.Close()
			t.Fatal("another P03 fixture owns this simulator")
		}
		t.Cleanup(func() { lock.Close() })
	}

	dir := t.TempDir()
	subject := uuid.NewString()
	mobile := readMobileTest(t, dir, subject)
	now := time.Now()
	rootPublic, rootKey, _ := ed25519.GenerateKey(rand.Reader)
	root := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, rootPublic, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string, raw []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
	caFile := write("root.pem", caPEM)
	ca := x509.NewCertPool()
	ca.AppendCertsFromPEM(caPEM)
	type identity struct{ cert, key, pin, spki string }
	certs := make(map[string]identity)
	ids := []string{"coordinator", "a1", "b1", "c1", "a2", "b2", "c2", "a3", "b3", "c3"}
	for i, id := range ids {
		var pub ed25519.PublicKey
		var priv ed25519.PrivateKey
		if mobile != nil && id == mobile.role {
			pub = mobile.key(t)
		} else {
			pub, priv, _ = ed25519.GenerateKey(rand.Reader)
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 2)), NotBefore: root.NotBefore, NotAfter: root.NotAfter,
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}}
		der, err := x509.CreateCertificate(rand.Reader, cert, root, pub, rootKey)
		if err != nil {
			t.Fatal(err)
		}
		keyFile := ""
		if len(priv) != 0 {
			keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
			if err != nil {
				t.Fatal(err)
			}
			keyFile = write(id+".key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
		}
		digest := sha256.Sum256(der)
		leaf, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		keyDigest := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
		certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		if mobile != nil && id == mobile.role {
			mobile.cert, mobile.ca, mobile.rawCertificate = certificate, caPEM, leaf
		}
		certs[id] = identity{write(id+".crt", certificate), keyFile, hex.EncodeToString(digest[:]), hex.EncodeToString(keyDigest[:])}
	}
	coordinatorPublic, coordinator, _ := ed25519.GenerateKey(rand.Reader)
	approvalPublic, approval, _ := ed25519.GenerateKey(rand.Reader)
	recoveryPublic, recovery, _ := ed25519.GenerateKey(rand.Reader)
	secondApproval, secondRecovery := approval, recovery
	if multi {
		_, secondApproval, _ = ed25519.GenerateKey(rand.Reader)
		_, secondRecovery, _ = ed25519.GenerateKey(rand.Reader)
	}
	endpoints := make(map[string]ceremony.Endpoint)
	listeners := make(map[string]net.Listener)
	for _, id := range ids[1:] {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		listeners[id] = l
		endpoints[id] = ceremony.Endpoint{URL: "https://" + l.Addr().String(), CertificateSHA256: certs[id].pin, SPKISHA256: certs[id].spki}
	}
	children := make(map[string]*exec.Cmd)
	configs := make(map[string]string)
	start := func(id string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestParticipantHelper$")
		cmd.Env = append(os.Environ(), "NU54_P03_TEST_PARTICIPANT="+configs[id])
		log, err := os.OpenFile(filepath.Join(dir, id+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer log.Close()
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		children[id] = cmd
	}
	stop := func(id string) {
		t.Helper()
		cmd := children[id]
		if cmd != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			delete(children, id)
		}
	}
	t.Cleanup(func() {
		for id := range children {
			stop(id)
		}
		if t.Failed() {
			for _, id := range ids[1:] {
				d, err := os.ReadFile(filepath.Join(dir, id+".log"))
				if err == nil && len(d) > 0 {
					t.Logf("%s: %s", id, d)
				}
			}
		}
	})
	var restartNative func()
	for _, id := range ids[1:] {
		l := listeners[id]
		address := l.Addr().String()
		l.Close()
		vault := filepath.Join(dir, id+"-vault")
		if err := os.Mkdir(vault, 0700); err != nil {
			t.Fatal(err)
		}
		var key [32]byte
		rand.Read(key[:])
		peerEndpoints := make(map[string]ceremony.Endpoint)
		for peer, e := range endpoints {
			if multi && (peer[1] == '2') != (id[1] == '2') {
				continue
			}

			if peer != id {
				peerEndpoints[peer] = e
			}
		}
		cert := certs[id]
		cfg := ceremony.Config{ID: id, Listen: address, Certificate: cert.cert, CertificateKey: cert.key, ClientCA: caFile,
			CoordinatorCertificateSHA256: certs["coordinator"].pin, CoordinatorSPKISHA256: certs["coordinator"].spki, Vault: vault, VaultKey: write(id+".vault-key", key[:]),
			Peers: peerEndpoints, CoordinatorKey: coordinatorPublic, ApprovalKey: approvalPublic, RecoveryKey: recoveryPublic}
		if multi && id[1] == '2' {
			cfg.ApprovalKey, cfg.RecoveryKey = secondApproval.Public().(ed25519.PublicKey), secondRecovery.Public().(ed25519.PublicKey)
		}
		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		configs[id] = write(id+".json", raw)
		if mobile != nil && id == mobile.role {
			cfg.Certificate, cfg.CertificateKey, cfg.ClientCA, cfg.Vault, cfg.VaultKey = "", "", "", "", ""
			mobile.config = cfg
			continue
		}
		if mobile == nil && id == "a1" && os.Getenv("NU54_P03_ANDROID_SERIAL") != "" {
			if os.Getenv("P03_TEST_DATABASE_URL") == "" {
				t.Fatal("Android participant requires combined API and DB test")
			}
			restartNative = androidParticipant(t, os.Getenv("NU54_P03_ANDROID_SERIAL"), endpoints, configs[id],
				map[string]string{"tls.crt": cert.cert, "tls.key": cert.key, "root.pem": caFile})
			continue
		}
		start(id)
	}
	co := certs["coordinator"]
	controller, err := ceremony.NewController(co.cert, co.key, caFile, endpoints)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := make(map[string]ceremony.Endpoint)
	duplicate["a1"] = endpoints["a1"]
	conflict := endpoints["b1"]
	conflict.SPKISHA256 = endpoints["a1"].SPKISHA256
	duplicate["b1"] = conflict
	if _, err := ceremony.NewController(co.cert, co.key, caFile, duplicate); err == nil {
		t.Fatal("reused TLS public key accepted across two participants")
	}
	if dsn := os.Getenv("P03_TEST_DATABASE_URL"); dsn != "" {
		if multi {
			runMultiWallet(t, dsn, dir, endpoints, co.cert, co.key, caFile, coordinator, [2]ed25519.PrivateKey{approval, secondApproval},
				[2]ed25519.PrivateKey{recovery, secondRecovery})
			return
		}
		runWalletAPI(t, dsn, controller, coordinator, approval, recovery, restartNative, mobile, endpoints, subject)
		return
	}
	cert, err := tls.LoadX509KeyPair(co.cert, co.key)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: ca, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}}, Timeout: 3 * time.Second}
	request := func(id, method, path string, body any) (int, ceremony.Result) {
		t.Helper()
		var out ceremony.Result
		raw, _ := json.Marshal(body)
		req, err := http.NewRequest(method, endpoints[id].URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return 0, out
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
		}
		return resp.StatusCode, out
	}
	launch := func(s ceremony.Session, key ed25519.PrivateKey, ids ...string) ceremony.Authorization {
		t.Helper()
		fingerprint, err := s.Fingerprint(time.Now())
		if err != nil {
			t.Fatal(err)
		}
		message := ceremony.ApprovalMessage(s.Kind, fingerprint)
		auth := ceremony.Authorization{Session: s, Coordinator: ed25519.Sign(coordinator, message), Independent: ed25519.Sign(key, message)}
		for _, id := range ids {
			for attempt := 0; ; attempt++ {
				code, result := request(id, http.MethodPost, "/v1/ceremony", auth)
				if code == 200 && result.State == "running" {
					break
				}
				if code != 0 || attempt >= 30 {
					t.Fatalf("%s failed to reserve %s: %d %s", id, s.Kind, code, result.State)
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		return auth
	}
	wait := func(s ceremony.Session, wants map[string]string) map[string]ceremony.Result {
		t.Helper()
		results := make(map[string]ceremony.Result)
		for time.Now().Unix() < s.Expires {
			for id, expected := range wants {
				code, r := request(id, http.MethodGet, "/v1/ceremony/"+s.ID, nil)
				if code == 200 && r.State == expected {
					results[id] = r
				}
				if r.State == "interrupted" {
					t.Fatalf("%s interrupted %s", id, s.Kind)
				}
			}
			if len(results) == len(wants) {
				return results
			}
			time.Sleep(250 * time.Millisecond)
		}
		t.Fatalf("%s expired with %v", s.Kind, results)
		return nil
	}
	wallet := "d4a523f9-4ca2-4e06-bd94-dc428578d0de"
	old := []ceremony.Peer{{"a1", ceremony.A, 1}, {"b1", ceremony.B, 1}, {"c1", ceremony.C, 1}}
	dkg := ceremony.Session{ID: "9412fb2c-ae29-4fc3-95da-c38f65dc531e", Wallet: wallet, Kind: ceremony.DKG, Epoch: 1,
		Expires: time.Now().Add(14 * time.Minute).Unix(), Peers: old}
	auth := launch(dkg, approval, "a1", "b1", "c1")
	if code, _ := request("a1", http.MethodPost, "/v1/nonce", [32]byte{}); code != 403 {
		t.Fatalf("coordinator could access nonce: %d", code)
	}
	res := wait(dkg, map[string]string{"a1": "ready", "b1": "ready", "c1": "ready"})
	public := res["a1"].PublicKey
	if len(public) != 65 || !bytes.Equal(public, res["b1"].PublicKey) || !bytes.Equal(public, res["c1"].PublicKey) {
		t.Fatal("DKG public key disagreement")
	}
	stop("a1")
	start("a1")
	if got := wait(dkg, map[string]string{"a1": "ready"})["a1"]; !bytes.Equal(got.PublicKey, public) {
		t.Fatal("restart lost committed DKG result")
	}
	if code, result := request("a1", http.MethodPost, "/v1/ceremony", auth); code != 200 || result.State != "ready" {
		t.Fatalf("idempotent DKG changed: %d %s", code, result.State)
	}
	sign := func(id, base string, epoch uint64, peers []ceremony.Peer) {
		t.Helper()
		s := ceremony.Session{ID: id, Wallet: wallet, Kind: ceremony.Sign, Epoch: epoch, KeySession: base,
			Expires: time.Now().Add(3 * time.Minute).Unix(), Peers: peers,
			Digest: sha256.Sum256([]byte("transaction " + id)), Intent: sha256.Sum256([]byte("testnet " + id))}
		a, b := peers[0].ID, peers[1].ID
		fingerprint, err := s.Fingerprint(time.Now())
		if err != nil {
			t.Fatal(err)
		}
		message := ceremony.ApprovalMessage(s.Kind, fingerprint)
		if code, _ := request(b, http.MethodPost, "/v1/ceremony", ceremony.Authorization{Session: s, Coordinator: ed25519.Sign(coordinator, message)}); code != 409 {
			t.Fatalf("coordinator-only signing must fail: %d", code)
		}
		approved := launch(s, approval, b, a)
		got := wait(s, map[string]string{a: "signed", b: "signed"})
		if !bytes.Equal(got[a].Signature, got[b].Signature) || !ceremony.VerifySignature(public, s.Digest, got[a].Signature) {
			t.Fatal("threshold signature mismatch")
		}
		if code, result := request(a, http.MethodPost, "/v1/ceremony", approved); code != 200 || !bytes.Equal(result.Signature, got[a].Signature) {
			t.Fatalf("signing lost-response retry did not return same result: %d", code)
		}
	}
	sign("cf4f81a1-47c6-49de-82bc-45c092cf84f0", dkg.ID, 1, old[:2])
	for _, id := range []string{"b1", "c1"} {
		if code, _ := request(id, http.MethodPut, "/v1/fence", struct {
			Wallet string
			Epoch  uint64
		}{wallet, 1}); code != 204 {
			t.Fatalf("failed to fence %s: %d", id, code)
		}
	}
	if code, result := request("b1", http.MethodGet, "/v1/ceremony/cf4f81a1-47c6-49de-82bc-45c092cf84f0", nil); code != 200 || result.State != "fenced" || len(result.Signature) != 0 {
		t.Fatalf("old participant published a fenced signature: %d %s", code, result.State)
	}
	newPeers := []ceremony.Peer{{"a2", ceremony.A, 2}, {"b2", ceremony.B, 2}, {"c2", ceremony.C, 2}}
	all := append(append([]ceremony.Peer{}, old[1:]...), newPeers...)
	refresh := ceremony.Session{ID: "a052e42c-39e6-4848-8ea0-97d3e5ae7640", Wallet: wallet, Kind: ceremony.Refresh, Epoch: 2, KeySession: dkg.ID,
		Expires: time.Now().Add(14 * time.Minute).Unix(), Peers: all}
	launch(refresh, recovery, "b1", "c1", "a2", "b2", "c2")
	rotated := wait(refresh, map[string]string{"b1": "retired", "c1": "retired", "a2": "ready", "b2": "ready", "c2": "ready"})
	for id, result := range rotated {
		if !bytes.Equal(result.PublicKey, public) {
			t.Fatal(fmt.Sprintf("%s changed wallet public key", id))
		}
	}
	sign("e6e6f1a1-ef68-4d5a-9037-b7b04689fcd1", refresh.ID, 2, newPeers[:2])
}
