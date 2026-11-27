package service

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	sampleID     = "23a6f801-e925-42b9-b4fa-05f5d4f11cee"
	sampleDomain = "987e50a6-a874-487c-8d85-1451edacfe99"
	sampleObject = "03f43389-3f6b-4bc4-a1d7-6cb11af65ccb"
	sampleTenant = "80d39491-fa9b-4f35-9c2a-f6a258dd45d6"
	sampleActor  = "0fb9757b-d626-4fb8-b02e-68a9fc8930f0"
)

func TestReadCredentialScopeExpiryAndNoPaymentKey(t *testing.T) {
	now := time.Unix(1600, 0)
	req := Request{Version: 2, Contract: "C7", Operation: "evidence.read", RequestID: sampleID,
		Domain: sampleDomain, Object: sampleObject, ExpiresAt: 1630, Body: map[string]string{"kind": "finalized"}}
	auth := Principal{Role: "evidence-reader", Tenant: sampleTenant, Subject: sampleActor, ExpiresAt: 1800,
		Grants: []Grant{{Domain: sampleDomain, Object: sampleObject, Operations: []string{"C7/evidence.read"}}}}
	bound, err := Authorize(req, auth, now)
	if err != nil || bound.Tenant != sampleTenant || bound.RequestDigest == "" {
		t.Fatalf("read did not bind to credentials: %v", err)
	}
	req.IdempotencyKey = sampleID
	if _, err := Authorize(req, auth, now); CodeOf(err) != BadRequest {
		t.Fatalf("read accepted payment idempotency key: %v", err)
	}
	req.IdempotencyKey = ""
	req.Object = "3b9d114a-9736-4fe8-9506-74d5146ca587"
	if _, err := Authorize(req, auth, now); CodeOf(err) != Forbidden {
		t.Fatalf("cross-object request: %v", err)
	}
	req.Object = sampleObject
	if _, err := Authorize(req, auth, time.Unix(1630, 0)); CodeOf(err) != Expired {
		t.Fatalf("expired request: %v", err)
	}
}

func TestSharedServiceFixturesAndRejections(t *testing.T) {
	var fixtures []struct {
		ID            string
		Request       Request
		Scope         Principal
		At            int64
		IntentDigest  string
		RequestDigest string
		Wallet        *WalletApproval
		Payment       *Payment
		Checkpoint    *Checkpoint
		Provider      *Provider
		Reply         *Reply
	}
	body, err := os.ReadFile("../../v2/service-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, item := range fixtures {
		t.Run(item.ID, func(t *testing.T) {
			now := time.Unix(item.At, 0)
			item.Scope.Wallet, item.Scope.Payment = item.Wallet, item.Payment
			item.Scope.Checkpoint, item.Scope.Provider = item.Checkpoint, item.Provider
			got, err := Authorize(item.Request, item.Scope, now)
			if err != nil || got.IntentDigest != item.IntentDigest || got.RequestDigest != item.RequestDigest {
				t.Fatalf("binding failed: %+v %v", got, err)
			}
			if item.Request.IdempotencyKey != "" {
				next := item.Request
				next.RequestID = sampleObject
				next.ExpiresAt++
				retry, err := Authorize(next, item.Scope, now)
				if err != nil || retry.RequestDigest == got.RequestDigest || MatchIntent(retry, got.IntentDigest) != nil {
					t.Fatalf("retry failed: %v", err)
				}
				if MatchIntent(retry, strings.Repeat("0", 64)) != Conflict {
					t.Fatal("changed command reused")
				}
			}
			if item.Reply != nil {
				if err := ValidateReply(got, *item.Reply, now); err != nil {
					t.Fatal(err)
				}
				item.Reply.Evidence.Generation = sampleObject
				if err := ValidateReply(got, *item.Reply, now); err != Mismatch {
					t.Fatal("wrong generation accepted")
				}
			}
			switch item.ID {
			case "C5/transaction.submit":
				item.Request.Body["nonce"] = "6"
				if _, err := Authorize(item.Request, item.Scope, now); err != Forbidden {
					t.Fatal("wrong nonce")
				}
			case "C5/transaction.submit/sponsored":
				item.Request.Body["feeLimit"] = "501"
				if _, err := Authorize(item.Request, item.Scope, now); err != Forbidden {
					t.Fatal("over sponsor fee budget")
				}
			case "C5/transaction.status":
				if err := Reconcile(got, item.Request.Body["receiptRef"], item.Request.Body["priorRequestDigest"]); err != nil {
					t.Fatal(err)
				}
				if Reconcile(got, sampleID, item.Request.Body["priorRequestDigest"]) != Mismatch {
					t.Fatal("another receipt accepted")
				}
			case "C6/chain.append":
				item.Request.Body["parentHash"] = strings.Repeat("c", 64)
				if _, err := Authorize(item.Request, item.Scope, now); err != Mismatch {
					t.Fatal("unrelated chain header")
				}
			case "C9/task.invoke":
				item.Scope.Provider.Active = false
				if _, err := Authorize(item.Request, item.Scope, now); err != Forbidden {
					t.Fatal("revoked consent")
				}
			}
		})
	}
}

func TestDecodeRejectsDisguisedTenantAndDuplicateKeys(t *testing.T) {
	for _, payload := range []string{
		`{"version":2,"Version":2}`,
		`{"version":2,"version":2}`,
		`{"body":{"scope":"valid","scope":"override"}}`,
		`{"body":{"tenantRef":"override"},"tenantRef":"override"}`,
		`{"body":{"forbidden":[]}}`,
		strings.Repeat(" ", 4097),
	} {
		if _, err := DecodeRequest(strings.NewReader(payload)); err != BadRequest {
			t.Fatalf("ambiguous envelope accepted: %q", payload)
		}
	}
}
