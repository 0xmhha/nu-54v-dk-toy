package core

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

// ReadFinalizedV2 verifies a C7 response against the existing scoped order.
// The caller supplies a client with verified server TLS and its C7 mTLS identity;
// this adapter returns proof for reconciliation without applying any ledger effect.
func ReadFinalizedV2(ctx context.Context, client *http.Client, host string, req service.Request,
	principal service.Principal, merchantKey, orderKey string, now time.Time,
) (*service.Evidence, error) {
	started := time.Now()
	if client == nil {
		return nil, service.Forbidden
	}
	if req.Contract != "C7" || req.Operation != "evidence.read" || req.Body["kind"] != "finalized" {
		return nil, service.Capability
	}
	bound, err := service.Authorize(req, principal, now)
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(host)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil ||
		(endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, service.BadRequest
	}
	endpoint.Path = "/v2/evidence/read"
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	// C7 bodies contain scoped evidence references. Do not forward to another endpoint.
	reader := *client
	reader.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := reader.Do(request)
	if err != nil {
		return nil, service.Unknown
	}
	defer response.Body.Close()
	if response.TLS == nil || len(response.TLS.VerifiedChains) == 0 {
		return nil, service.Forbidden
	}
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			return nil, service.Forbidden
		}
		return nil, service.Unknown
	}
	reply, err := service.DecodeReply(response.Body)
	if err != nil {
		return nil, err
	}
	if err := service.ValidateReply(bound, reply, now.Add(time.Since(started))); err != nil {
		return nil, err
	}
	if reply.Status != "finalized" || reply.Evidence == nil ||
		reply.Evidence.Merchant != merchantKey || reply.Evidence.Order != orderKey {
		return nil, service.Mismatch
	}
	return reply.Evidence, nil
}
