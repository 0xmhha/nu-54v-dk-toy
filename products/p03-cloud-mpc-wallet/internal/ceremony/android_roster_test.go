package ceremony_test

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/ceremony"
	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/mobileconfig"
)

type mobileTest struct {
	serial, role, subject, requests, approved string
	csr                                       string
	ios                                       bool
	bundle, container                         string
	cert                                      []byte
	ca                                        []byte
	config                                    ceremony.Config
	rawCertificate                            *x509.Certificate
	sign                                      ed25519.PrivateKey
	restart                                   func()
}

func readMobileTest(t *testing.T, base, subject string) *mobileTest {
	t.Helper()
	seedFile := os.Getenv("NU54_P03_ANDROID_ROSTER_SEED")
	ios := os.Getenv("NU54_P03_IOS_SIMULATOR") != ""
	if ios {
		seedFile = os.Getenv("NU54_P03_IOS_ROSTER_SEED")
	}
	if seedFile == "" {
		return nil
	}
	serial := os.Getenv("NU54_P03_ANDROID_SERIAL")
	role := os.Getenv("NU54_P03_ANDROID_ROLE")
	if ios {
		serial = os.Getenv("NU54_P03_IOS_SIMULATOR")
		role = os.Getenv("NU54_P03_IOS_ROLE")
	}
	if serial == "" || role != "a1" && role != "a2" && role != "a3" {
		t.Fatal("signed Android fixture requires a supported role")
	}
	seed, err := os.ReadFile(seedFile)
	if err != nil || len(seed) != ed25519.SeedSize {
		t.Fatal("invalid synthetic roster seed file")
	}
	defer clear(seed)
	m := &mobileTest{ios: ios, serial: serial, role: role, subject: subject, requests: filepath.Join(base, "mobile-requests"),
		approved: filepath.Join(base, "mobile-approved"), sign: ed25519.NewKeyFromSeed(seed)}
	for _, dir := range []string{m.requests, m.approved} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { clear(m.sign) })
	if ios {
		m.iosPrepare(t)
		return m
	}
	cmd := exec.Command("adb", "-s", serial, "shell", "am", "instrument", "-w", "-e", "mode", "prepare", "-e", "subject", subject,
		"com.nu54renter.test/com.nu54renter.ble.CloudWalletParticipantLab")
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal("Android did not prepare a public CSR")
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		encoded, ok := strings.CutPrefix(line, "INSTRUMENTATION_RESULT: p03CSR=")
		if !ok {
			continue
		}
		public, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatal("invalid public CSR fixture")
		}
		m.csr = string(public)
		break
	}
	if m.csr == "" {
		t.Fatal("Android did not return a public CSR")
	}
	return m
}

func (m *mobileTest) key(t *testing.T) ed25519.PublicKey {
	t.Helper()
	block, _ := pem.Decode([]byte(m.csr))
	if block == nil {
		t.Fatal("invalid Android public CSR")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil {
		t.Fatal("invalid Android CSR signature")
	}
	key, ok := csr.PublicKey.(ed25519.PublicKey)
	if !ok {
		t.Fatal("Android CSR did not use Ed25519")
	}
	return key
}

func (m *mobileTest) publish(t *testing.T, epoch uint64) {
	t.Helper()
	nameHash := sha256.Sum256(m.rawCertificate.RawSubjectPublicKeyInfo)
	name := m.subject + "-" + hex.EncodeToString(nameHash[:])
	if _, err := os.Stat(filepath.Join(m.requests, name+".csr")); err != nil {
		t.Fatal("normal API did not reserve Android public CSR")
	}
	r := mobileconfig.Roster{Subject: m.subject, Epoch: epoch, Expires: m.rawCertificate.NotAfter.Unix(),
		Certificate: string(m.cert), CA: string(m.ca), Config: m.config}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(mobileconfig.Envelope{Payload: raw, Signature: ed25519.Sign(m.sign, append([]byte(mobileconfig.Domain), raw...))})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.approved, name+".json"), out, 0600); err != nil {
		t.Fatal(err)
	}
}

func (m *mobileTest) launch(t *testing.T, endpoints map[string]ceremony.Endpoint, roster []byte, control *ceremony.Controller) {
	t.Helper()
	if m.ios {
		m.iosServe(t, roster)
		return
	}
	adb := func(args ...string) {
		t.Helper()
		cmd := exec.Command("adb", append([]string{"-s", m.serial}, args...)...)
		if err := cmd.Run(); err != nil {
			t.Fatalf("Android roster fixture command failed: %v", err)
		}
	}
	for id, endpoint := range endpoints {
		u, err := url.Parse(endpoint.URL)
		if err != nil {
			t.Fatal(err)
		}
		port := "tcp:" + u.Port()
		route := "reverse"
		if id == m.role {
			route = "forward"
		}
		adb(route, "--no-rebind", port, port)
		t.Cleanup(func() { _ = exec.Command("adb", "-s", m.serial, route, "--remove", port).Run() })
	}
	var active *exec.Cmd
	stop := func() {
		if active == nil {
			return
		}
		_ = exec.Command("adb", "-s", m.serial, "shell", "am", "force-stop", "com.nu54renter").Run()
		_ = active.Process.Kill()
		_ = active.Wait()
		active = nil
	}
	start := func(args ...string) {
		stop()
		params := []string{"-s", m.serial, "shell", "am", "instrument", "-w", "-e", "subject", m.subject}
		params = append(params, args...)
		params = append(params, "com.nu54renter.test/com.nu54renter.ble.CloudWalletParticipantLab")
		active = exec.Command("adb", params...)
		out, err := active.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := active.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan struct{})
		go func() { _, _ = io.Copy(io.Discard, out); close(exited) }()
		deadline := time.Now().Add(25 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case <-exited:
				stop()
				t.Fatal("Android participant exited before ready")
			default:
			}
			probe, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
			_, err := control.Status(probe, m.role, "62e685e3-3d68-4e15-933d-35743331c346")
			cancel()
			if errors.Is(err, os.ErrNotExist) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		stop()
		t.Fatal("Android participant readiness timed out")

	}
	start("-e", "roster", base64.StdEncoding.EncodeToString(roster))
	t.Cleanup(stop)
	m.restart = func() {
		adb("shell", "-T", "run-as com.nu54renter sh -c 'test -s no_backup/cloud-p03-enrolled-"+m.subject+"/device-tls.sealed && test -s no_backup/cloud-p03-enrolled-"+m.subject+"/vault-key.sealed && ! test -e no_backup/cloud-p03-enrolled-"+m.subject+"/tls.key'")
		start()
	}
}
