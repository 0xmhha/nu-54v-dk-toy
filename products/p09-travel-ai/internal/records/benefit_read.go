package records

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

type BenefitStatus struct {
	Credited   int64 `json:"credited"`
	Adjusted   int64 `json:"adjusted"`
	Uncredited int64 `json:"uncredited"`
}

type benefitReader struct {
	endpoint string
	client   *http.Client
}

func (s *Service) EnableBenefitReader(endpoint, caFile, certFile, keyFile string) error {
	target, err := url.Parse(endpoint)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil ||
		target.RawQuery != "" || target.Fragment != "" || target.EscapedPath() != "/v1/visit-benefits/status" {
		return errors.New("invalid P05 benefit status endpoint")
	}
	ca, err := os.ReadFile(caFile)
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		return errors.New("P05 benefit status authority unavailable")
	}
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return errors.New("P05 benefit reader identity unavailable")
	}
	s.benefitRead = &benefitReader{endpoint: endpoint, client: &http.Client{
		Timeout:       2 * time.Second,
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{cert}}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
	return nil
}

func (reader *benefitReader) attach(ctx context.Context, choices []VisitOption) bool {
	if reader == nil {
		return false
	}
	type evidence struct {
		Issuer string `json:"issuer"`
		Proof  string `json:"proof"`
	}
	var scope []evidence
	var selected []int
	for i, item := range choices {
		if item.Environment != "live" {
			continue
		}
		scope = append(scope, evidence{item.Issuer, item.Proof})
		selected = append(selected, i)
	}
	if len(scope) == 0 {
		return true
	}
	body, err := json.Marshal(scope)
	if err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reader.endpoint, bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := reader.client.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16385))
	var statuses []BenefitStatus
	if err != nil || len(raw) > 16384 || json.Unmarshal(raw, &statuses) != nil || len(statuses) != len(scope) {
		return false
	}
	for _, state := range statuses {
		if state.Credited < 0 || state.Adjusted < 0 || state.Uncredited < state.Adjusted {
			return false
		}
	}
	for i, state := range statuses {
		choices[selected[i]].Benefit = &state
	}
	return true
}
