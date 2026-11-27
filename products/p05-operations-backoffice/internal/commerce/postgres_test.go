package commerce

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
)

const (
	tenant     = "80d39491-fa9b-4f35-9c2a-f6a258dd45d6"
	branch     = "987e50a6-a874-487c-8d85-1451edacfe99"
	other      = "23a6f801-e925-42b9-b4fa-05f5d4f11cee"
	customerA  = "0fb9757b-d626-4fb8-b02e-68a9fc8930f0"
	customerB  = "03f43389-3f6b-4bc4-a1d7-6cb11af65ccb"
	terminal   = "9bbe99f1-37bf-4a65-b13e-d95c5c9a9652"
	posOrigin  = "a190ade3-78ac-4a23-b893-d979e0f395bb"
	posOrigin2 = "abaf3a81-ee3c-4c4b-9a56-8d299121949e"
)

type actor struct{ id, role string }

func authorize(t *testing.T, who actor, operation, domain, obj, key string, fields map[string]string) service.Bound {
	t.Helper()
	now := time.Now()
	req := service.Request{Version: 2, Contract: "C3", Operation: operation, RequestID: other,
		Domain: domain, Object: obj, ExpiresAt: now.Unix() + 45, IdempotencyKey: key, Body: fields}
	principal := service.Principal{Tenant: tenant, Subject: who.id, Role: who.role, ExpiresAt: now.Unix() + 60,
		Grants: []service.Grant{{Domain: domain, Object: obj, Operations: []string{"C3/" + operation}}}}
	bound, err := service.Authorize(req, principal, now)
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

type barrierSigner struct {
	key     *ecdsa.PrivateKey
	start   chan struct{}
	release chan struct{}
	count   atomic.Int32
}

func (s *barrierSigner) SignDigest(ctx context.Context, digest [32]byte) ([]byte, error) {
	if s.count.Add(1) == 1 {
		close(s.start)
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	raw, err := crypto.Sign(digest[:], s.key)
	if err == nil {
		raw[64] += 27
	}
	return raw, err
}

func TestPostgresLastStockAndSinglePayerWithRestart(t *testing.T) {
	dsn := os.Getenv("P05_COMMERCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires a migrated disposable PostgreSQL database")
	}
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(adminConfig.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(adminConfig.Host, "/private/tmp/") || strings.HasPrefix(adminConfig.Host, "/tmp/")) {
		t.Fatal("use a local nu54_commerce_* disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	role := "nu54_commerce_fixture_" + strings.TrimPrefix(adminConfig.Database, "nu54_commerce_")
	if strings.ContainsAny(role, " -;/") {
		t.Fatal("unsafe scratch database name")
	}
	exec("CREATE ROLE " + pgx.Identifier{role}.Sanitize() + " LOGIN IN ROLE nu54_p05_commerce")
	defer admin.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize())
	runtimeConfig := adminConfig.Copy()
	runtimeConfig.User = role
	store, err := Open(ctx, runtimeConfig.ConnString()+" user="+role)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(ctx, "SELECT id FROM ledger.sale"); err == nil {
		t.Fatal("runtime role sees base sale rows")
	}
	if _, err := Open(ctx, dsn); err == nil {
		t.Fatal("administrator connection passed least-privilege gate")
	}

	a := actor{customerA, "customer"}
	b := actor{customerB, "customer"}
	pos := actor{terminal, "terminal"}
	profile := FixtureProfile()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	profile.Signer = crypto.PubkeyToAddress(key.PublicKey)
	enrolled := profile.Digest()
	asset := strings.ToLower(profile.Asset.Hex())
	exec("INSERT INTO ledger.branch VALUES ($1,$3,'Asia/Seoul','04:00'),($2,$3,'Asia/Seoul','04:00')", branch, other, tenant)
	exec("INSERT INTO ledger.command_rule SELECT id,'synthetic order','synthetic basis','expire fixture',interval '2 minutes' FROM ledger.branch WHERE id IN ($1,$2)", branch, other)
	exec("INSERT INTO ledger.commerce_grant VALUES ($1,$2,'customer',NULL,now()+interval '2 minutes'),($1,$3,'customer',NULL,now()+interval '2 minutes'),($1,$4,'terminal','pos',now()+interval '2 minutes')",
		branch, a.id, b.id, pos.id)
	exec("INSERT INTO ledger.menu_release VALUES ($1,1,now()-interval '1 minute',now()+interval '2 minutes')", branch)
	exec("INSERT INTO ledger.price VALUES ($1,1,'last','plain','Last', $2,7),($1,1,'pos','plain','POS', $2,19),($1,1,'pos','iced','POS iced', $2,23)", branch, asset)
	exec("INSERT INTO ledger.stock VALUES ($1,'last',1),($1,'pos',4)", branch)
	exec("INSERT INTO ledger.catalog_option_stock VALUES ($1,'pos','plain',2),($1,'pos','iced',2)", branch)
	exec("INSERT INTO ledger.issuance_profile VALUES ($1,1,$2,60,true)", branch, enrolled[:])

	stockStart := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, user := range []actor{a, b} {
		bound := authorize(t, user, "order.create", branch, user.id, user.id,
			map[string]string{"asset": asset, "quantity": "1", "intentDigest": strings.Repeat("a", 64)})
		wg.Go(func() {
			<-stockStart
			_, err := store.Reserve(ctx, bound, Cart{Origin: "menu", OriginRef: user.id, MenuVersion: 1, Items: []Item{{"last", "plain", 1}}})
			results <- err
		})
	}
	close(stockStart)
	wg.Wait()
	close(results)
	var succeeded, stockDenied int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrStock):
			stockDenied++
		default:
			t.Fatalf("unexpected last item result: %v", err)
		}
	}
	if succeeded != 1 || stockDenied != 1 {
		t.Fatalf("last item sold %d times, %d refusal(s)", succeeded, stockDenied)
	}

	cart := Cart{Origin: "pos", OriginRef: posOrigin, MenuVersion: 1, Items: []Item{{"pos", "plain", 1}, {"pos", "iced", 1}}}
	fields := map[string]string{"asset": asset, "quantity": "2", "intentDigest": strings.Repeat("a", 64)}
	reserve := authorize(t, pos, "order.create", branch, posOrigin, posOrigin, fields)
	exec("UPDATE ledger.catalog_option_stock SET available=0 WHERE branch_id=$1 AND sku='pos' AND option_key='plain'", branch)
	if _, err := store.Reserve(ctx, reserve, cart); !errors.Is(err, ErrStock) {
		t.Fatalf("option shortage did not reject despite available SKU stock: %v", err)
	}
	var remainingSKU, earlierOption, partialWrites int
	if err := admin.QueryRow(ctx, `SELECT s.available,o.available,
		(SELECT count(*) FROM ledger.sale WHERE origin_ref=$2)
		FROM ledger.stock s JOIN ledger.catalog_option_stock o USING(branch_id,sku)
		WHERE s.branch_id=$1 AND s.sku='pos' AND o.option_key='iced'`, branch, posOrigin).
		Scan(&remainingSKU, &earlierOption, &partialWrites); err != nil || remainingSKU != 4 || earlierOption != 2 || partialWrites != 0 {
		t.Fatalf("option shortage left partial SKU/option/sale writes: %d %d %d %v", remainingSKU, earlierOption, partialWrites, err)
	}
	exec("UPDATE ledger.catalog_option_stock SET available=2 WHERE branch_id=$1 AND sku='pos' AND option_key='plain'", branch)
	sale, err := store.Reserve(ctx, reserve, cart)
	if err != nil {
		t.Fatal(err)
	}
	// A terminal mode switch may deny a new cart, but a lost reserve receipt remains recoverable by the same actor.
	exec("UPDATE ledger.commerce_grant SET terminal_mode='menu',mode_revision=mode_revision+1 WHERE branch_id=$1 AND actor_id=$2", branch, pos.id)
	if replay, err := store.Reserve(ctx, reserve, cart); err != nil || replay != sale {
		t.Fatalf("mode change lost old receipt: %s %v", replay, err)
	}
	if _, err := store.Reserve(ctx, authorize(t, pos, "order.create", branch, posOrigin, posOrigin2, fields), cart); !errors.Is(err, service.Forbidden) {
		t.Fatalf("new old-mode reserve accepted: %v", err)
	}
	exec("UPDATE ledger.commerce_grant SET terminal_mode='pos',mode_revision=mode_revision+1 WHERE branch_id=$1 AND actor_id=$2", branch, pos.id)
	second := authorize(t, pos, "order.create", branch, posOrigin, posOrigin2, fields)
	reverse := cart
	reverse.Items = []Item{cart.Items[1], cart.Items[0]}
	if duplicate, err := store.Reserve(ctx, second, reverse); err != nil || duplicate != sale {
		t.Fatalf("POS same order returned %s: %v", duplicate, err)
	}
	changed := cart
	changed.Items = []Item{{"pos", "plain", 2}}
	if _, err := store.Reserve(ctx, second, changed); err != service.Conflict {
		t.Fatalf("changed cart for stored second key: %v", err)
	}

	exec(`INSERT INTO ledger.payer_enrollment VALUES ($1,$2,$4,1,1,now()+interval '1 minute'),
		($1,$3,$5,1,1,now()+interval '1 minute')`, branch, a.id, b.id,
		common.HexToAddress("0x4001").Bytes(), common.HexToAddress("0x4002").Bytes())
	claimPath := func(payer actor, sale string, requestKey string) Claim {
		t.Helper()
		authorized := authorize(t, pos, "checkout.authorize", branch, sale, requestKey, map[string]string{"customerRef": payer.id})
		claim, err := store.AuthorizeCheckout(ctx, authorized)
		if err != nil {
			t.Fatal(err)
		}
		retry, err := store.AuthorizeCheckout(ctx, authorized)
		if err != nil || claim != retry {
			t.Fatalf("checkout claim retry changed: %v", err)
		}
		return claim
	}
	issuedForA := claimPath(a, sale, customerA)
	issuedForB := claimPath(b, sale, customerB)
	if _, err := store.Status(ctx, authorize(t, a, "order.status", other, sale, "", map[string]string{})); err != service.Forbidden {
		t.Fatalf("foreign branch status exposed: %v", err)
	}
	requestA := authorize(t, a, "quote.issue", branch, sale, sale,
		map[string]string{"claimDigest": issuedForA.Digest, "orderRevision": "0"})
	requestB := authorize(t, b, "quote.issue", branch, sale, sale,
		map[string]string{"claimDigest": issuedForB.Digest, "orderRevision": "0"})
	terminalRequest := authorize(t, pos, "quote.issue", branch, sale, sale, requestA.Request.Body)
	wrongClaim := authorize(t, pos, "quote.issue", branch, sale, other, map[string]string{
		"claimDigest": strings.Repeat("a", 64), "orderRevision": "0"})
	sign := &barrierSigner{key: key, start: make(chan struct{}), release: make(chan struct{})}
	secret := [32]byte{7}
	cipher, err := NewCipher(secret[:])
	if err != nil {
		t.Fatal(err)
	}
	h := HTTP{Store: store, Profiles: map[string]Profile{branch: profile}, Signer: sign, Cipher: cipher, PresentationKey: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{13}, 32))}
	type outcome struct {
		quote *quoteReply
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		quote, err := h.issue(ctx, terminalRequest)
		done <- outcome{quote, err}
	}()
	select {
	case <-sign.start:
	case first := <-done:
		t.Fatalf("issue stopped before signer: %v", first.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := h.issue(ctx, wrongClaim); err != service.Forbidden {
		t.Fatalf("POS obtained quote using the public sale without a private claim: %v", err)
	}
	if _, err := h.issue(ctx, requestB); err != service.Forbidden {
		t.Fatalf("other payer entered signing: %v", err)
	}
	if _, err := h.issue(ctx, requestA); err != service.Busy {
		t.Fatalf("concurrent same payer entered signing: %v", err)
	}
	exec("SELECT id FROM ledger.sale WHERE id=$1 FOR UPDATE NOWAIT", sale)
	close(sign.release)
	finished := <-done
	if finished.err != nil {
		t.Fatal(finished.err)
	}
	retry, err := h.issue(ctx, requestA)
	if err != nil || !reflect.DeepEqual(retry, finished.quote) || sign.count.Load() != 1 {
		t.Fatalf("committed quote replay differs: %v", err)
	}
	rotated := profile
	rotated.Beneficiary = common.HexToAddress("0x3002")
	rotated.Signer = common.HexToAddress("0x3003")
	rotated.BeneficiaryVersion++
	rotated.KeyVersion++
	rotation := rotated.Digest()
	exec("UPDATE ledger.issuance_profile SET version=2,authorized_digest=$2 WHERE branch_id=$1", branch, rotation[:])
	h.Profiles[branch] = rotated
	prior, err := h.issue(ctx, requestA)
	if err != nil || prior == nil || !reflect.DeepEqual(prior, retry) || sign.count.Load() != 1 {
		t.Fatalf("rotated issuance profile changed an existing quote: %v", err)
	}
	exec("UPDATE ledger.issuance_profile SET enabled=false WHERE branch_id=$1", branch)
	delete(h.Profiles, branch)
	prior, err = h.issue(ctx, requestA)
	if err != nil || prior == nil || !reflect.DeepEqual(prior, retry) || sign.count.Load() != 1 {
		t.Fatalf("issuance stop affected authenticated evidence recovery: %v", err)
	}
	exec("UPDATE ledger.issuance_profile SET version=1,authorized_digest=$2,enabled=true WHERE branch_id=$1", branch, enrolled[:])
	h.Profiles[branch] = profile
	exerciseAuthenticatedRoute(t, h, requestA, *retry)
	exerciseAuthenticatedRoute(t, h, terminalRequest, *retry)
	proof := retry.Presentation
	if proof == nil || proof.Customer != customerA || proof.Origin != posOrigin || proof.Claim != issuedForA.Digest {
		t.Fatal("authenticated P05 projection did not reflect the committed payer and POS reference")
	}
	parse := func(text string) [32]byte {
		raw, decodeErr := hex.DecodeString(text)
		if decodeErr != nil || len(raw) != 32 {
			t.Fatal("wrong proof word")
		}
		return [32]byte(raw)
	}
	signature, _ := hex.DecodeString(proof.Signature)
	rawQuote, _ := hex.DecodeString(retry.Signed)
	generation := binary.BigEndian.Uint64(rawQuote[536:544])
	verify, err := presentationMessage(branch, sale, customerA, posOrigin, parse(proof.Claim), parse(retry.OrderKey), parse(retry.Digest), generation, uint64(retry.ExpiresAt))
	if err != nil || !ed25519.Verify(h.PresentationKey.Public().(ed25519.PublicKey), verify, signature) {
		t.Fatal("bad independent presentation signature")
	}
	foreign, err := presentationMessage(branch, sale, customerA, posOrigin2, parse(proof.Claim), parse(retry.OrderKey), parse(retry.Digest), generation, uint64(retry.ExpiresAt))
	if err != nil || ed25519.Verify(h.PresentationKey.Public().(ed25519.PublicKey), foreign, signature) {
		t.Fatal("private presentation accepted another POS reference")
	}

	// A configured sponsorship witness may be attached to the exact old quote
	// without asking the merchant quote signer to issue it again.
	supported := h
	gas := GasSponsorship{Account: common.HexToAddress("0xfeed"), Grant: sha256.Sum256([]byte("synthetic signed policy")),
		ExpiresAt: time.Now().Add(time.Minute).Unix()}
	active := true
	supported.GasSponsors = func(_ context.Context, requested string, current Profile, payer common.Address, _ time.Time) (GasSponsorship, bool) {
		return gas, active && requested == branch && current == profile && payer != gas.Account
	}
	withGas := readAuthenticatedQuote(t, supported, terminalRequest, nil)
	if withGas.Signed != retry.Signed || withGas.Digest != retry.Digest || withGas.Presentation.GasSponsor == nil || sign.count.Load() != 1 {
		t.Fatal("nearby terminal did not receive the exact original quote and sponsorship witness")
	}
	witness := withGas.Presentation.GasSponsor
	gas.ExpiresAt = witness.ExpiresAt
	witnessSig, err := hex.DecodeString(witness.Signature)
	if err != nil || witness.Account != strings.ToLower(gas.Account.Hex()) || witness.Grant != hex.EncodeToString(gas.Grant[:]) ||
		witness.ExpiresAt > time.Now().Add(30*time.Second).Unix() ||
		!ed25519.Verify(h.PresentationKey.Public().(ed25519.PublicKey), gasSponsorMessage(verify, rawQuote, gas), witnessSig) ||
		ed25519.Verify(h.PresentationKey.Public().(ed25519.PublicKey), gasSponsorMessage(foreign, rawQuote, gas), witnessSig) {
		t.Fatal("the sponsorship proof was not limited to the original audience and quote")
	}
	active = false
	exerciseAuthenticatedRoute(t, supported, terminalRequest, *retry)
	if sign.count.Load() != 1 {
		t.Fatal("withdrawing sponsorship issued a second quote")
	}
	t.Log("real P05 terminal mTLS replay preserves one signed quote and returns sponsorship only while active")

	var eventCount int
	var ciphertext []byte
	if err := admin.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.quote_outbox WHERE sale_id=$1), payload_cipher FROM ledger.quote_work WHERE sale_id=$1`, sale).Scan(&eventCount, &ciphertext); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 || bytes.Equal(ciphertext, []byte(retry.Signed)) {
		t.Fatal("issue event/ciphertext not atomic")
	}
	exec("UPDATE ledger.quote_work SET consumed_at=now() WHERE sale_id=$1", sale)
	if _, err := h.issue(ctx, requestA); err != service.Expired {
		t.Fatalf("consumed quote exposed: %v", err)
	}

	cart.OriginRef = posOrigin2
	cart2 := authorize(t, pos, "order.create", branch, posOrigin2, other, fields)
	nextSale, err := store.Reserve(ctx, cart2, cart)
	if err != nil {
		t.Fatal(err)
	}
	nextClaim := claimPath(a, nextSale, nextSale)
	claimA, err := hex.DecodeString(nextClaim.Digest)
	if err != nil {
		t.Fatal(err)
	}
	next := authorize(t, a, "quote.issue", branch, nextSale, nextSale,
		map[string]string{"claimDigest": nextClaim.Digest, "orderRevision": "0"})
	exec("UPDATE ledger.issuance_profile SET enabled=false WHERE branch_id=$1", branch)
	if _, err := h.issue(ctx, next); err != service.Expired || sign.count.Load() != 1 {
		t.Fatalf("issuance stop allowed new signing: %v", err)
	}
	exec("UPDATE ledger.issuance_profile SET enabled=true WHERE branch_id=$1", branch)
	prepared, err := store.BeginQuote(ctx, next, [32]byte(claimA), 0, profile)
	if err != nil {
		t.Fatal(err)
	}
	exec("UPDATE ledger.quote_work SET lease_until=clock_timestamp()-interval '1 second' WHERE sale_id=$1", nextSale)
	resumed, err := store.BeginQuote(ctx, next, [32]byte(claimA), 0, profile)
	if err != nil || resumed.Fence != prepared.Fence+1 || resumed.QuoteID != prepared.QuoteID || resumed.Nonce != prepared.Nonce {
		t.Fatalf("crash restart allocated another authorization: %+v %v", resumed, err)
	}
	if err := store.FinishQuote(ctx, next, prepared, profile, make([]byte, 765), [32]byte{1}); err != service.Expired {
		t.Fatalf("stale signer fence was accepted: %v", err)
	}
	exec("UPDATE ledger.order_checkout SET expires_at=clock_timestamp()-interval '1 second' WHERE sale_id=$1", nextSale)
	exec("UPDATE ledger.order_checkout SET expires_at=clock_timestamp()-interval '1 second' WHERE sale_id=$1", sale)
	if reclaimed, err := store.ReclaimExpired(ctx); err != nil || !reclaimed {
		t.Fatalf("stale preview did not release inventory: %v", err)
	}
	if reclaimed, err := store.ReclaimExpired(ctx); err != nil || reclaimed {
		t.Fatalf("issued order was reclaimed or preview released twice: %v", err)
	}
	var left int
	if err := admin.QueryRow(ctx, "SELECT available FROM ledger.stock WHERE branch_id=$1 AND sku='pos'", branch).Scan(&left); err != nil || left != 2 {
		t.Fatalf("stock was restored more than once or issued stock changed: %d %v", left, err)
	}
	var optionUnits, heldOptions int
	if err := admin.QueryRow(ctx, "SELECT sum(available) FROM ledger.catalog_option_stock WHERE branch_id=$1 AND sku='pos'", branch).Scan(&optionUnits); err != nil || optionUnits != 2 {
		t.Fatalf("both options must restore exactly once: %d %v", optionUnits, err)
	}
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM ledger.catalog_option_hold WHERE sale_id=$1", nextSale).Scan(&heldOptions); err != nil || heldOptions != 0 {
		t.Fatalf("reclaimed order retained inventory holds: %d %v", heldOptions, err)
	}

	for _, want := range []bool{true, false} {
		var closed bool
		err := admin.QueryRow(ctx, `SELECT ledger.close_unpaid(sale_id,signed_digest,gen_random_uuid(),expires_at)
			FROM ledger.quote_work WHERE sale_id=$1`, sale).Scan(&closed)
		if err != nil || closed != want {
			t.Fatalf("issued order close returned %v, want %v: %v", closed, want, err)
		}
	}
	if err := admin.QueryRow(ctx, `SELECT s.available,min(o.available),count(o.option_key)
		FROM ledger.stock s JOIN ledger.catalog_option_stock o USING(branch_id,sku)
		WHERE s.branch_id=$1 AND s.sku='pos' GROUP BY s.available`, branch).
		Scan(&remainingSKU, &earlierOption, &heldOptions); err != nil || remainingSKU != 4 || earlierOption != 2 || heldOptions != 2 {
		t.Fatalf("issued order must restore both options and SKU once: %d %d %d %v", remainingSKU, earlierOption, heldOptions, err)
	}
	issueP01LedgerQuotes(t, ctx, h, exec, func(sale string) (string, int, int) {
		var state string
		var payments, relays int
		err := admin.QueryRow(ctx, `SELECT s.state,
		  (SELECT count(*) FROM ledger.payment WHERE sale_id=s.id), `+countOrderRelayAttempts+`
		  FROM ledger.sale s WHERE s.id=$1`, sale).Scan(&state, &payments, &relays)
		if err != nil {
			t.Fatal(err)
		}
		return state, payments, relays
	}, profile)
}
