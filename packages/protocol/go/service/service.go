// Package service validates the C3–C9 v2 boundaries. Credentials and verified
// evidence originate in transport/domain adapters, outside the request body.
package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/big"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Code string

const (
	BadRequest       Code = "BAD_REQUEST"
	Version          Code = "UNSUPPORTED_VERSION"
	Capability       Code = "UNSUPPORTED_CAPABILITY"
	Forbidden        Code = "NOT_AUTHORIZED"
	Expired          Code = "EXPIRED"
	Conflict         Code = "REPLAY_CONFLICT"
	CheckoutConflict Code = "CHECKOUT_CONFLICT"
	Busy             Code = "BUSY"
	Unknown          Code = "UNKNOWN_RESULT"
	Mismatch         Code = "MISMATCHED_EVIDENCE"
	OutOfStock       Code = "OUT_OF_STOCK"
	CatalogChanged   Code = "CATALOG_CHANGED"
)

func (c Code) Error() string { return string(c) }

func CodeOf(err error) Code {
	var code Code
	if errors.As(err, &code) {
		return code
	}
	return ""
}

type Rules struct {
	SchemaVersion         int                  `json:"schemaVersion"`
	MaxEnvelopeBytes      int64                `json:"maxEnvelopeBytes"`
	MaxEvidenceAgeSeconds int64                `json:"maxEvidenceAgeSeconds"`
	Operations            map[string]Operation `json:"operations"`
}

type Operation struct {
	Mode       string            `json:"mode"`
	Roles      []string          `json:"roles"`
	TTLSeconds int64             `json:"ttlSeconds"`
	Fields     map[string]string `json:"fields"`
	Response   string            `json:"response"`
}

var rules = mustRules()

func mustRules() Rules {
	var result Rules
	if err := json.Unmarshal([]byte(registryJSON), &result); err != nil {
		panic(err)
	}
	return result
}

type Request struct {
	Version        int               `json:"version"`
	Contract       string            `json:"contract"`
	Operation      string            `json:"operation"`
	RequestID      string            `json:"requestId"`
	Domain         string            `json:"domainRef"`
	Object         string            `json:"objectRef"`
	ExpiresAt      int64             `json:"expiresAt"`
	IdempotencyKey string            `json:"idempotencyKey,omitempty"`
	Body           map[string]string `json:"body"`
}

type Grant struct {
	Domain     string   `json:"domainRef"`
	Object     string   `json:"objectRef"`
	Operations []string `json:"operations"`
}

// Payment is populated by the trusted payment adapter after quote and authorization verification.
type Payment struct {
	QuoteDigest, TxDigest, Payer, FeePayer, Asset, MaxFee, Nonce string
	ValidUntil                                                   int64
}

// Checkpoint comes from the trusted chain profile and the current write lease.
type Checkpoint struct {
	ManifestDigest, ABIDigest, BlockHash, ParentHash, NextHeight, LogIndex, Fence string
}

// Provider is the verified issuer/audience or purpose grant for a C9 call.
type Provider struct {
	Issuer, Audience, AssertionDigest, Ref, Purpose, ConsentVersion string
	ValidUntil                                                      int64
	Active                                                          bool
}

type WalletApproval struct {
	Generation, ApprovalRef, PreimageDigest string
	ValidUntil                              int64
}

type OrderApproval struct {
	QuoteDigest, ApprovalDigest string
	ValidUntil                  int64
}

type Principal struct {
	Tenant     string `json:"tenantRef"`
	Subject    string `json:"subjectRef"`
	Role       string `json:"role"`
	ExpiresAt  int64  `json:"expiresAt"`
	Grants     []Grant
	Wallet     *WalletApproval `json:"-"`
	Order      *OrderApproval  `json:"-"`
	Payment    *Payment        `json:"-"`
	Checkpoint *Checkpoint     `json:"-"`
	Provider   *Provider       `json:"-"`
}

type Bound struct {
	Tenant, Subject, Role, RequestDigest, IntentDigest string
	Request                                            Request
	rule                                               Operation
}

var (
	uuidPattern    = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	codePattern    = regexp.MustCompile(`^[0-9a-f]{8}$`)
	digestPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	addressPattern = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
	decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
)

func valid(field, value string) bool {
	switch field {
	case "uuid":
		return uuidPattern.MatchString(value)
	case "code8":
		return codePattern.MatchString(value)
	case "digest":
		return digestPattern.MatchString(value)
	case "address":
		return addressPattern.MatchString(value) && value != "0x"+strings.Repeat("0", 40)
	}
	if strings.Contains(field, "|") {
		return slices.Contains(strings.Split(field, "|"), value)
	}
	if len(value) > 78 || !decimalPattern.MatchString(value) {
		return false
	}
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return false
	}
	switch field {
	case "uint32":
		return n.BitLen() <= 32
	case "uint64":
		return n.BitLen() <= 64
	case "uint256":
		return n.BitLen() <= 256
	case "positive256":
		return n.Sign() > 0 && n.BitLen() <= 256
	case "positive64":
		return n.Sign() > 0 && n.BitLen() <= 64
	case "time":
		return n.Sign() > 0 && n.IsInt64()
	default:
		return false
	}
}

// DecodeRequest strictly accepts one bounded JSON envelope with unique keys.
func DecodeRequest(input io.Reader) (Request, error) {
	raw, err := io.ReadAll(io.LimitReader(input, rules.MaxEnvelopeBytes+1))
	if err != nil || int64(len(raw)) > rules.MaxEnvelopeBytes {
		return Request{}, BadRequest
	}
	keys := json.NewDecoder(bytes.NewReader(raw))
	if !uniqueObject(keys, 0, "body") {
		return Request{}, BadRequest
	}
	if _, err := keys.Token(); err != io.EOF {
		return Request{}, BadRequest
	}
	var req Request
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return Request{}, BadRequest
	}
	// Go matches tagged fields case-insensitively. Exact envelope spelling is canonical.
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Request{}, BadRequest
	}
	for field := range envelope {
		if !slices.Contains([]string{"version", "contract", "operation", "requestId", "domainRef", "objectRef", "expiresAt", "idempotencyKey", "body"}, field) {
			return Request{}, BadRequest
		}
	}
	for _, required := range []string{"version", "contract", "operation", "requestId", "domainRef", "objectRef", "expiresAt", "body"} {
		if value, exists := envelope[required]; !exists || bytes.Equal(value, []byte("null")) {
			return Request{}, BadRequest
		}
	}
	if value, exists := envelope["idempotencyKey"]; exists && (bytes.Equal(value, []byte("null")) || req.IdempotencyKey == "") {
		return Request{}, BadRequest
	}
	return req, nil
}

func uniqueObject(decoder *json.Decoder, depth int, nestedKey string) bool {
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') || depth > 1 {
		return false
	}
	fields := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, stringKey := token.(string)
		if err != nil || !stringKey || fields[key] {
			return false
		}
		fields[key] = true
		if depth == 0 && key == nestedKey {
			if !uniqueObject(decoder, 1, nestedKey) {
				return false
			}
			continue
		}
		value, err := decoder.Token()
		if _, compound := value.(json.Delim); err != nil || compound || value == nil {
			return false
		}
	}
	end, err := decoder.Token()
	return err == nil && end == json.Delim('}')
}

func Authorize(req Request, principal Principal, now time.Time) (Bound, error) {
	if req.Version != rules.SchemaVersion {
		return Bound{}, Version
	}
	operation := req.Contract + "/" + req.Operation
	rule, ok := rules.Operations[operation]
	if !ok {
		return Bound{}, Capability
	}
	if !valid("uuid", req.RequestID) || !valid("uuid", req.Domain) || !valid("uuid", req.Object) || req.Body == nil ||
		(rule.Mode == "command" && !valid("uuid", req.IdempotencyKey)) || (rule.Mode == "read" && req.IdempotencyKey != "") ||
		len(req.Body) != len(rule.Fields) {
		return Bound{}, BadRequest
	}
	for field, kind := range rule.Fields {
		if !valid(kind, req.Body[field]) {
			return Bound{}, BadRequest
		}
	}
	if !valid("uuid", principal.Tenant) || !valid("uuid", principal.Subject) ||
		!slices.Contains(rule.Roles, principal.Role) || principal.ExpiresAt <= now.Unix() {
		return Bound{}, Forbidden
	}
	if !slices.ContainsFunc(principal.Grants, func(grant Grant) bool {
		return grant.Domain == req.Domain && grant.Object == req.Object && slices.Contains(grant.Operations, operation)
	}) {
		return Bound{}, Forbidden
	}
	if req.ExpiresAt <= now.Unix() || req.ExpiresAt > now.Unix()+rule.TTLSeconds || req.ExpiresAt > principal.ExpiresAt {
		return Bound{}, Expired
	}
	if err := checkAuthority(operation, req.Body, principal, now.Unix()); err != nil {
		return Bound{}, err
	}
	keys := make([]string, 0, len(req.Body))
	for key := range req.Body {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// Bind principal and intent apart from transport request ID/TTL so a retry can refresh those.
	values := []string{"nu54-intent-v2", operation, principal.Tenant, principal.Subject, req.Domain, req.Object, req.IdempotencyKey}
	for _, key := range keys {
		values = append(values, key, req.Body[key])
	}
	intent := digestFields(values...)
	req.Body = maps.Clone(req.Body)
	return Bound{Tenant: principal.Tenant, Subject: principal.Subject, Role: principal.Role, IntentDigest: intent,
		RequestDigest: digestFields("nu54-request-v2", intent, req.RequestID, strconv.FormatInt(req.ExpiresAt, 10)),
		Request:       req, rule: rule}, nil
}

func checkAuthority(operation string, body map[string]string, p Principal, now int64) error {
	switch operation {
	case "C3/payment.forward":
		a := p.Order
		if a == nil || a.ValidUntil <= now || a.QuoteDigest != body["quoteDigest"] || a.ApprovalDigest != body["approvalDigest"] {
			return Forbidden
		}
	case "C4/mpc.sign", "C4/mpc.status":
		a := p.Wallet
		if a == nil || a.ValidUntil <= now || a.Generation != body["generation"] ||
			(operation == "C4/mpc.sign" && (a.ApprovalRef != body["approvalRef"] || a.PreimageDigest != body["preimageDigest"])) {
			return Forbidden
		}
	case "C5/transaction.submit":
		a := p.Payment
		if a == nil || a.QuoteDigest != body["quoteDigest"] || a.TxDigest != body["txDigest"] ||
			a.Payer != body["payer"] || a.FeePayer != body["feePayer"] || a.Asset != body["asset"] || a.Nonce != body["nonce"] ||
			!valid("positive256", a.MaxFee) {
			return Forbidden
		}
		direct := body["mode"] == "direct"
		if (direct && (p.Role != "payer" || a.Payer != a.FeePayer)) || (!direct && (p.Role != "sponsor" || a.Payer == a.FeePayer)) {
			return Forbidden
		}
		fee, _ := new(big.Int).SetString(body["feeLimit"], 10)
		maximum, _ := new(big.Int).SetString(a.MaxFee, 10)
		if fee.Cmp(maximum) > 0 {
			return Forbidden
		}
		deadline, _ := strconv.ParseInt(body["txExpiresAt"], 10, 64)
		if deadline <= now || deadline > a.ValidUntil {
			return Expired
		}
	case "C6/chain.append":
		a := p.Checkpoint
		if a == nil || a.ManifestDigest != body["manifestDigest"] || a.ABIDigest != body["abiDigest"] ||
			a.BlockHash != body["blockHash"] || a.ParentHash != body["parentHash"] || a.NextHeight != body["height"] ||
			a.LogIndex != body["logIndex"] || a.Fence != body["fence"] {
			return Mismatch
		}
	case "C9/oidc.bind":
		a := p.Provider
		if a == nil || a.ValidUntil <= now || a.Issuer != body["issuerRef"] || a.Audience != body["audienceRef"] ||
			a.AssertionDigest != body["assertionDigest"] {
			return Forbidden
		}
	case "C9/task.invoke", "C9/task.status", "C9/task.revoke", "C9/task.delete":
		a := p.Provider
		if a == nil || a.ValidUntil <= now || a.Ref != body["providerRef"] || a.Purpose != body["purpose"] ||
			a.ConsentVersion != body["consentVersion"] || (operation == "C9/task.invoke" && !a.Active) {
			return Forbidden
		}
	}
	return nil
}

func digestFields(fields ...string) string {
	hash := sha256.New()
	for _, field := range fields {
		fmt.Fprintf(hash, "%d:%s", len(field), field)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// Reconcile demands an exact prior request binding on unknown-result recovery.
// Owners perform this check against the stored receipt before any side effect.
func Reconcile(bound Bound, receipt, originalDigest string) error {
	if bound.rule.Response != "resume" || receipt != bound.Request.Body["receiptRef"] ||
		originalDigest != bound.Request.Body["priorRequestDigest"] {
		return Mismatch
	}
	return nil
}

// MatchIntent enforces an existing owner receipt's immutable command digest.
func MatchIntent(command Bound, existingDigest string) error {
	if command.rule.Mode != "command" || !valid("digest", existingDigest) || command.IntentDigest != existingDigest {
		return Conflict
	}
	return nil
}
