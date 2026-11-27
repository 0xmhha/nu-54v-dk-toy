package ceremony

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

type Controller struct {
	clients   map[string]*http.Client
	endpoints map[string]Endpoint
}

func NewController(certPath, keyPath, caPath string, endpoints map[string]Endpoint) (*Controller, error) {
	if _, err := readSecret(keyPath, 0); err != nil {
		return nil, err
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	ca := x509.NewCertPool()
	if !ca.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("invalid participant trust roots")
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}
	seenCerts := map[[32]byte]bool{sha256.Sum256(leaf.Raw): true}
	seenKeys := map[[32]byte]bool{sha256.Sum256(leaf.RawSubjectPublicKeyInfo): true}
	c := &Controller{clients: make(map[string]*http.Client, len(endpoints)), endpoints: endpoints}
	for id, endpoint := range endpoints {
		target, err := url.Parse(endpoint.URL)
		if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil || target.Path != "" || target.RawQuery != "" || target.Fragment != "" || !peerID.MatchString(id) || id == "coordinator" {
			return nil, errors.New("invalid participant endpoint")
		}
		pin, err := decodePin(endpoint.CertificateSHA256)
		keyPin, keyErr := decodePin(endpoint.SPKISHA256)
		if err != nil || keyErr != nil || seenCerts[pin] || seenKeys[keyPin] {
			return nil, errors.New("participant identities and TLS keys must be independent")
		}
		seenCerts[pin], seenKeys[keyPin] = true, true
		c.clients[id] = &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{IdleConnTimeout: 30 * time.Second, TLSClientConfig: &tls.Config{
			RootCAs: ca, MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert},
			VerifyConnection: func(cs tls.ConnectionState) error {
				if certificateHash(cs) != pin || keyHash(cs) != keyPin {
					return errors.New("unexpected participant certificate")
				}
				return nil
			},
		}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return c, nil
}

func (c *Controller) Status(ctx context.Context, id, session string) (Result, error) {
	var r Result
	err := c.do(ctx, id, http.MethodGet, "/v1/ceremony/"+session, nil, &r)
	return r, err
}

func (c *Controller) Start(ctx context.Context, id string, auth Authorization) (Result, error) {
	var r Result
	err := c.do(ctx, id, http.MethodPost, "/v1/ceremony", auth, &r)
	return r, err
}

func (c *Controller) Fence(ctx context.Context, id, wallet string, epoch uint64) error {
	return c.do(ctx, id, http.MethodPut, "/v1/fence", struct {
		Wallet string
		Epoch  uint64
	}{wallet, epoch}, nil)
}

func (c *Controller) do(ctx context.Context, id, method, path string, body, result any) error {
	client := c.clients[id]
	if client == nil {
		return errors.New("unknown participant")
	}
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoints[id].URL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return os.ErrNotExist
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("participant returned %d", resp.StatusCode)
	}
	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}
