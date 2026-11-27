package settlement

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var publicHash = regexp.MustCompile(`^0x[0-9a-f]{64}$`)

// PublicExplorer points only to the deployment operator's trusted public P07
// origin. The public client has no credentials, cookies, or redirects.
type PublicExplorer struct {
	origin url.URL
	client *http.Client
}

func NewPublicExplorer(configured, operatorOrigin string) (*PublicExplorer, error) {
	if configured == "" {
		return nil, nil
	}
	base, err := url.Parse(configured)
	if err != nil || base.Hostname() == "" || base.User != nil || base.Opaque != "" ||
		base.RawPath != "" || (base.Path != "" && base.Path != "/") ||
		base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || strings.Contains(configured, "#") ||
		(base.Scheme != "https" && !(base.Scheme == "http" && (base.Hostname() == "127.0.0.1" || base.Hostname() == "::1"))) {
		return nil, errors.New("public explorer must be a trusted HTTPS origin or local loopback")
	}
	if private, err := url.Parse(operatorOrigin); err == nil &&
		strings.EqualFold(strings.TrimSuffix(private.Hostname(), "."), strings.TrimSuffix(base.Hostname(), ".")) &&
		private.Scheme == base.Scheme && originPort(private) == originPort(base) {
		return nil, errors.New("public explorer must use a different origin from the operator console")
	}
	base.Path = "/"
	return &PublicExplorer{
		origin: *base,
		client: &http.Client{Timeout: 2 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func originPort(origin *url.URL) string {
	if port := origin.Port(); port != "" {
		return port
	}
	if origin.Scheme == "https" {
		return "443"
	}
	return "80"
}

func (explorer *PublicExplorer) link(query url.Values) string {
	page := explorer.origin
	page.RawQuery = query.Encode()
	return page.String()
}

type explorerReceipt struct {
	Merchant, Order, Transaction, Amount string
	Log                                  int32
}

func (explorer *PublicExplorer) verifiedLink(ctx context.Context, target Target, receipt explorerReceipt) string {
	if !financeRef.MatchString(target.Stream) || !publicHash.MatchString(receipt.Merchant) ||
		!publicHash.MatchString(receipt.Transaction) || receipt.Log < 0 {
		return ""
	}
	query := url.Values{
		"stream": {target.Stream}, "merchant": {receipt.Merchant},
		"txHash": {receipt.Transaction}, "logIndex": {strconv.FormatInt(int64(receipt.Log), 10)},
	}
	api := explorer.origin
	api.Path, api.RawQuery = "/v2/receipts", query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.String(), nil)
	if err != nil {
		return ""
	}
	response, err := explorer.client.Do(req)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	const limit = 32768
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || len(raw) > limit {
		return ""
	}
	var data struct {
		Deployment *struct {
			Stream, Chain, Contract, Asset, Status string
			Version                                int
		}
		Receipts []struct {
			Kind, Merchant, Order, TxHash, Amount string
			LogIndex                              *int32
		}
	}
	if json.Unmarshal(raw, &data) != nil || data.Deployment == nil || len(data.Receipts) != 1 {
		return ""
	}
	current, row := data.Deployment, data.Receipts[0]
	if current.Stream != target.Stream || current.Chain != "8283" || current.Contract != lower(target.Contract) ||
		current.Asset != lower(target.Asset) || current.Version != 2 || current.Status != "current" ||
		row.Kind != "paid" || row.Merchant != receipt.Merchant || row.Order != receipt.Order ||
		row.TxHash != receipt.Transaction || row.Amount != receipt.Amount || row.LogIndex == nil || *row.LogIndex != receipt.Log {
		return ""
	}
	return explorer.link(query)
}
