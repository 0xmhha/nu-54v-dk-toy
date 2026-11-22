package commerce

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
)

type installedMenuBoard struct {
	mu                   sync.Mutex
	dir, host, validator string
	ctx                  context.Context
	service              *exec.Cmd
	address              string
	connection           net.Conn
	payer                *ecdsa.PrivateKey
	display              *ecdsa.PrivateKey
	decision             byte
	stage                string
}

func prepareInstalledMenuBoard(t *testing.T, ctx context.Context) *installedMenuBoard {
	t.Helper()
	host, validator := os.Getenv("NU54_W10_BOARD_HOST"), os.Getenv("NU54_W10_BOARD_VALIDATOR")
	if !filepath.IsAbs(host) || !filepath.IsAbs(validator) {
		t.Fatal("W10 requires the built host board and installation verifier")
	}
	payer, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	b := &installedMenuBoard{dir: t.TempDir(), host: host, validator: validator, ctx: ctx, payer: payer}
	t.Cleanup(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.disconnect()
		if b.service != nil {
			_ = syscall.Kill(-b.service.Process.Pid, syscall.SIGKILL)
			_ = b.service.Wait()
		}
	})
	return b
}

func (b *installedMenuBoard) disconnect() {
	if b.connection != nil {
		_ = b.connection.Close()
		b.connection = nil
	}
}

func (b *installedMenuBoard) enroll(raw []byte, signer []byte) (any, error) {
	if b.service != nil {
		return nil, errors.New("board setup already fixed")
	}
	der, err := base64.StdEncoding.DecodeString(string(raw))
	if err != nil {
		return nil, err
	}
	pub, err := x509.ParsePKIXPublicKey(der)
	phone, ok := pub.(*ecdsa.PublicKey)
	if err != nil || !ok || phone.Curve != elliptic.P256() {
		return nil, errors.New("invalid owner public identity")
	}
	board, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	ca, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	b.display, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	cert := func(serial int64, name string) *x509.Certificate {
		return &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: time.Now().Add(-time.Minute),
			NotAfter: time.Now().Add(time.Hour), Subject: pkix.Name{CommonName: "NU54 W10 synthetic " + name},
			BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature}
	}
	makeCert := func(template, parent *x509.Certificate, public *ecdsa.PublicKey, signer *ecdsa.PrivateKey) ([]byte, error) {
		return x509.CreateCertificate(rand.Reader, template, parent, public, signer)
	}
	material := func(name, kind string, raw []byte) error {
		return os.WriteFile(filepath.Join(b.dir, name), pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: raw}), 0600)
	}
	root := cert(1, "CA")
	root.IsCA = true
	root.KeyUsage |= x509.KeyUsageCertSign
	rootDER, err := makeCert(root, root, &ca.PublicKey, ca)
	if err != nil {
		return nil, err
	}
	copyRoot, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return nil, err
	}
	mint := func(name string, num int64, role string, usage x509.ExtKeyUsage, pub *ecdsa.PublicKey) ([]byte, error) {
		c := cert(num, role)
		c.ExtKeyUsage = []x509.ExtKeyUsage{usage}
		c.URIs = []*url.URL{{Scheme: "urn", Opaque: "nu54:role:" + role}}
		wire, err := makeCert(c, copyRoot, pub, ca)
		if err != nil {
			return nil, err
		}
		return wire, material(name+".pem", "CERTIFICATE", wire)
	}
	if err = material("ca.pem", "CERTIFICATE", rootDER); err != nil {
		return nil, err
	}
	boardDER, err := mint("board", 2, "board", x509.ExtKeyUsageServerAuth, &board.PublicKey)
	if err != nil {
		return nil, err
	}
	ownerDER, err := mint("owner", 3, "owner", x509.ExtKeyUsageClientAuth, phone)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(board)
	if err != nil {
		return nil, err
	}
	if err = material("board.key", "EC PRIVATE KEY", keyDER); err != nil {
		return nil, err
	}
	display, err := x509.MarshalPKIXPublicKey(&b.display.PublicKey)
	if err != nil {
		return nil, err
	}
	if err = material("display-key", "PUBLIC KEY", display); err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(b.dir, "payment-key.fixture"), append(bytes.Clone(signer), crypto.FromECDSA(b.payer)...), 0600); err != nil {
		return nil, err
	}
	identity, err := exec.CommandContext(b.ctx, b.host, b.dir, "inspect").Output()
	if err != nil {
		return nil, err
	}
	fields := bytes.Fields(identity)
	if len(fields) < 3 {
		return nil, errors.New("missing board identity")
	}
	device, err := hex.DecodeString(string(fields[2]))
	if err != nil {
		return nil, err
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(device))
	boardCert, err := x509.ParseCertificate(boardDER)
	if err != nil {
		return nil, err
	}
	pin := fmt.Sprintf("%x", sha256.Sum256(boardCert.RawSubjectPublicKeyInfo))
	service := exec.CommandContext(b.ctx, b.host, b.dir, "serve", "0", "8", "240")
	service.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	service.Env = append(os.Environ(), "NU54_BOARD_PAYMENT_INSTALL_TEST="+b.validator)
	stdout, err := service.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = service.Start(); err != nil {
		return nil, err
	}
	b.service = service
	reader := bufio.NewScanner(stdout)
	if !reader.Scan() {
		return nil, errors.New("board did not bind")
	}
	var port int
	if _, err = fmt.Sscanf(reader.Text(), "PORT %d", &port); err != nil || port < 1 || port > 65535 {
		return nil, errors.New("invalid board port")
	}
	go func() {
		for reader.Scan() {
		}
	}()
	b.address = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if portFile := os.Getenv("NU54_W10_IOS_HOST_PORT_FILE"); portFile != "" {
		if !filepath.IsAbs(portFile) || filepath.Base(portFile) != "w04-host-port" ||
			filepath.Base(filepath.Dir(portFile)) != "Documents" {
			return nil, errors.New("invalid iOS QA port file")
		}
		file, openErr := os.OpenFile(portFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if openErr != nil {
			return nil, openErr
		}
		_, writeErr := io.WriteString(file, strconv.Itoa(port))
		if err := errors.Join(writeErr, file.Close()); err != nil {
			return nil, err
		}
	}
	return map[string]any{"version": 2, "exercised": false, "mode": "personal",
		"enrollment": map[string]any{"version": 2, "device": fingerprint, "boardSPKI": pin,
			"ca": base64.StdEncoding.EncodeToString(rootDER), "certificateChain": []string{base64.StdEncoding.EncodeToString(ownerDER)}},
		"verified": map[string]any{"boardFingerprint": fingerprint, "address": hexAddress(crypto.PubkeyToAddress(b.payer.PublicKey)),
			"network": "8283", "generation": 1}}, nil
}

func (b *installedMenuBoard) exchange(path string, raw []byte, menu *installedSignedMenu) (any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch path {
	case "/owner/stage":
		switch string(raw) {
		case "profile-read", "native-identity", "host-enrollment", "profile-save", "owner-ready":
			b.stage = string(raw)
		default:
			return nil, errors.New("invalid diagnostic stage")
		}
	case "/owner/enroll":
		profile, err := b.enroll(raw, menu.profile.Signer.Bytes())
		if err != nil {
			b.stage = "host-enrollment-failed"
		}
		return profile, err
	case "/owner/open":
		b.disconnect()
		if b.address == "" {
			return nil, errors.New("board not enrolled")
		}
		connection, err := net.DialTimeout("tcp", b.address, time.Second)
		if err != nil {
			return nil, err
		}
		b.connection = connection
	case "/owner/close":
		b.disconnect()
	case "/owner/write":
		frame, err := base64.StdEncoding.DecodeString(string(raw))
		if err != nil || len(frame) == 0 || b.connection == nil {
			return nil, errors.New("invalid board frame")
		}
		_ = b.connection.SetWriteDeadline(time.Now().Add(time.Second))
		if _, err := b.connection.Write(frame); err != nil {
			return nil, err
		}
	case "/owner/read":
		if b.connection == nil {
			return map[string]any{"closed": true}, nil
		}
		_ = b.connection.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		frame := make([]byte, 16384)
		n, err := b.connection.Read(frame)
		if n > 0 {
			return map[string]any{"fragment": base64.StdEncoding.EncodeToString(frame[:n])}, nil
		}
		if errors.Is(err, io.EOF) {
			b.disconnect()
			return map[string]any{"closed": true}, nil
		}
		if netErr, ok := err.(net.Error); err != nil && (!ok || !netErr.Timeout()) {
			return nil, err
		}
	case "/owner/sw1", "/owner/sw2":
		button := byte(2)
		if path == "/owner/sw1" {
			if os.Getenv("NU54_W10_INSTALLED_SETTLEMENT") != "1" {
				return nil, errors.New("SW1 requires a dedicated settlement fixture")
			}
			button = 1
		}
		if b.decision != 0 {
			return nil, errors.New("a board decision was already sent")
		}
		rawQuote, err := os.ReadFile(filepath.Join(b.dir, "display-request"))
		if err != nil {
			return nil, errors.New("board did not present a verified quote")
		}
		challenge, err := os.ReadFile(filepath.Join(b.dir, "display-challenge"))
		if err != nil || len(challenge) != 32 {
			return nil, errors.New("board did not challenge the independent display")
		}
		menu.mu.Lock()
		matching := hex.EncodeToString(rawQuote) == menu.initial.Signed && menu.retrieved && menu.initial.Presentation.GasSponsor != nil
		menu.mu.Unlock()
		if !matching {
			return nil, errors.New("board request differs from original sponsored quote")
		}
		message := append(append([]byte("N54D"), challenge...), rawQuote...)
		digest := sha256.Sum256(message)
		r, s, err := ecdsa.Sign(rand.Reader, b.display, digest[:])
		if err != nil {
			return nil, err
		}
		proof := make([]byte, 64)
		r.FillBytes(proof[:32])
		s.FillBytes(proof[32:])
		if err = os.WriteFile(filepath.Join(b.dir, "display-proof"), proof, 0600); err != nil {
			return nil, err
		}
		decision := filepath.Join(b.dir, "display-decision")
		if err = os.WriteFile(decision+".new", []byte{button}, 0600); err != nil {
			return nil, err
		}
		if err = os.Rename(decision+".new", decision); err != nil {
			return nil, err
		}
		b.decision = button
	default:
		return nil, errors.New("unknown board operation")
	}
	return struct{}{}, nil
}

func (b *installedMenuBoard) serve(w http.ResponseWriter, r *http.Request, menu *installedSignedMenu) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
	if err == nil {
		answer, handleErr := b.exchange(r.URL.Path, raw, menu)
		if handleErr == nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(answer)
			return
		}
	}
	http.Error(w, "isolated board operation failed", http.StatusConflict)
}
