package settlement

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/test/settlement/w10fixture"
	"github.com/ethereum/go-ethereum/common"
)

type fixtureRentalReviewEscrow struct{ manifest []byte }

func (f fixtureRentalReviewEscrow) Deployment(context.Context, string) ([]byte, bool) {
	return f.manifest, true
}

func (f fixtureRentalReviewEscrow) Check(context.Context, string, common.Address, uint64, *big.Int) ([]byte, time.Time, bool) {
	return f.manifest, time.Now().Add(15 * time.Second), true
}

func TestPostgresCustomerRentalReviewProof(t *testing.T) {
	testPostgresCustomerRentalReview(t, false)
}

func TestPostgresCustomerRentalDecisionForward(t *testing.T) {
	testPostgresCustomerRentalReview(t, true)
}

func testPostgresCustomerRentalReview(t *testing.T, exerciseForward bool) {
	t.Helper()
	db, dsn := scratchDB(t)
	ctx := t.Context()
	buyer := fixtureSigner(102)
	if exerciseForward {
		buyer = fixtureSigner(105)
	}
	target := Target{Stream: fixtureUUID(t, db), Asset: common.HexToAddress("0x1111"), Contract: common.HexToAddress("0x2222")}
	now := time.Now().UTC().Truncate(time.Second)
	cafe := seedPayerPreview(t, db, target, "6500000", buyer)
	fixtureExec(t, db, "UPDATE ledger.sale SET origin='menu' WHERE id=$1", cafe.Sale)
	cafe = seedPayerIssue(t, db, cafe, now.Add(-time.Second), "6500000", 98, 2, 120*time.Second)
	manager, device, other := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	fixtureExec(t, db, "UPDATE ledger.sale SET revision=1 WHERE id=$1", cafe.Sale)
	fixtureExec(t, db, `INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,expires_at,intent_digest)
		VALUES($1,$2,$3,$4,$5)`, cafe.Sale, cafe.Branch, cafe.Actor, cafe.Order.Expires, cafe.Order.Intent[:])
	fixtureExec(t, db, `INSERT INTO ledger.menu_release VALUES($1,1,now()-interval '1 minute',now()+interval '15 minutes')`, cafe.Branch)
	fixtureExec(t, db, `INSERT INTO ledger.price VALUES($1,1,'cafe','plain','Cafe',$2,6500000)`, cafe.Branch, lower(target.Asset))
	fixtureExec(t, db, `INSERT INTO ledger.stock VALUES($1,'cafe',1)`, cafe.Branch)
	fixtureExec(t, db, `INSERT INTO ledger.order_line VALUES($1,$2,1,'cafe','plain',1,6500000)`, cafe.Sale, cafe.Branch)
	fixtureExec(t, db, `INSERT INTO ledger.payer_enrollment VALUES($1,$2,$3,1,2,$4)`, cafe.Branch, cafe.Actor, buyer.Address().Bytes(), cafe.Order.Expires)
	fixtureExec(t, db, `INSERT INTO ledger.checkout_claim VALUES($1,$2,$3,$4,$5,1,2,$6)`,
		cafe.Sale, cafe.Branch, cafe.Actor, cafe.Order.Claim[:], buyer.Address().Bytes(), cafe.Order.Expires)
	fixtureExec(t, db, `INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until)
		VALUES($1,$2,'customer',now()+interval '5 minutes')`, cafe.Branch, other)
	fixtureExec(t, db, "INSERT INTO ledger.login_subject VALUES($1,$2)", "review_fixture_"+strings.ReplaceAll(manager, "-", ""), manager)
	fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'rental_manage',now()+interval '5 minutes')", cafe.Branch, manager)
	fixtureExec(t, db, `INSERT INTO ledger.rental_enrollment(branch_id,device_ref,generation,account_ref,withdraw_ref,revision,
		enrollment_digest,attested_until,attested_at) VALUES($1,$2,1,$3,$4,'review-fixture',sha256(convert_to($3,'UTF8')),
		now()+interval '5 minutes',now()-interval '1 minute')`, cafe.Branch, device, lower(buyer.Address()), lower(fixtureSigner(103).Address()))
	manifest := bytes.Repeat([]byte{42}, 32)
	var handover string
	if err := db.QueryRow(ctx, `SELECT ledger.offer_renter_handover($1,$2,$3,$4,1,$5,$6,
		now()+interval '2 minutes',$7)::text`,
		cafe.Tenant, cafe.Branch, manager, device, cafe.Actor, lower(buyer.Address()), manifest).Scan(&handover); err != nil {
		t.Fatal(err)
	}
	fixtureExec(t, db, "SELECT ledger.change_renter_return($1,$2,$3,'accept',$4)", cafe.Tenant, cafe.Actor, handover, lower(buyer.Address()))

	profile := commerce.Profile{Chain: 8283, Contract: target.Contract, Asset: target.Asset, Beneficiary: fixtureSigner(104).Address(),
		Signer: fixtureSigner(101).Address(), MerchantID: [32]byte{31: 50}, KeyVersion: 1, BeneficiaryVersion: 1, CommitmentKey: [32]byte{31: 9}}
	_, commerceDSN := testLogin(t, db, dsn, "nu54_p05_commerce")
	store, err := commerce.Open(ctx, commerceDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	quoteCipher, err := commerce.NewCipher(cafe.CipherKey[:])
	if err != nil {
		t.Fatal(err)
	}
	x, y := elliptic.P256().ScalarBaseMult([]byte{3})
	pk := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	spki, err := x509.MarshalPKIXPublicKey(pk)
	if err != nil {
		t.Fatal(err)
	}
	commissioned := commerce.RentalReviewCommissioning{Signer: [65]byte(elliptic.Marshal(pk.Curve, pk.X, pk.Y)),
		BoardPin: sha256.Sum256(spki), TerminalPin: [32]byte{17}, ExpiresAt: now.Add(time.Hour)}
	var activeBoard atomic.Pointer[w10fixture.RenterBoard]
	var refusalBoard, approvalBoard *w10fixture.RenterBoard
	if bridge, binary := os.Getenv("NU54_W10_RENTER_BOARD_BRIDGE"), os.Getenv("NU54_W10_RENTER_BOARD_BIN"); exerciseForward && (bridge != "" || binary != "") {
		payer := append(make([]byte, 31), byte(buyer))
		refusalBoard, err = w10fixture.NewRenterBoard(t.TempDir(), bridge, binary, payer)
		if err != nil {
			t.Fatal(err)
		}
		approvalBoard, err = w10fixture.NewRenterBoard(t.TempDir(), bridge, binary, payer)
		if err != nil {
			t.Fatal(err)
		}
		activeBoard.Store(refusalBoard)
		commissioned = refusalBoard.Commissioning
	}
	var configured atomic.Bool
	var sponsorGrant atomic.Uint32
	sponsorUntil := time.Now().Add(10 * time.Second).Unix()
	trust, certs, _ := mtlsClients(t, 2)
	principals := map[string]service.Principal{}
	for index, actor := range []string{cafe.Actor, other} {
		principals[fingerprint(certs[index])] = service.Principal{Role: "customer", Tenant: cafe.Tenant, Subject: actor, ExpiresAt: now.Add(time.Hour).Unix()}
	}
	mux := http.NewServeMux()
	escrow := fixtureRentalReviewEscrow{manifest: manifest}
	sponsor := func(_ context.Context, _ string, _ commerce.Profile, _ common.Address, _ time.Time) (commerce.GasSponsorship, bool) {
		original := sponsorGrant.Load()
		return commerce.GasSponsorship{
			Account: fixtureSigner(110).Address(), Grant: [32]byte{byte(original)}, ExpiresAt: sponsorUntil,
			CurrentPublication: func(at time.Time) bool { return sponsorGrant.Load() == original && at.Unix() < sponsorUntil },
		}, original != 0
	}
	var forwarder commerce.PaymentForwarder
	if exerciseForward {
		settlementStore, err := Open(ctx, commerceDSN, "nu54_p05_commerce")
		if err != nil {
			t.Fatal(err)
		}
		defer settlementStore.Close()
		relayCipher, err := commerce.NewCipher(bytes.Repeat([]byte{7}, 32))
		if err != nil {
			t.Fatal(err)
		}
		forwarder = Forwarder{Store: settlementStore, QuoteCipher: quoteCipher, RelayCipher: relayCipher,
			GasSponsors: sponsor, RentalEscrow: escrow, Targets: map[string]Target{cafe.Branch: target}}
	}
	commerce.HTTP{
		Store: store, Profiles: map[string]commerce.Profile{cafe.Branch: profile}, Credentials: principals,
		Signer: fixtureSigner(101), Cipher: quoteCipher, PresentationKey: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, 32)),
		RentalEscrow: escrow, GasSponsors: sponsor, Payment: forwarder,
		RentalReview: func(_ context.Context, branch string, given commerce.Profile, deployed []byte, payer common.Address, generation uint64, amount *big.Int) (commerce.RentalReviewCommissioning, bool) {
			identity := commissioned
			if board := activeBoard.Load(); board != nil {
				identity = board.Commissioning
			}
			return identity, configured.Load() && branch == cafe.Branch && given.Contract == profile.Contract &&
				bytes.Equal(deployed, manifest) && payer == buyer.Address() && generation == 1 && amount.Int64() == 6500000
		},
	}.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	server.StartTLS()
	defer server.Close()
	key := fixtureUUID(t, db)
	call := func(t *testing.T, identity int, data any, want int) *http.Response {
		t.Helper()
		raw, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		transport := server.Client().Transport.(*http.Transport).Clone()
		transport.TLSClientConfig.Certificates = []tls.Certificate{certs[identity]}
		t.Cleanup(transport.CloseIdleConnections)
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		res, err := client.Post(server.URL+"/v2/commerce", "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { res.Body.Close() })
		if res.StatusCode != want {
			var reply service.Reply
			_ = json.NewDecoder(res.Body).Decode(&reply)
			t.Fatalf("commerce response = HTTP %d/%s, want %d", res.StatusCode, reply.Error, want)
		}
		return res
	}
	request := func(identity int, challenge *commerce.RentalReviewChallenge, want int) *commerce.Presentation {
		t.Helper()
		command := service.Request{Version: 2, Contract: "C3", Operation: "quote.issue", Domain: cafe.Branch, Object: cafe.Sale,
			RequestID: fixtureUUID(t, db), IdempotencyKey: key, ExpiresAt: time.Now().Add(25 * time.Second).Unix(),
			Body: map[string]string{"claimDigest": rawHex(cafe.Order.Claim[:]), "orderRevision": "0"}}
		res := call(t, identity, struct {
			Command   service.Request                 `json:"command"`
			Challenge *commerce.RentalReviewChallenge `json:"rentalReview,omitempty"`
		}{command, challenge}, want)
		defer res.Body.Close()
		if want != http.StatusOK {
			return nil
		}
		var out struct {
			service.Reply
			Quote struct {
				Signed       string                 `json:"signedQuote"`
				Presentation *commerce.Presentation `json:"presentation"`
			} `json:"quote"`
		}
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil || out.ResultRef != cafe.Sale || out.Quote.Signed != rawHex(cafe.Signed) ||
			out.Quote.Presentation == nil || out.Quote.Presentation.Customer != cafe.Actor {
			t.Fatalf("quote response lacked the authenticated original: %v", err)
		}
		return out.Quote.Presentation
	}
	challenge := commerce.RentalReviewChallenge{Request: strings.Repeat("a", 32), Challenge: strings.Repeat("1", 64)}
	if plain := request(0, nil, 200); plain.RentalReview != nil || plain.RentalEscrow == nil {
		t.Fatal("legacy quote changed or lacked the separate funding proof")
	}
	if unavailable := request(0, &challenge, 200); unavailable.RentalReview != nil || unavailable.RentalEscrow == nil {
		t.Fatal("funding proof alone authorized a board review")
	}
	configured.Store(true)
	if held := request(0, &challenge, 200); held.RentalReview != nil || held.RentalEscrow == nil {
		t.Fatal("board commissioning without a committed sponsor authorized review")
	}
	sponsorGrant.Store(1)
	proof := request(0, &challenge, 200).RentalReview
	if proof == nil || proof.RentalReviewChallenge != challenge || proof.BoardPin != rawHex(commissioned.BoardPin[:]) || proof.ExpiresAt != sponsorUntil {
		t.Fatal("current original did not receive its commissioned board proof")
	}
	request(1, &challenge, 404)
	for _, grant := range []uint32{0, 2} {
		sponsorGrant.Store(grant)
		if held := request(0, &challenge, 200); held.RentalReview != nil || held.RentalEscrow == nil {
			t.Fatal("withdrawn or replaced sponsorship authorized another board review")
		}
	}
	sponsorGrant.Store(1)
	retry := challenge
	retry.Challenge = strings.Repeat("2", 64)
	updated := request(0, &retry, 200).RentalReview
	if updated == nil || updated.RentalReviewChallenge != retry || updated.Signature == proof.Signature {
		t.Fatal("existing quote did not bind the new original phone challenge")
	}
	if exerciseForward {
		forwardKey := fixtureUUID(t, db)
		signed, err := buyer.SignDigest(ctx, cafe.Quote.ApprovalHash)
		if err != nil {
			t.Fatal(err)
		}
		forward := func(t *testing.T, identity, want int, signature string) service.Reply {
			t.Helper()
			command := service.Request{Version: 2, Contract: "C3", Operation: "payment.forward", Domain: cafe.Branch, Object: cafe.Sale,
				RequestID: fixtureUUID(t, db), IdempotencyKey: forwardKey, ExpiresAt: time.Now().Add(25 * time.Second).Unix(),
				Body: map[string]string{"quoteDigest": rawHex(cafe.Quote.Digest[:]), "approvalDigest": rawHex(cafe.Quote.ApprovalHash[:])}}
			res := call(t, identity, struct {
				Command   service.Request `json:"command"`
				Signature string          `json:"approvalSignature,omitempty"`
			}{command, signature}, want)
			var reply service.Reply
			if want != http.StatusOK {
				return reply
			}
			if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
				t.Fatal(err)
			}
			return reply
		}
		count := func(t *testing.T, want int) {
			t.Helper()
			var receipts, submissions, payments int
			err := db.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM ledger.command_receipt
				 WHERE result_ref=$1 AND operation='payment_forward' AND request_key=$2 AND scope_ref=$3),
				(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1),
				(SELECT count(*) FROM ledger.payment WHERE sale_id=$1)`, cafe.Sale, forwardKey, cafe.Actor).
				Scan(&receipts, &submissions, &payments)
			if err != nil || receipts != want || submissions != want || payments != 0 {
				t.Fatalf("unexpected receipt/submission/payment counts: %d/%d/%d (%v)", receipts, submissions, payments, err)
			}
		}
		t.Run("refusal_has_no_payer_approval", func(t *testing.T) {
			if refusalBoard != nil {
				refused, err := refusalBoard.Decide(ctx, cafe.Signed, lower(profile.Signer), cafe.Quote.ApprovalHash,
					cafe.Quote.Digest, cafe.Order.Expires, updated, false)
				if err != nil || len(refused) != 0 {
					t.Fatalf("independent C2 refusal: %v", err)
				}
			}
			forward(t, 0, http.StatusBadRequest, "")
			count(t, 0)
		})
		t.Run("signed_approval_admits_once_and_reopens_original", func(t *testing.T) {
			if approvalBoard != nil {
				activeBoard.Store(approvalBoard)
				next := challenge
				next.Challenge = strings.Repeat("3", 64)
				authority := request(0, &next, http.StatusOK).RentalReview
				if authority == nil {
					t.Fatal("no committed P05 authority for the C2 board")
				}
				approved, err := approvalBoard.Decide(ctx, cafe.Signed, lower(profile.Signer), cafe.Quote.ApprovalHash,
					cafe.Quote.Digest, cafe.Order.Expires, authority, true)
				if err != nil || len(approved) != 65 {
					t.Fatalf("independent C2 approval: %v", err)
				}
				signed = approved
			}
			forward(t, 1, http.StatusNotFound, rawHex(signed))
			count(t, 0)
			first := forward(t, 0, http.StatusOK, rawHex(signed))
			if first.ResultRef != cafe.Sale {
				t.Fatal("payer approval did not admit original sale")
			}
			count(t, 1)
			sponsorGrant.Store(0)
			fixtureExec(t, db, "SELECT ledger.change_renter_return($1,$2,$3,'revoke','')", cafe.Tenant, manager, handover)
			retry := forward(t, 0, http.StatusOK, rawHex(signed))
			if retry.ResultRef != first.ResultRef {
				t.Fatal("same-key recovery did not resolve the admitted original")
			}
			status := call(t, 0, struct {
				Command service.Request `json:"command"`
			}{service.Request{Version: 2, Contract: "C3", Operation: "order.status", Domain: cafe.Branch, Object: cafe.Sale,
				RequestID: fixtureUUID(t, db), ExpiresAt: time.Now().Add(25 * time.Second).Unix(), Body: map[string]string{}}}, http.StatusOK)
			var receipt struct {
				Order *commerce.Order `json:"order"`
			}
			if json.NewDecoder(status.Body).Decode(&receipt) != nil || receipt.Order == nil ||
				receipt.Order.ID != cafe.Sale || receipt.Order.State != "checking" ||
				receipt.Order.Payment == nil || receipt.Order.Payment.Stage != "received" {
				t.Fatal("customer could not read the same admitted original")
			}
			count(t, 1)
		})
		return
	}
	fixtureExec(t, db, "SELECT ledger.change_renter_return($1,$2,$3,'revoke','')", cafe.Tenant, manager, handover)
	if held := request(0, &retry, 200); held.RentalReview != nil || held.RentalEscrow != nil {
		t.Fatal("withdrawn handover released a new review or funding proof")
	}
	var receipts, payments int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.command_receipt WHERE result_ref=$1 AND operation='issue_quote'),
		(SELECT count(*) FROM ledger.settlement_submission WHERE sale_id=$1)`, cafe.Sale).Scan(&receipts, &payments); err != nil ||
		receipts != 1 || payments != 0 {
		t.Fatalf("review challenge created additional durable effects: receipts %d, payments %d, error %v", receipts, payments, err)
	}
}
