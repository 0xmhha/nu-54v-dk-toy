package chainview

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

var errQuoteOriginDenied = errors.New("quote origin unknown or outside scope")

type QuoteOriginReader struct {
	Client *http.Client
	URL    string
}

func (s QuoteOriginReader) Read(ctx context.Context, query service.QuoteBoundaryLookup) (service.QuoteBoundaryOrigin, error) {
	var out service.QuoteBoundaryOrigin
	data, err := json.Marshal(query)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(data))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	reply, err := s.Client.Do(req)
	if err != nil {
		return out, err
	}
	defer reply.Body.Close()
	if reply.StatusCode == http.StatusNotFound {
		return out, errQuoteOriginDenied
	}
	if reply.StatusCode != http.StatusOK {
		return out, errors.New("issuer read unavailable")
	}
	const max = 4096
	body, err := io.ReadAll(io.LimitReader(reply.Body, max+1))
	if err != nil || len(body) > max {
		return out, errors.New("invalid issuer response")
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return out, err
	}
	if out.QuoteBoundaryLookup != query || !hexKey(out.Digest, 64) || out.IssuedAt <= 0 || out.ExpiresAt <= out.IssuedAt {
		return out, errQuoteOriginDenied
	}
	return out, nil
}

type quoteCoverage struct {
	chain, contract, asset, manifest, abi, codeHash string
	Base                                            int64     `json:"coverageStart"`
	Height                                          int64     `json:"height"`
	Hash                                            string    `json:"hash"`
	Time                                            time.Time `json:"time"`
}

func captureQuoteCoverage(ctx context.Context, db absenceQuery, stream string) (quoteCoverage, error) {
	var c quoteCoverage
	err := db.QueryRow(ctx, "SELECT * FROM chainview.explorer_quote_coverage($1)", stream).
		Scan(&c.chain, &c.contract, &c.asset, &c.manifest, &c.abi, &c.codeHash, &c.Base, &c.Height, &c.Hash, &c.Time)
	return c, err
}

func (c quoteCoverage) matches(origin service.QuoteBoundaryOrigin) bool {
	return c.chain == origin.Chain && c.contract == origin.Contract && c.asset == origin.Asset &&
		c.manifest == origin.Manifest && c.abi == origin.ABI && c.codeHash == origin.CodeHash
}

type originalBoundary struct {
	Kind           string            `json:"kind"`
	QuoteIssuedAt  time.Time         `json:"quoteIssuedAt"`
	QuoteExpiresAt time.Time         `json:"quoteExpiresAt"`
	QuoteDigest    string            `json:"quoteDigest"`
	Coverage       quoteCoverage     `json:"coverage"`
	Evidence       *boundaryEvidence `json:"evidence,omitempty"`
}

func quoteBoundaryResult(c quoteCoverage, origin service.QuoteBoundaryOrigin, evidence boundaryEvidence) originalBoundary {
	out := originalBoundary{Kind: "pending", Coverage: c, QuoteIssuedAt: time.Unix(origin.IssuedAt, 0).UTC(),
		QuoteExpiresAt: time.Unix(origin.ExpiresAt, 0).UTC(), QuoteDigest: origin.Digest}
	if evidence.Policy != origin.Manifest || evidence.Anchor > c.Height || evidence.AnchorTime == nil {
		return out
	}
	switch evidence.Kind {
	case "paid":
		if evidence.EventQuoteDigest == nil || *evidence.EventQuoteDigest != origin.Digest || evidence.IncludedAt == nil ||
			evidence.IncludedAt.Before(out.QuoteIssuedAt) || !evidence.IncludedAt.Before(out.QuoteExpiresAt) {
			return out
		}
	case "absent":
		expected := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(fmt.Sprintf("nu54-absence-v2\n%s\n%s\n%s\n%s\n%s\n%d\nfalse",
			origin.Manifest, origin.Stream, origin.Merchant, origin.Order, evidence.AnchorHash, origin.ExpiresAt))))
		if evidence.Proof == nil || *evidence.Proof != expected || evidence.CoverageStart == nil || *evidence.CoverageStart != c.Base ||
			evidence.AnchorTime.Before(out.QuoteExpiresAt) || evidence.Expires == nil || !time.Now().Before(*evidence.Expires) {
			return out
		}
	default:
		return out
	}
	evidence.QuoteIssuedAt, evidence.QuoteExpiresAt, evidence.QuoteDigest = &out.QuoteIssuedAt, &out.QuoteExpiresAt, &out.QuoteDigest
	out.Kind, out.Evidence = evidence.Kind, &evidence
	return out
}
