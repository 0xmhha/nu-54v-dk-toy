package ceremony

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Endpoint struct {
	URL               string
	CertificateSHA256 string
	SPKISHA256        string
}

type Config struct {
	ID                           string
	Listen                       string
	Certificate                  string
	CertificateKey               string
	ClientCA                     string
	CoordinatorCertificateSHA256 string
	CoordinatorSPKISHA256        string
	Vault                        string
	VaultKey                     string
	Peers                        map[string]Endpoint
	CoordinatorKey               ed25519.PublicKey
	ApprovalKey                  ed25519.PublicKey
	RecoveryKey                  ed25519.PublicKey
}

func Serve(ctx context.Context, cfg Config) error {
	key, err := readSecret(cfg.VaultKey, 32)
	if err != nil {
		return err
	}
	defer clear(key)
	return ServeWithVaultKey(ctx, cfg, key, nil)
}

// ServeWithVaultKey starts the same participant using an in-memory vault
// key supplied by the native app. ready runs only after the socket is bound.
func ServeWithVaultKey(ctx context.Context, cfg Config, key []byte, ready func()) error {
	caPEM, err := os.ReadFile(cfg.ClientCA)
	if err != nil {
		return err
	}
	if _, err := readSecret(cfg.CertificateKey, 0); err != nil {
		return err
	}
	cert, err := tls.LoadX509KeyPair(cfg.Certificate, cfg.CertificateKey)
	if err != nil {
		return err
	}
	return ServeWithCredentials(ctx, cfg, key, cert, caPEM, ready)
}

// ServeWithCredentials accepts the decrypted in-process TLS identity of a native device.
func ServeWithCredentials(ctx context.Context, cfg Config, key []byte, cert tls.Certificate, caPEM []byte, ready func()) error {
	v, err := openVault(cfg.Vault, key)
	if err != nil {
		return err
	}
	opened := false
	defer func() {
		if !opened {
			_ = v.close()
		}
	}()
	ca := x509.NewCertPool()
	if !ca.AppendCertsFromPEM(caPEM) {
		return errors.New("invalid participant trust roots")
	}
	if cfg.ID == "coordinator" {
		return errors.New("reserved participant ID")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return err
	}
	selfKey := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	identities := map[[32]byte]pinnedIdentity{sha256.Sum256(cert.Certificate[0]): {name: cfg.ID, key: selfKey}}
	usedKeys := map[[32]byte]bool{selfKey: true}
	coordinatorPin, err := decodePin(cfg.CoordinatorCertificateSHA256)
	coordinatorKey, keyErr := decodePin(cfg.CoordinatorSPKISHA256)
	if err != nil || keyErr != nil || identities[coordinatorPin].name != "" || usedKeys[coordinatorKey] {
		return errors.New("invalid coordinator pin")
	}
	identities[coordinatorPin] = pinnedIdentity{name: "coordinator", key: coordinatorKey}
	usedKeys[coordinatorKey] = true
	clients := make(map[string]*http.Client, len(cfg.Peers))
	for id, endpoint := range cfg.Peers {
		target, err := url.Parse(endpoint.URL)
		if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil || target.Path != "" || target.RawQuery != "" || target.Fragment != "" || !peerID.MatchString(id) {
			return errors.New("invalid participant endpoint")
		}
		pin, err := decodePin(endpoint.CertificateSHA256)
		keyPin, keyErr := decodePin(endpoint.SPKISHA256)
		if err != nil || keyErr != nil || identities[pin].name != "" || usedKeys[keyPin] || id == cfg.ID || id == "coordinator" {
			return errors.New("duplicated peer identity")
		}
		identities[pin] = pinnedIdentity{name: id, key: keyPin}
		usedKeys[keyPin] = true
		transport := &http.Transport{ForceAttemptHTTP2: true, TLSClientConfig: &tls.Config{
			RootCAs: ca, MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert},
			VerifyConnection: func(cs tls.ConnectionState) error {
				if certificateHash(cs) != pin || keyHash(cs) != keyPin {
					return errors.New("unexpected peer certificate")
				}
				return nil
			},
		}}
		defer transport.CloseIdleConnections()
		clients[id] = &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	n, err := newNode(ctx, cfg.ID, v, cfg.CoordinatorKey, cfg.ApprovalKey, cfg.RecoveryKey, func(ctx context.Context, frame Frame) error {
		client := clients[frame.To]
		if client == nil {
			return errors.New("unknown participant route")
		}
		return sendFrame(ctx, client, cfg.Peers[frame.To].URL, frame)
	}, func(ctx context.Context, id string, fingerprint [32]byte) ([32]byte, error) {
		return fetchNonce(ctx, clients[id], cfg.Peers[id].URL, fingerprint)
	})
	if err != nil {
		return err
	}
	opened = true
	defer n.Close()
	mux := http.NewServeMux()
	control := func(fn func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if identity(r, identities) != "coordinator" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			fn(w, r)
		}
	}
	mux.HandleFunc("POST /v1/ceremony", control(func(w http.ResponseWriter, r *http.Request) {
		var auth Authorization
		if !decodeRequest(w, r, &auth, 8192) {
			return
		}
		for _, peer := range auth.Session.Peers {
			if peer.ID != cfg.ID && clients[peer.ID] == nil {
				http.Error(w, "unknown participant", http.StatusBadRequest)
				return
			}
		}
		result, err := n.Start(auth)
		if err != nil {
			http.Error(w, "ceremony unavailable", http.StatusConflict)
			return
		}
		writeJSON(w, result)
	}))
	mux.HandleFunc("GET /v1/ceremony/{id}", control(func(w http.ResponseWriter, r *http.Request) {
		result, err := n.Status(r.PathValue("id"))
		if err != nil {
			http.Error(w, "ceremony unavailable", http.StatusNotFound)
			return
		}
		writeJSON(w, result)
	}))
	mux.HandleFunc("PUT /v1/fence", control(func(w http.ResponseWriter, r *http.Request) {
		var fence struct {
			Wallet string
			Epoch  uint64
		}
		if !decodeRequest(w, r, &fence, 1024) {
			return
		}
		if err := n.Fence(fence.Wallet, fence.Epoch); err != nil {
			http.Error(w, "fence unavailable", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("POST /v1/nonce", func(w http.ResponseWriter, r *http.Request) {
		id := identity(r, identities)
		if clients[id] == nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var fingerprint [32]byte
		if !decodeRequest(w, r, &fingerprint, 1024) {
			return
		}
		nonce, err := n.Nonce(id, fingerprint)
		if errors.Is(err, os.ErrNotExist) {
			w.WriteHeader(http.StatusTooEarly)
			return
		}
		if err != nil {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(nonce[:])
		clear(nonce[:])
	})
	mux.HandleFunc("POST /v1/frame", func(w http.ResponseWriter, r *http.Request) {
		id := identity(r, identities)
		if clients[id] == nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var frame Frame
		if !decodeRequest(w, r, &frame, 1<<19) {
			return
		}
		if err := n.Receive(id, frame); errors.Is(err, os.ErrNotExist) {
			http.Error(w, "session not running", http.StatusTooEarly)
			return
		} else if err != nil {
			http.Error(w, "frame rejected", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
	server := &http.Server{Addr: cfg.Listen, Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 20 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca, Certificates: []tls.Certificate{cert}},
	}
	closed := make(chan error, 1)
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	go func() { closed <- server.ServeTLS(listener, "", "") }()
	if ready != nil {
		ready()
	}
	select {
	case err := <-closed:
		return err
	case <-ctx.Done():
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(stop)
	}
}

func readSecret(filename string, size int) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("private file permissions are invalid")
	}
	b, err := io.ReadAll(io.LimitReader(f, 32769))
	if err != nil {
		return nil, err
	}
	if len(b) > 32768 || size > 0 && len(b) != size {
		return nil, errors.New("private file length is invalid")
	}
	if size == 0 {
		clear(b)
		return nil, nil
	}
	return b, nil
}

func decodePin(encoded string) ([32]byte, error) {
	var out [32]byte
	b, err := hex.DecodeString(encoded)
	if err != nil || len(b) != 32 || encoded != strings.ToLower(encoded) {
		return out, errors.New("invalid SHA-256 pin")
	}
	copy(out[:], b)
	return out, nil
}

func certificateHash(cs tls.ConnectionState) [32]byte {
	if len(cs.PeerCertificates) == 0 {
		return [32]byte{}
	}
	return sha256.Sum256(cs.PeerCertificates[0].Raw)
}

func keyHash(cs tls.ConnectionState) [32]byte {
	if len(cs.PeerCertificates) == 0 {
		return [32]byte{}
	}
	return sha256.Sum256(cs.PeerCertificates[0].RawSubjectPublicKeyInfo)
}

type pinnedIdentity struct {
	name string
	key  [32]byte
}

func identity(r *http.Request, identities map[[32]byte]pinnedIdentity) string {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
		return ""
	}
	id := identities[certificateHash(*r.TLS)]
	if id.key != keyHash(*r.TLS) {
		return ""
	}
	return id.name
}

func decodeRequest(w http.ResponseWriter, r *http.Request, dst any, max int64) bool {
	reader := json.NewDecoder(http.MaxBytesReader(w, r.Body, max))
	reader.DisallowUnknownFields()
	if err := reader.Decode(dst); err == nil && reader.Decode(new(any)) == io.EOF {
		return true
	}
	http.Error(w, "invalid request", http.StatusBadRequest)
	return false
}

func writeJSON(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(result)
}

func sendFrame(ctx context.Context, client *http.Client, endpoint string, frame Frame) error {
	body, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(endpoint, "/")+"/v1/frame", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusAccepted:
				return nil
			case http.StatusTooEarly:
			default:
				return fmt.Errorf("peer rejected round frame (%d)", resp.StatusCode)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func fetchNonce(ctx context.Context, client *http.Client, endpoint string, fingerprint [32]byte) ([32]byte, error) {
	var zero [32]byte
	if client == nil {
		return zero, errors.New("nonce leader unavailable")
	}
	body, err := json.Marshal(fingerprint)
	if err != nil {
		return zero, err
	}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/nonce", bytes.NewReader(body))
		if err != nil {
			return zero, err
		}
		resp, err := client.Do(req)
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				data, err := io.ReadAll(io.LimitReader(resp.Body, 33))
				resp.Body.Close()
				if err != nil || len(data) != 32 {
					return zero, errors.New("invalid nonce response")
				}
				defer clear(data)
				result := [32]byte(data)
				if result == zero || result == fingerprint {
					return zero, errors.New("invalid session nonce")
				}
				return result, nil
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusTooEarly {
				return zero, errors.New("nonce request denied")
			}
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// ValidEndpoint checks that configured peer origins use the fixed HTTPS transport.
func ValidEndpoint(e Endpoint) bool {
	target, err := url.Parse(e.URL)
	return err == nil && target.Scheme == "https" && target.Hostname() != "" && target.User == nil &&
		target.Path == "" && target.RawQuery == "" && target.Fragment == ""
}
