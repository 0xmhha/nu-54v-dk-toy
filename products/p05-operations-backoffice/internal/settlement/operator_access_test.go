package settlement

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p05-operations-backoffice/internal/commerce"
	"github.com/ethereum/go-ethereum/common"
)

func TestPostgresOperatorAccessRequiresTwoIdentities(t *testing.T) {
	db, dsn := scratchDB(t)
	ctx := t.Context()
	tenant, branch, other := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	maker, checker, terminal, staff := fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db), fixtureUUID(t, db)
	assignee, posSource := fixtureUUID(t, db), fixtureUUID(t, db)
	firstCross, secondCross := fixtureUUID(t, db), fixtureUUID(t, db)
	fixtureExec(t, db, `INSERT INTO ledger.branch VALUES($1,$3,'America/New_York','04:00'),($2,$3,'Asia/Seoul','04:00')`, branch, other, tenant)
	for index, actor := range []string{maker, checker, terminal, staff, assignee, firstCross, secondCross} {
		fixtureExec(t, db, "INSERT INTO ledger.login_subject VALUES($1,$2)", fmt.Sprintf("fixture_operator_%s_%d", tenant, index), actor)
	}
	for _, actor := range []string{maker, checker} {
		fixtureExec(t, db, `INSERT INTO ledger.finance_grant VALUES($1,$2,'access_manage',now()+interval '1 hour'),
         ($1,$2,'audit_read',now()+interval '1 hour')`, branch, actor)
	}
	fixtureExec(t, db, `INSERT INTO ledger.staff_grant VALUES($1,$3,'operator',now(),NULL),($2,$3,'operator',now(),NULL)`, branch, other, maker)
	_, scoped := testLogin(t, db, dsn, "nu54_p05_commerce")
	store, err := commerce.Open(ctx, scoped)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	trust, certs, _ := mtlsClients(t, 8)
	identities := map[string]service.Principal{}
	valid := time.Now().Add(time.Hour).Unix()
	identities[fingerprint(certs[0])] = service.Principal{Role: "operator", Tenant: tenant, Subject: maker, ExpiresAt: valid}
	identities[fingerprint(certs[1])] = service.Principal{Role: "operator", Tenant: tenant, Subject: checker, ExpiresAt: valid}
	identities[fingerprint(certs[2])] = service.Principal{Role: "terminal", Tenant: tenant, Subject: terminal, ExpiresAt: valid,
		Grants: []service.Grant{{Domain: branch, Object: terminal, Operations: []string{"C3/terminal.state"}}}}
	identities[fingerprint(certs[3])] = service.Principal{Role: "operator", Tenant: tenant, Subject: staff, ExpiresAt: valid}
	identities[fingerprint(certs[4])] = service.Principal{Role: "operator", Tenant: tenant, Subject: assignee, ExpiresAt: valid}
	identities[fingerprint(certs[5])] = service.Principal{Role: "pos", Tenant: tenant, Subject: posSource, ExpiresAt: valid,
		Grants: []service.Grant{{Domain: branch, Object: branch, Operations: []string{"POS/submit"}}}}
	identities[fingerprint(certs[6])] = service.Principal{Role: "operator", Tenant: tenant, Subject: firstCross, ExpiresAt: valid}
	identities[fingerprint(certs[7])] = service.Principal{Role: "operator", Tenant: tenant, Subject: secondCross, ExpiresAt: valid}
	asset := common.HexToAddress("0x0000000000000000000000000000000000000001")
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	origin := "https://" + server.Listener.Addr().String()
	browserScript, browserDist := os.Getenv("NU54_ACCESS_BROWSER_SCRIPT"), os.Getenv("NU54_ACCESS_BROWSER_DIST")
	handoffDist := os.Getenv("NU54_ACCESS_HANDOFF_DIST")
	dist := browserDist
	if handoffDist != "" {
		dist = handoffDist
	}
	if browserScript != "" || handoffDist != "" {
		if (browserScript != "" && !filepath.IsAbs(browserScript)) || !filepath.IsAbs(dist) {
			t.Fatal("absolute operator browser script and dist required")
		}
		mux.Handle("/ops/", http.StripPrefix("/ops/", http.FileServer(http.Dir(dist))))
	}
	commerce.HTTP{Store: store, Credentials: identities, OperatorOrigin: origin,
		Profiles: map[string]commerce.Profile{branch: {Chain: 8283, Asset: asset}}}.Register(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: trust}
	server.StartTLS()
	defer server.Close()
	clients := make([]*http.Client, len(certs))
	for i, cert := range certs {
		cfg := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
		cfg.Certificates = []tls.Certificate{cert}
		clients[i] = &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 10 * time.Second}
		defer clients[i].CloseIdleConnections()
	}
	call := func(who int, path string, body any, want int) string {
		t.Helper()
		method := "GET"
		if body != nil {
			method = "POST"
		}
		wire, _ := json.Marshal(body)
		r, _ := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(wire))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		reply, err := clients[who].Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		raw, err := io.ReadAll(reply.Body)
		if err != nil || reply.StatusCode != want {
			t.Fatalf("%s: HTTP %d, want %d: %s %v", path, reply.StatusCode, want, raw, err)
		}
		return string(raw)
	}
	names := call(0, "/v2/operations/scopes", nil, 200)
	if !strings.Contains(names, branch) || !strings.Contains(names, other) || strings.Count(names, "등록 이름 없음") != 2 {
		t.Fatal("existing unnamed branches missing after upgrade")
	}
	path := "/v2/operations/access/" + branch
	call(0, path+"/name", map[string]string{"label": "기존 운영 지점"}, 200)
	if scopes := call(0, "/v2/operations/scopes", nil, 200); !strings.Contains(scopes, "기존 운영 지점") {
		t.Fatal(scopes)
	}
	browser := func(phase string, args ...string) {
		t.Helper()
		if browserScript == "" {
			return
		}
		dir := t.TempDir()
		env := append(os.Environ(), "NU54_ACCESS_ORIGIN="+origin, "NU54_ACCESS_BRANCH="+branch, "NU54_ACCESS_OTHER_BRANCH="+other, "NU54_ACCESS_DIR="+dir)
		for index, label := range []string{"maker", "checker"} {
			key, err := x509.MarshalPKCS8PrivateKey(certs[index].PrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			for suffix, block := range map[string]pem.Block{
				".crt": {Type: "CERTIFICATE", Bytes: certs[index].Certificate[0]},
				".key": {Type: "PRIVATE KEY", Bytes: key},
			} {
				if err := os.WriteFile(filepath.Join(dir, label+suffix), pem.EncodeToMemory(&block), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
		cmd := exec.CommandContext(ctx, "sh", append([]string{browserScript, phase}, args...)...)
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("browser %s: %v\n%s", phase, err, output)
		}
		t.Logf("browser %s verified", phase)
	}
	change := func(target, role, operation string) string {
		t.Helper()
		key := fixtureUUID(t, db)
		command := map[string]string{"requestKey": key, "target": target, "operation": operation, "role": role}
		call(0, "/v2/operations/access/"+other+"/propose", command, 404)
		if browserScript == "" {
			call(0, path+"/propose", command, 200)
			call(0, path+"/approve", map[string]string{"requestKey": key}, 404)
			call(1, path+"/approve", map[string]string{"requestKey": key}, 200)
		} else {
			browser("change", target, role, operation)
			if err := db.QueryRow(ctx, `SELECT key::text FROM ledger.operator_access_change
				WHERE branch_id=$1 AND subject=$2 AND role_code=$3 AND operation=$4 AND proposer=$5
				  AND approver=$6 AND applied_at IS NOT NULL`, branch, target, role, operation, maker, checker).Scan(&key); err != nil {
				t.Fatal(err)
			}
		}
		return key
	}
	register := change(terminal, "terminal", "register")
	terminalRead := func(want int) {
		call(2, "/v2/commerce", map[string]any{"command": service.Request{Version: 2, Contract: "C3", Operation: "terminal.state",
			Domain: branch, Object: terminal, RequestID: fixtureUUID(t, db), ExpiresAt: time.Now().Add(20 * time.Second).Unix(), Body: map[string]string{}}}, want)
	}
	terminalRead(200)
	if view := call(0, path, nil, 200); strings.Contains(view, `"acknowledgedAt":null`) {
		t.Fatal("terminal's real authenticated read did not record acknowledgement")
	}
	browser("ack", register)
	fixtureExec(t, db, "INSERT INTO ledger.staff_grant VALUES($1,$2,'operator',now()-interval '1 minute',NULL)", branch, assignee)
	fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'pickup_manage',now()+interval '1 hour')", branch, assignee)
	fixtureExec(t, db, "INSERT INTO ledger.command_rule VALUES($1,'fixture','fixture','fixture',interval '1 hour')", branch)
	fixtureExec(t, db, "INSERT INTO ledger.pos_source VALUES($1,$2,$3,now()+interval '1 hour')", branch, posSource, terminal)
	posSubmit := func(reference string, want int) {
		t.Helper()
		call(5, "/v2/commerce", map[string]any{"pos": map[string]any{
			"operation": "submit", "branch": branch, "reference": reference, "chainId": 8283, "asset": lower(asset), "amount": "25",
		}}, want)
	}
	posSubmit("original", 200)
	var original string
	if err := db.QueryRow(ctx, "SELECT sale_id::text FROM ledger.pos_order WHERE branch_id=$1 AND external_ref='original'", branch).
		Scan(&original); err != nil {
		t.Fatal(err)
	}
	payload := map[string]string{"target": terminal, "role": "terminal", "handoffTo": assignee}
	var review struct {
		ReviewKey string
		OrderRefs []string
		HandoffTo string
	}
	previewBody := call(0, path+"/revoke-review", payload, 200)
	if err := json.Unmarshal([]byte(previewBody), &review); err != nil || review.ReviewKey == "" ||
		len(review.OrderRefs) != 1 || review.OrderRefs[0] != original || review.HandoffTo != assignee {
		t.Fatalf("wrong original order impact: %s %v", previewBody, err)
	}
	handoffBrowser := func(who int, phase, key string) {
		t.Helper()
		if handoffDist == "" {
			return
		}
		dir := t.TempDir()
		keyMaterial, err := x509.MarshalPKCS8PrivateKey(certs[who].PrivateKey)
		if err != nil {
			t.Fatal(err)
		}
		certPath, keyPath := filepath.Join(dir, "operator.crt"), filepath.Join(dir, "operator.key")
		for path, block := range map[string]*pem.Block{
			certPath: {Type: "CERTIFICATE", Bytes: certs[who].Certificate[0]},
			keyPath:  {Type: "PRIVATE KEY", Bytes: keyMaterial},
		} {
			if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.CommandContext(ctx, "node", "../../test/finance/access-handoff-browser.cjs",
			origin, branch, terminal, original, assignee, "terminal", phase, key, certPath, keyPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("handoff Chrome %s: %v\n%s", phase, err, out)
		} else {
			t.Logf("handoff Chrome %s: %s", phase, out)
		}
	}
	handoffBrowser(0, "cancel", "")
	terminalRead(200)
	var drafts int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM ledger.operator_access_change WHERE branch_id=$1 AND operation='revoke'", branch).Scan(&drafts); err != nil || drafts != 0 {
		t.Fatalf("review or cancel changed access: %d %v", drafts, err)
	}
	call(0, "/v2/operations/access/"+other+"/revoke-review", payload, 404)
	revoke := fixtureUUID(t, db)
	commandForRevoke := map[string]string{"requestKey": revoke, "target": terminal, "role": "terminal", "operation": "revoke"}
	call(0, path+"/propose", commandForRevoke, 409)
	commandForRevoke["handoffTo"] = assignee
	commandForRevoke["reviewKey"] = review.ReviewKey
	if handoffDist != "" {
		handoffBrowser(0, "propose", "")
		if err := db.QueryRow(ctx, `SELECT key::text FROM ledger.operator_access_change WHERE branch_id=$1
			AND subject=$2 AND operation='revoke' AND applied_at IS NULL`, branch, terminal).Scan(&revoke); err != nil {
			t.Fatal(err)
		}
		commandForRevoke["requestKey"] = revoke
	}
	call(0, path+"/propose", commandForRevoke, 200)
	call(0, path+"/propose", commandForRevoke, 200)
	call(0, path+"/approve", map[string]string{"requestKey": revoke}, 404)
	if view := call(4, path+"/handoffs", nil, 200); view != "[]\n" {
		t.Fatalf("pending proposal exposed work to its recipient: %s", view)
	}
	terminalRead(200)
	posSubmit("added-after-review", 200)
	var added string
	if err := db.QueryRow(ctx, "SELECT sale_id::text FROM ledger.pos_order WHERE branch_id=$1 AND external_ref='added-after-review'", branch).
		Scan(&added); err != nil {
		t.Fatal(err)
	}
	call(1, path+"/approve", map[string]string{"requestKey": revoke}, 409)
	terminalRead(200)
	fixtureExec(t, db, "UPDATE ledger.sale SET state='closed' WHERE id=$1", added)
	fixtureExec(t, db, "UPDATE ledger.finance_grant SET valid_until=clock_timestamp() WHERE branch_id=$1 AND actor_id=$2 AND capability='pickup_manage'", branch, assignee)
	call(1, path+"/approve", map[string]string{"requestKey": revoke}, 404)
	terminalRead(200)
	fixtureExec(t, db, "UPDATE ledger.finance_grant SET valid_until=now()+interval '1 hour' WHERE branch_id=$1 AND actor_id=$2 AND capability='pickup_manage'", branch, assignee)
	if handoffDist != "" {
		handoffBrowser(1, "approve", revoke)
	}
	call(1, path+"/approve", map[string]string{"requestKey": revoke}, 200)
	call(1, path+"/approve", map[string]string{"requestKey": revoke}, 200)
	terminalRead(404)
	posSubmit("new-after-revocation", 404)
	if view := call(4, path+"/handoffs", nil, 200); !strings.Contains(view, original) || !strings.Contains(view, revoke) {
		t.Fatalf("assigned original not visible to the assignee: %s", view)
	}
	call(1, path+"/handoff-ack", map[string]string{"requestKey": revoke, "saleId": original}, 404)
	call(4, "/v2/operations/access/"+other+"/handoffs", nil, 404)
	handoffBrowser(4, "receive", revoke)
	call(4, path+"/handoff-ack", map[string]string{"requestKey": revoke, "saleId": original}, 200)
	call(4, path+"/handoff-ack", map[string]string{"requestKey": revoke, "saleId": original}, 200)
	var handoffs, retained, rejected int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM ledger.operator_access_handoff WHERE branch_id=$1 AND access_key=$2 AND accepted_at IS NOT NULL),
		(SELECT count(*) FROM ledger.order_checkout WHERE sale_id=$3 AND creator=$4),
		(SELECT count(*) FROM ledger.pos_order WHERE branch_id=$1 AND external_ref='new-after-revocation')`, branch, revoke, original, terminal).
		Scan(&handoffs, &retained, &rejected); err != nil || handoffs != 1 || retained != 1 || rejected != 0 {
		t.Fatalf("handoff did not preserve exactly one original: %d %d %d %v", handoffs, retained, rejected, err)
	}
	t.Logf("revocation %s assigned same original order %s to authorized staff %s; one acceptance and no change to the original checkout", revoke, original, assignee)
	zone, _ := time.LoadLocation("America/New_York")
	today := time.Now().In(zone).Format("2006-01-02")
	call(0, "/v2/operations/audit/"+branch+"?from="+today+"&until="+today, nil, 200)
	call(1, "/v2/operations/audit/"+other+"?from="+today+"&until="+today, nil, 404)
	fixtureExec(t, db, "INSERT INTO ledger.staff_grant VALUES($1,$2,'operator',now(),NULL)", branch, staff)
	call(3, path, nil, 404)
	call(3, "/v2/operations/catalog/"+branch, nil, 200)
	fixtureExec(t, db, `INSERT INTO ledger.finance_grant VALUES
       ($1,$2,'legacy_ledger',now()+interval '1 hour'),($1,$2,'legacy_cashout',now()+interval '1 hour'),($1,$2,'reconcile_read',now()+interval '1 hour')`, branch, staff)
	var writable bool
	if err := db.QueryRow(ctx, "SELECT ledger.legacy_cashout_scope($1,$2,$3,true)", tenant, branch, staff).Scan(&writable); err != nil || !writable {
		t.Fatalf("precondition legacy grant absent %v", err)
	}
	staffRevoke := change(staff, "operator", "revoke")
	call(3, "/v2/operations/catalog/"+branch, nil, 404)
	browser("history", today, register, revoke, staffRevoke)
	if err := db.QueryRow(ctx, "SELECT ledger.legacy_cashout_scope($1,$2,$3,true)", tenant, branch, staff).Scan(&writable); err == nil {
		t.Fatal("revoked staff can withdraw from legacy profile")
	}
	if _, err := db.Exec(ctx, "SELECT * FROM ledger.operator_payment_evidence($1,$2,$3,$4)", tenant, branch, staff, fixtureUUID(t, db)); err == nil ||
		sqlError(err) != service.Forbidden {
		t.Fatalf("revoked staff can read payout evidence: %v", err)
	}
	// Two competing revocations nominate each other's target. The branch
	// orders the authority updates, so only the still-authorized recipient wins.
	for _, cross := range []string{firstCross, secondCross} {
		fixtureExec(t, db, "INSERT INTO ledger.staff_grant VALUES($1,$2,'operator',now()-interval '1 minute',NULL)", branch, cross)
		fixtureExec(t, db, "INSERT INTO ledger.finance_grant VALUES($1,$2,'pickup_manage',now()+interval '1 hour')", branch, cross)
		sale := fixtureUUID(t, db)
		fixtureExec(t, db, "INSERT INTO ledger.sale(id,branch_id,origin,origin_ref,asset_ref,amount) VALUES($1,$2,'pos',gen_random_uuid(),$3,25)", sale, branch, lower(asset))
		fixtureExec(t, db, "INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,expires_at,intent_digest) VALUES($1,$2,$3,now()+interval '1 hour',sha256('crossed revocation fixture'))", sale, branch, cross)
	}
	crossKeys := make([]string, 0, 2)
	for _, crossed := range [][2]string{{firstCross, secondCross}, {secondCross, firstCross}} {
		var plan struct{ ReviewKey string }
		wire := call(0, path+"/revoke-review", map[string]string{"target": crossed[0], "role": "operator", "handoffTo": crossed[1]}, 200)
		if err := json.Unmarshal([]byte(wire), &plan); err != nil || plan.ReviewKey == "" {
			t.Fatalf("bad crossed review: %s %v", wire, err)
		}
		key := fixtureUUID(t, db)
		crossKeys = append(crossKeys, key)
		call(0, path+"/propose", map[string]string{"requestKey": key, "operation": "revoke", "role": "operator", "target": crossed[0], "handoffTo": crossed[1], "reviewKey": plan.ReviewKey}, 200)
	}
	outcomes := make(chan int, 2)
	startCross := make(chan struct{})
	for _, key := range crossKeys {
		go func(key string) {
			<-startCross
			payload, _ := json.Marshal(map[string]string{"requestKey": key})
			request, err := http.NewRequestWithContext(ctx, "POST", server.URL+path+"/approve", bytes.NewReader(payload))
			if err != nil {
				outcomes <- 0
				return
			}
			request.Header.Set("Origin", origin)
			request.Header.Set("Content-Type", "application/json")
			response, err := clients[1].Do(request)
			if err != nil {
				outcomes <- 0
				return
			}
			response.Body.Close()
			outcomes <- response.StatusCode
		}(key)
	}
	close(startCross)
	left, right := <-outcomes, <-outcomes
	if !((left == 200 && right == 404) || (left == 404 && right == 200)) {
		t.Fatalf("crossed revocations did not serialize: %d and %d", left, right)
	}
	t.Log("crossed assignee revocations serialized; one still-authorized assignment")

	// Queue a reviewer behind the held target lock, then retire the maker
	// before committing. An HTTP identity and old statement time are insufficient.
	pending := fixtureUUID(t, db)
	command := map[string]string{"requestKey": pending, "target": terminal, "operation": "register", "role": "terminal"}
	call(0, path+"/propose", command, 200)
	fence, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer fence.Rollback(ctx)
	if _, err := fence.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1||$2||'terminal',32))", branch, terminal); err != nil {
		t.Fatal(err)
	}
	denied := make(chan int, 1)
	go func() {
		payload, _ := json.Marshal(map[string]string{"requestKey": pending})
		request, _ := http.NewRequestWithContext(ctx, "POST", server.URL+path+"/approve", bytes.NewReader(payload))
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		reply, err := clients[1].Do(request)
		if err != nil {
			denied <- 0
			return
		}
		reply.Body.Close()
		denied <- reply.StatusCode
	}()
	waitUntil(t, func() bool {
		var waiting bool
		err := db.QueryRow(ctx, "SELECT EXISTS (SELECT FROM pg_stat_activity WHERE wait_event='advisory' AND query LIKE '%ledger.apply_operator_access%')").Scan(&waiting)
		return err == nil && waiting
	})
	if _, err := fence.Exec(ctx, "UPDATE ledger.finance_grant SET valid_until=clock_timestamp() WHERE branch_id=$1 AND actor_id=$2 AND capability='access_manage'", branch, maker); err != nil {
		t.Fatal(err)
	}
	if err := fence.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if result := <-denied; result != 404 {
		t.Fatalf("stale approval committed: %d", result)
	}
	terminalRead(404)
}
