package w10fixture

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
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/crypto"
)

type RenterBoard struct {
	dir, bridge, binary string
	payer               []byte
	Commissioning       commerce.RentalReviewCommissioning
}

func NewRenterBoard(dir, bridge, binary string, payer []byte) (*RenterBoard, error) {
	if !filepath.IsAbs(dir) || !filepath.IsAbs(bridge) || !filepath.IsAbs(binary) || len(payer) != 32 {
		return nil, errors.New("renter C2 requires a private fixture directory, bridge and board")
	}
	board := &RenterBoard{dir: dir, bridge: bridge, binary: binary, payer: bytes.Clone(payer)}
	clock := time.Now()
	ends := clock.Add(8 * time.Minute)
	root, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "W10 fixture CA"}, NotBefore: clock.Add(-time.Minute), NotAfter: ends,
		KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true, IsCA: true}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &root.PublicKey, root)
	if err != nil {
		return nil, err
	}
	if err = writeFixtureFile(dir, "ca.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); err != nil {
		return nil, err
	}
	for _, role := range []struct {
		name   string
		serial int64
	}{{"board", 2}, {"terminal", 3}, {"display", 4}} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		private, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, err
		}
		if err = writeFixtureFile(dir, role.name+".key", pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: private})); err != nil {
			return nil, err
		}
		if role.name == "display" {
			continue
		}
		identity, err := url.Parse("urn:nu54:role:" + role.name)
		if err != nil {
			return nil, err
		}
		leaf := &x509.Certificate{SerialNumber: big.NewInt(role.serial), Subject: pkix.Name{CommonName: "W10 fixture " + role.name}, NotBefore: ca.NotBefore, NotAfter: ends,
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
			URIs: []*url.URL{identity}, BasicConstraintsValid: true}
		raw, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, root)
		if err != nil {
			return nil, err
		}
		if err = writeFixtureFile(dir, role.name+".pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})); err != nil {
			return nil, err
		}
		spki, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			return nil, err
		}
		if role.name == "board" {
			board.Commissioning.BoardPin = sha256.Sum256(spki)
			board.Commissioning.Signer = [65]byte(elliptic.Marshal(key.Curve, key.X, key.Y))
		} else {
			board.Commissioning.TerminalPin = sha256.Sum256(spki)
		}
	}
	board.Commissioning.ExpiresAt = ends
	if err = writeFixtureFile(dir, "payer.key", board.payer); err != nil {
		return nil, err
	}
	return board, nil
}

func writeFixtureFile(dir, name string, data []byte) error {
	file, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	return errors.Join(writeErr, file.Close())
}

func (b *RenterBoard) Decide(ctx context.Context, quote []byte, signer string, approvalDigest, digest [32]byte, deadline time.Time,
	authority *commerce.RentalReviewPresentation, approved bool) ([]byte, error) {
	payload, err := json.Marshal(map[string]any{
		"authority": authority, "quote": hex.EncodeToString(quote), "signer": signer,
		"payment": map[string]any{"deadline": deadline.Unix(), "generation": "1", "digest": hex.EncodeToString(digest[:]),
			"approvalDigest": hex.EncodeToString(approvalDigest[:]), "payer": payerAddress(b.payer)},
	})
	if err != nil {
		return nil, err
	}
	decision, err := b.Exchange(ctx, payload, time.Unix(authority.ExpiresAt, 0), func(_ []byte) (bool, error) {
		return approved, nil
	})
	if err != nil {
		return nil, err
	}
	want := byte(2)
	if approved {
		want = 1
	}
	if decision[36] != want {
		return nil, errors.New("C2 renter board returned a different decision")
	}
	if !approved {
		return nil, nil
	}
	return decision[37:102], nil
}

// Exchange drives one authenticated C2 connection. The switch callback sees the
// independently verified review before a test can select a physical intent.
func (b *RenterBoard) Exchange(ctx context.Context, payload []byte, expiresAt time.Time,
	switchIntent func([]byte) (bool, error)) ([]byte, error) {
	ctx, cancel := context.WithDeadline(ctx, expiresAt)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", b.bridge, b.binary, b.dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 2 * time.Second
	cmd.Stdin = bytes.NewReader(append(payload, '\n'))
	stream, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		if cmd.ProcessState == nil {
			cancel()
			_ = cmd.Wait()
		}
	}()
	events := bufio.NewScanner(stream)
	read := func(name string, size int) ([]byte, error) {
		if !events.Scan() {
			return nil, errors.Join(errors.New("missing C renter board "+name), events.Err())
		}
		var event map[string]string
		if json.Unmarshal(events.Bytes(), &event) != nil || len(event) != 1 {
			return nil, errors.New("malformed C renter board event")
		}
		raw, err := hex.DecodeString(event[name])
		if err != nil || len(raw) != size {
			return nil, errors.New("invalid C renter board evidence")
		}
		return raw, nil
	}
	receipt, err := read("receipt", 296)
	if err != nil {
		_ = cmd.Wait()
		code := regexp.MustCompile(`(?:Error|code): ['"]?([A-Z][A-Z_]+)`).Find(diagnostic.Bytes())
		return nil, errors.Join(err, errors.New(string(code)))
	}
	approved, err := switchIntent(receipt)
	if err != nil {
		return nil, err
	}
	switchValue := "refuse\n"
	if approved {
		switchValue = "approve\n"
	}
	if err = writeFixtureFile(b.dir, "switch.request", []byte(switchValue)); err != nil {
		return nil, err
	}
	if err = os.Rename(filepath.Join(b.dir, "switch.request"), filepath.Join(b.dir, "switch")); err != nil {
		return nil, err
	}
	decision, err := read("decision", 166)
	if err != nil {
		return nil, err
	}
	if err = cmd.Wait(); err != nil {
		return nil, errors.New("C2 renter board did not close cleanly")
	}
	return decision, nil
}

func payerAddress(secret []byte) string {
	key, err := crypto.ToECDSA(secret)
	if err != nil {
		return ""
	}
	return "0x" + hex.EncodeToString(crypto.PubkeyToAddress(key.PublicKey).Bytes())
}
