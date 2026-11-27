package main

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
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/evidencev2"
)

var generationUUID = regexp.MustCompile(`^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)

func runRentalGeneration(gate *service.RecoveryGate) error {
	for _, suffix := range []string{"DATABASE_URL", "RPC", "PROFILE", "PROFILE_KEY", "PROFILE_SIGNATURE",
		"CA", "CERT", "KEY", "GRANTS", "AUTHORITY_ORIGIN", "AUTHORITY_CA", "AUTHORITY_CERT", "AUTHORITY_KEY", "AUTHORITY_SPKI"} {
		if os.Getenv("P07_RENTAL_"+suffix) == "" {
			return fmt.Errorf("P07_RENTAL_%s is required", suffix)
		}
	}
	profile, err := loadRentalProfile()
	if err != nil {
		return err
	}
	trust, err := rentalTrust("P07_RENTAL_CA")
	if err != nil {
		return err
	}
	source := grantFile[evidencev2.GenerationGrant](os.Getenv("P07_RENTAL_GRANTS"))
	grants, err := source.read()
	if err != nil || len(grants) == 0 {
		return errors.New("P07_RENTAL_GRANTS must contain dedicated certificate grants")
	}
	for _, grant := range grants {
		if !validGenerationGrant(grant, profile.StreamID) {
			return errors.New("invalid P07 rental scope")
		}
	}
	authority, transport, err := rentalAuthority(profile)
	if err != nil {
		return err
	}
	defer transport.CloseIdleConnections()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var repo *chainview.Repository
	var rpc *chainview.RPCSource
	defer func() {
		if repo != nil {
			repo.Close()
		}
		if rpc != nil {
			rpc.Close()
		}
	}()
	reader := &chainview.Reader{Profile: profile}
	endpoint := os.Getenv("P07_RENTAL_RPC")
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" ||
		(parsed.Scheme != "https" && !(parsed.Scheme == "http" && net.ParseIP(parsed.Hostname()).IsLoopback())) {
		return errors.New("P07 rental RPC requires HTTPS, or HTTP on loopback")
	}
	if err := gate.Run(ctx, func(ctx context.Context) error {
		start, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		repo, err = chainview.OpenGenerationReader(start, os.Getenv("P07_RENTAL_DATABASE_URL"))
		if err != nil {
			return err
		}
		rpc, err = chainview.Dial(start, endpoint, profile)
		if err != nil {
			return errors.New("could not verify signed P07 rental deployment over RPC")
		}
		if _, err = rpc.VerifyAssetDecimals(start); err != nil {
			return errors.New("could not verify signed P07 rental asset")
		}
		reader.DB, reader.Source = repo, rpc
		return reader.CheckGenerationReady(start)
	}); err != nil {
		return err
	}
	lookup := func(fingerprint string) (evidencev2.GenerationGrant, bool) {
		grant, ok := source.lookup(fingerprint)
		return grant, ok && validGenerationGrant(grant, profile.StreamID)
	}
	getReader := func(stream string) (evidencev2.GenerationReader, bool) {
		return reader, stream == profile.StreamID
	}
	verifier := evidencev2.RegisteredGenerationVerifier{Authority: authority, Clock: time.Now}
	mux := http.NewServeMux()
	mux.Handle(rentalv1.ReconcilePath, evidencev2.GenerationHandler(lookup, verifier, getReader, time.Now))
	mux.Handle(rentalv1.CloseAnchorPath, evidencev2.GenerationCloseAnchorHandler(lookup, authority, profile, reader, time.Now))
	server := &http.Server{
		Addr: env("P07_RENTAL_HTTP_ADDR", "127.0.0.1:8445"), TLSConfig: evidencev2.TLSConfig(trust),
		Handler:           gate.Handler(mux),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	err = server.ListenAndServeTLS(os.Getenv("P07_RENTAL_CERT"), os.Getenv("P07_RENTAL_KEY"))
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func validGenerationGrant(grant evidencev2.GenerationGrant, stream string) bool {
	return grant.Stream == stream && !grant.ExpiresAt.IsZero() && generationUUID.MatchString(grant.Branch) &&
		generationUUID.MatchString(grant.Board) && grant.Branch != "00000000-0000-0000-0000-000000000000" &&
		grant.Board != "00000000-0000-0000-0000-000000000000"
}

func loadRentalProfile() (chainview.Profile, error) {
	var profile chainview.Profile
	file, err := os.Open(os.Getenv("P07_RENTAL_PROFILE"))
	if err != nil {
		return profile, errors.New("could not open P07 rental profile")
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(raw) > 4096 {
		return profile, errors.New("invalid P07 rental profile size")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	key, keyErr := hex.DecodeString(os.Getenv("P07_RENTAL_PROFILE_KEY"))
	signature, sigErr := hex.DecodeString(os.Getenv("P07_RENTAL_PROFILE_SIGNATURE"))
	if decoder.Decode(&profile) != nil || decoder.Decode(new(any)) != io.EOF || profile.Version != 1 || profile.ChainID != 8283 ||
		keyErr != nil || sigErr != nil || len(key) != ed25519.PublicKeySize ||
		!chainview.VerifyProfileApproval(profile, key, signature) {
		return profile, errors.New("rental reconciliation requires an approved v1 profile")
	}
	return profile, nil
}

func rentalTrust(key string) (*x509.CertPool, error) {
	pem, err := os.ReadFile(os.Getenv(key))
	ca := x509.NewCertPool()
	if err != nil || !ca.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("%s must contain a CA certificate", key)
	}
	return ca, nil
}

func rentalAuthority(profile chainview.Profile) (evidencev2.RemoteGenerationAuthority, *http.Transport, error) {
	invalid := evidencev2.RemoteGenerationAuthority{}
	u, err := url.Parse(os.Getenv("P07_RENTAL_AUTHORITY_ORIGIN"))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return invalid, nil, errors.New("P07 rental authority requires an HTTPS origin")
	}
	ca, err := rentalTrust("P07_RENTAL_AUTHORITY_CA")
	if err != nil {
		return invalid, nil, err
	}
	cert, err := tls.LoadX509KeyPair(os.Getenv("P07_RENTAL_AUTHORITY_CERT"), os.Getenv("P07_RENTAL_AUTHORITY_KEY"))
	if err != nil {
		return invalid, nil, errors.New("P07 rental authority requires a dedicated client certificate and key")
	}
	pin, err := hex.DecodeString(os.Getenv("P07_RENTAL_AUTHORITY_SPKI"))
	if err != nil || len(pin) != sha256.Size {
		return invalid, nil, errors.New("P07 rental authority requires the SHA-256 leaf SPKI pin")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: ca, Certificates: []tls.Certificate{cert},
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 {
				return errors.New("unverified P05 authority certificate")
			}
			digest := sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo)
			if !bytes.Equal(digest[:], pin) {
				return errors.New("P05 authority certificate pin mismatch")
			}
			return nil
		},
	}}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	u.Path = rentalv1.AuthorityPath
	return evidencev2.RemoteGenerationAuthority{Client: client, URL: u.String(), Profile: profile, Clock: time.Now}, transport, nil
}
