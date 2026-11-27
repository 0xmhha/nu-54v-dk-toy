package commerce

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/ethereum/go-ethereum/common"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
)

func uuidForCatalogFixture(t *testing.T, ctx context.Context, admin *pgx.Conn) string {
	t.Helper()
	var key string
	if err := admin.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestPostgresOperatorCatalogRealTLS(t *testing.T) {
	runPostgresOperatorCatalogRealTLS(t, nil)
}

// RunInstalledW10Settlement reuses the normal installed menu entry, with a
// dedicated local deployment and settlement worker supplied by the test.
func RunInstalledW10Settlement(t *testing.T, settlement InstalledW10Settlement) {
	t.Helper()
	if os.Getenv("NU54_K10_INSTALLED_RUNNER") == "" || os.Getenv("NU54_K10_SIGNED_REVIEW") != "1" ||
		os.Getenv("NU54_W10_INSTALLED_BOARD") != "1" || settlement.Profile.Chain != 8283 ||
		settlement.Profile.Asset == (common.Address{}) || settlement.Profile.Contract == (common.Address{}) ||
		settlement.Signer == nil || settlement.QuoteCipher == nil || settlement.Payer == nil ||
		settlement.RelayAccount == (common.Address{}) || settlement.LookupSponsor == nil ||
		settlement.Forwarder == nil || settlement.FinalizeOriginal == nil || settlement.RegisterFinance == nil {
		t.Fatal("W10 installed settlement requires a complete local deployment and runner")
	}
	t.Setenv("NU54_W10_INSTALLED_SETTLEMENT", "1")
	runPostgresOperatorCatalogRealTLS(t, &settlement)
}

func runPostgresOperatorCatalogRealTLS(t *testing.T, settlement *InstalledW10Settlement) {
	config, err := pgx.ParseConfig(os.Getenv("P05_COMMERCE_TEST_DATABASE_URL"))
	if os.Getenv("P05_COMMERCE_TEST_DATABASE_URL") == "" {
		t.Skip("requires disposable migrated PostgreSQL")
	}
	if err != nil || !(strings.HasPrefix(config.Database, "nu54_commerce_") ||
		settlement != nil && strings.HasPrefix(config.Database, "nu54_settlement_")) ||
		!(strings.HasPrefix(config.Host, "/private/tmp/") || strings.HasPrefix(config.Host, "/tmp/")) {
		t.Fatal("use disposable commerce database")
	}
	browserScript := os.Getenv("NU54_CATALOG_BROWSER")
	liveHold := os.Getenv("NU54_K10_LIVE_HOLD") == "1"
	installedRunner := os.Getenv("NU54_K10_INSTALLED_RUNNER")
	timeout := 45 * time.Second
	certificateLifetime := time.Minute
	if liveHold || installedRunner != "" {
		timeout, certificateLifetime = 7*time.Minute, 8*time.Minute
	}
	if browserScript != "" {
		timeout = 2 * time.Minute
	}
	ctx, stop := context.WithTimeout(context.Background(), timeout)
	defer stop()
	admin, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	execute := func(q string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	role := "nu54_operator_" + strings.TrimPrefix(config.Database, "nu54_commerce_")
	execute("CREATE ROLE " + pgx.Identifier{role}.Sanitize() + " LOGIN IN ROLE nu54_p05_commerce")
	defer admin.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize())
	store, err := Open(ctx, config.ConnString()+" user="+role)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	branchOne := "5dc8d4be-c24d-4553-ab79-9c30f630e477"
	if settlement != nil {
		branchOne = settlement.Branch
	}
	const branchTwo = "b7d3377e-7c01-4a53-b9d6-fa5584557fae"
	const operator = "94001137-522a-44b9-b8b4-e3fceff2a920"
	const publishKey = "2392fb5e-d913-44db-8f90-b113837b7d79"
	const adjustKey = "4b858c3c-a377-41f5-8eb7-013ee13cfcbc"
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	defer server.Close()
	origin := "https://" + server.Listener.Addr().String()
	execute("INSERT INTO ledger.branch VALUES ($1,$3,'Asia/Seoul','04:00'),($2,$3,'Asia/Seoul','04:00')", branchOne, branchTwo, tenant)
	execute("INSERT INTO ledger.login_subject VALUES ('catalog_fixture_operator',$1)", operator)
	execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", branchOne, operator)
	execute("INSERT INTO ledger.command_rule VALUES ($1,'catalog test','fixture','dispose',interval '2 minutes')", branchOne)
	_, fixtureKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var key crypto.Signer = fixtureKey
	template := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(certificateLifetime),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if installedRunner != "" {
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template.Subject.CommonName = "Local K10 installation fixture CA"
		template.ExtKeyUsage = nil
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	cafile := x509.NewCertPool()
	cafile.AddCert(cert)
	digest := sha256.Sum256(der)
	asset := "0x" + strings.Repeat("2", 40)
	if settlement != nil {
		asset = hexAddress(settlement.Profile.Asset)
	}
	handler := HTTP{Store: store, OperatorOrigin: origin, Profiles: map[string]Profile{
		branchOne: {Chain: 8283, Asset: common.HexToAddress(asset)},
		branchTwo: {Chain: 8283, Asset: common.HexToAddress(asset)},
	}, Credentials: map[string]service.Principal{
		hex.EncodeToString(digest[:]): {Tenant: tenant, Subject: operator, Role: "operator", ExpiresAt: time.Now().Add(certificateLifetime).Unix()},
	}}
	const kiosk = "3ce37c55-4f32-433a-9807-2bef754496b5"
	execute("INSERT INTO ledger.commerce_grant (branch_id,actor_id,role_code,terminal_mode,valid_until) VALUES ($1,$2,'terminal','menu',now()+interval '2 minutes')", branchOne, kiosk)
	const clerk = "72530fcc-2be8-4db2-b545-404ec270a337"
	const maker = "d50021cb-6bb5-40aa-aaf9-bd278fa3420f"
	execute("INSERT INTO ledger.finance_grant VALUES ($1,$2,'pickup_manage',now()+interval '2 minutes'),($1,$3,'refund_request',now()+interval '2 minutes')", branchOne, clerk, maker)
	execute(`WITH new_orders AS (
		INSERT INTO ledger.sale(branch_id,origin,origin_ref,asset_ref,amount)
		  SELECT $1,'pos',gen_random_uuid(),$3,7 FROM generate_series(1,21) RETURNING id,branch_id
	) INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,expires_at,intent_digest)
	  SELECT id,branch_id,$2,now()+interval '2 minutes',sha256('K06 order') FROM new_orders`, branchOne, kiosk, asset)
	enroll := func(serial int64, subject, role string) tls.Certificate {
		t.Helper()
		public, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		var signingKey crypto.Signer = private
		var publicKey crypto.PublicKey = public
		if settlement != nil && serial == 4 {
			signingKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			publicKey = signingKey.Public()
		}
		leaf := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: template.NotBefore, NotAfter: template.NotAfter,
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		raw, err := x509.CreateCertificate(rand.Reader, leaf, cert, publicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(raw)
		handler.Credentials[hex.EncodeToString(digest[:])] = service.Principal{
			Tenant: tenant, Subject: subject, Role: role, ExpiresAt: time.Now().Add(certificateLifetime).Unix(),
		}
		return tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: signingKey}
	}
	kioskCert, clerkCert, makerCert := enroll(2, kiosk, "terminal"), enroll(3, clerk, "operator"), enroll(4, maker, "finance")
	purchaserCert, otherCustomerCert := enroll(5, customerA, "customer"), enroll(6, customerB, "customer")
	const menuKiosk = "21850a5b-154f-4c4c-8b7e-f0b820d22d46"
	menuKioskCert := enroll(7, menuKiosk, "terminal")
	execute("INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,terminal_mode,valid_until) VALUES($1,$2,'terminal','menu',now()+interval '7 minutes')", branchOne, menuKiosk)
	handler.Firmware = map[string]FirmwareRelease{strings.Repeat("a", 64): countedImageFixture(t)}
	rental := &testRentalOperator{}
	handler.Rental = rental
	if browserScript != "" {
		dist := os.Getenv("NU54_FINANCE_BROWSER_DIST")
		if !filepath.IsAbs(browserScript) || !filepath.IsAbs(dist) {
			t.Fatal("catalog browser requires absolute script and dist paths")
		}
		mux.Handle("/ops/", http.StripPrefix("/ops/", http.FileServer(http.Dir(dist))))
	}
	var signedMenu *installedSignedMenu
	var ownerBoard *installedMenuBoard
	var renterProof *installedRenterProof
	if installedRunner != "" && os.Getenv("NU54_K10_SIGNED_REVIEW") == "1" {
		signedMenu = prepareInstalledSignedMenu(t, ctx, admin, &handler, branchOne, asset, settlement)
		if settlement != nil && settlement.Rental != nil {
			rental := settlement.Rental
			renterProof = &installedRenterProof{}
			server.Config.Handler = renterProof.wrap(mux)
			handler.RentalEscrow = rental.Escrow
			handler.RentalReview = func(_ context.Context, branch string, profile Profile, manifest []byte, payer common.Address, generation uint64, amount *big.Int) (RentalReviewCommissioning, bool) {
				commissioned := rental.Kits[rental.kit.Load()].Commissioning
				return commissioned, branch == branchOne && profile.Contract == settlement.Profile.Contract &&
					bytes.Equal(manifest, rental.Manifest) && payer == gethcrypto.PubkeyToAddress(settlement.Payer.PublicKey) &&
					generation == 1 && amount.Cmp(big.NewInt(15_000_000)) == 0
			}
		} else if os.Getenv("NU54_W10_INSTALLED_BOARD") == "1" {
			ownerBoard = prepareInstalledMenuBoard(t, ctx)
			if settlement != nil {
				ownerBoard.payer = settlement.Payer
			} else {
				signedMenu.sponsorFixture(t, &handler, branchOne)
			}
		}
	}
	handler.Register(mux)
	var dispatchCert tls.Certificate
	if settlement != nil && settlement.RegisterPickupReceiver != nil {
		dispatchCert = enroll(8, uuidForCatalogFixture(t, ctx, admin), "test-pickup-delivery")
		fingerprint := sha256.Sum256(dispatchCert.Certificate[0])
		delete(handler.Credentials, hex.EncodeToString(fingerprint[:]))
		settlement.RegisterPickupReceiver(mux, hex.EncodeToString(fingerprint[:]))
	}
	if settlement != nil && settlement.Rental == nil {
		dist := os.Getenv("NU54_FINANCE_BROWSER_DIST")
		if !filepath.IsAbs(dist) {
			t.Fatal("installed W10 settlement requires the built finance console")
		}
		mux.Handle("/ops/", http.StripPrefix("/ops/", http.FileServer(http.Dir(dist))))
		execute("INSERT INTO ledger.finance_grant VALUES ($1,$2,'report_read',now()+interval '8 minutes')", branchOne, maker)
		settlement.RegisterFinance(mux, handler.Credentials, origin)
	}
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: cafile}
	if installedRunner != "" {
		server.TLS.Certificates = []tls.Certificate{menuFixtureServerCert(t, cert, key)}
	}
	server.StartTLS()
	configTLS := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	configTLS.Certificates = []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: configTLS}, Timeout: 4 * time.Second}
	defer client.CloseIdleConnections()
	clientFor := func(cert tls.Certificate) *http.Client {
		tlsConfig := configTLS.Clone()
		tlsConfig.Certificates = []tls.Certificate{cert}
		return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}, Timeout: 4 * time.Second}
	}
	kioskClient, clerkClient, makerClient := clientFor(kioskCert), clientFor(clerkCert), clientFor(makerCert)
	defer kioskClient.CloseIdleConnections()
	defer clerkClient.CloseIdleConnections()
	defer makerClient.CloseIdleConnections()
	exerciseKioskStaffRealTLS(t, ctx, server.URL, client, kioskClient, clerkClient, makerClient,
		branchOne, branchTwo, operator, kiosk, clerk, maker, origin, execute)
	exercisePickupAssignmentResponse(t, ctx, config.ConnString(), server.URL, kioskClient, branchOne, kiosk)
	exerciseTerminalModeRealTLS(t, ctx, server.URL, client, kioskClient, branchOne, branchTwo, kiosk)
	call := func(method, path, source string, body map[string]any, expected int) map[string]any {
		t.Helper()
		packed, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		endpoint := "/v2/operations/catalog/" + path
		if strings.HasPrefix(path, "/") {
			endpoint = path
		}
		request, err := http.NewRequestWithContext(ctx, method, server.URL+endpoint, strings.NewReader(string(packed)))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", source)
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var reply map[string]any
		readErr := json.NewDecoder(response.Body).Decode(&reply)
		if response.StatusCode != expected || expected == 200 && readErr != nil {
			t.Fatalf("%s got %d %v %v", path, response.StatusCode, reply, readErr)
		}
		return reply
	}
	displayAsset := call("GET", branchOne, origin, nil, 200)["asset"].(map[string]any)
	if displayAsset["symbol"] != "TestUSDC" || displayAsset["decimals"] != float64(6) || displayAsset["address"] != asset {
		t.Fatalf("unverified console asset: %v", displayAsset)
	}
	call("GET", branchTwo, origin, nil, 404)
	rows := []operatorRow{{SKU: "tea", Option: "hot", Label: "Hot tea", Asset: asset, DisplayPrice: "7.10", Tax: "0.10", Discount: "0.20", Required: true}, {SKU: "tea", Option: "cold", Label: "Iced tea", Asset: asset, DisplayPrice: "8.00", Required: true}}
	milk := []string{"milk"}
	none := []string{}
	rows[0].Metadata = &menuMetadata{Labels: map[string]string{"ko": "차", "en": "Tea"},
		OptionLabels: map[string]string{"ko": "따뜻함", "en": "Hot"}, Allergens: &milk, CrossContact: "unknown"}
	rows[1].Metadata = &menuMetadata{Allergens: &none, CrossContact: "possible"}
	publish := map[string]any{"requestKey": publishKey, "version": 0, "expiresAt": time.Now().Add(time.Hour).UTC(), "rows": rows}
	call("POST", branchOne+"/publish", "https://foreign.example.invalid", publish, 404)
	rows[0].Asset = "0x" + strings.Repeat("3", 40)
	call("POST", branchOne+"/publish", origin, publish, 400)
	rows[0].Asset = asset
	first := call("POST", branchOne+"/publish", origin, publish, 200)
	if first["version"] != float64(1) {
		t.Fatalf("wrong version %v", first)
	}
	call("POST", branchOne+"/publish", origin, publish, 200)
	publish["expiresAt"] = time.Now().Add(2 * time.Hour).UTC()
	call("POST", branchOne+"/publish", origin, publish, 409)
	stock := map[string]any{"requestKey": adjustKey, "version": 1, "sku": "tea", "delta": 2}
	for range 2 {
		changed := call("POST", branchOne+"/stock", origin, stock, 200)
		if changed["available"] != float64(2) {
			t.Fatalf("stock was repeated: %v", changed)
		}
	}
	execute("INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until) VALUES($1,$2,'customer',now()+interval '1 minute')", branchOne, customerA)
	purchaser := actor{customerA, "customer"}
	optionStock := map[string]any{"requestKey": adjustKey, "version": 1, "sku": "tea", "option": "hot", "delta": 1}
	once := call("POST", branchOne+"/option-stock", origin, optionStock, 200)
	repeated := call("POST", branchOne+"/option-stock", origin, optionStock, 200)
	if once["available"] != float64(1) || repeated["available"] != once["available"] {
		t.Fatal(once, repeated)
	}
	view, err := store.Catalog(ctx, authorize(t, purchaser, "catalog.read", branchOne, branchOne, "", map[string]string{"page": "0"}), 0)
	if err != nil || len(view) != 2 {
		t.Fatalf("customer menu: %+v %v", view, err)
	}
	for _, item := range view {
		if item.Metadata == nil || item.Metadata.Allergens == nil {
			t.Fatal("published menu declaration missing")
		}
		if item.Option == "hot" && (item.Metadata.Labels["en"] != "Tea" || item.Metadata.OptionLabels["ko"] != "따뜻함" ||
			(*item.Metadata.Allergens)[0] != "milk" || item.Metadata.CrossContact != "unknown") {
			t.Fatal(item)
		}
		if item.Option == "cold" && (len(*item.Metadata.Allergens) != 0 || item.Metadata.CrossContact != "possible") {
			t.Fatal(item)
		}
	}
	checkout := Cart{Origin: "menu", OriginRef: publishKey, MenuVersion: 1, Items: []Item{{SKU: "tea", Option: "hot", Qty: 1}}}
	request := authorize(t, purchaser, "order.create", branchOne, publishKey, adjustKey, map[string]string{
		"asset": asset, "quantity": "1", "intentDigest": strings.Repeat("a", 64),
	})
	purchase, err := store.Reserve(ctx, request, checkout)
	if err != nil {
		t.Fatal(err)
	}
	admitted := authorize(t, purchaser, "order.status", branchOne, purchase, "", map[string]string{})
	order, err := store.Status(ctx, admitted)
	if err != nil || order.Amount != "7000000" {
		t.Fatalf("sale does not use the published price: %v %v", order, err)
	}
	purchaserClient, otherCustomerClient := clientFor(purchaserCert), clientFor(otherCustomerCert)
	defer purchaserClient.CloseIdleConnections()
	defer otherCustomerClient.CloseIdleConnections()
	execute("INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until) VALUES($1,$2,'customer',now()+interval '2 minutes')", branchOne, customerB)

	const shopLabel = "여울 커피 성수점"
	execute("INSERT INTO ledger.operator_branch_label VALUES ($1,$2)", branchOne, shopLabel)
	readShop := func(client *http.Client, branch string, expected int) {
		t.Helper()
		command := service.Request{Version: 2, Contract: "C3", Operation: "catalog.read", Domain: branch,
			Object: branch, RequestID: other, ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{"page": "0"}}
		payload, err := json.Marshal(map[string]any{"command": command})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, "POST", server.URL+"/v2/commerce", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		reply, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer reply.Body.Close()
		body, err := io.ReadAll(reply.Body)
		if err != nil || reply.StatusCode != expected {
			t.Fatalf("shop discovery response %d, expected %d: %v", reply.StatusCode, expected, err)
		}
		if expected != 200 {
			if bytes.Contains(body, []byte(shopLabel)) || bytes.Contains(body, []byte(`"shop"`)) {
				t.Fatal("shop identity returned to a caller outside the enrolled branch")
			}
			return
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		var identity map[string]string
		if err := json.Unmarshal(fields["shop"], &identity); err != nil || len(identity) != 2 ||
			identity["branch"] != branch || identity["branchLabel"] != shopLabel {
			t.Fatalf("unexpected shop identity %v: %v", identity, err)
		}
	}
	readShop(purchaserClient, branchOne, 200)
	readShop(kioskClient, branchOne, 200)
	readShop(purchaserClient, branchTwo, 404)

	privateOrders := func(client *http.Client, domain, subject, after string, expected int) []CustomerOrderSummary {
		t.Helper()
		command := service.Request{Version: 2, Contract: "C3", Operation: "order.list", Domain: domain,
			Object: subject, RequestID: other, ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{"afterSale": after}}
		payload, marshalErr := json.Marshal(map[string]any{"command": command})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		req, requestErr := http.NewRequestWithContext(ctx, "POST", server.URL+"/v2/commerce", bytes.NewReader(payload))
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		reply, requestErr := client.Do(req)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		defer reply.Body.Close()
		if reply.StatusCode != expected {
			t.Fatalf("order list response %d; expected %d", reply.StatusCode, expected)
		}
		raw, readErr := io.ReadAll(reply.Body)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if expected != 200 {
			if bytes.Contains(raw, []byte(purchase)) || bytes.Contains(raw, []byte(`"orders"`)) {
				t.Fatal("denial disclosed order details")
			}
			return nil
		}
		var value result
		if decodeErr := json.Unmarshal(raw, &value); decodeErr != nil {
			t.Fatalf("order list response %d: %v", reply.StatusCode, decodeErr)
		}
		return value.Orders
	}
	if found := privateOrders(purchaserClient, branchOne, customerA, customerA, 200); len(found) != 1 || found[0].ID != purchase || found[0].Amount != "7000000" || found[0].AccessEndsAt != nil {
		t.Fatalf("private order list: %+v", found)
	}
	if found := privateOrders(otherCustomerClient, branchOne, customerB, customerB, 200); len(found) != 0 {
		t.Fatalf("foreign orders listed: %+v", found)
	}
	privateOrders(otherCustomerClient, branchOne, customerA, customerA, 404)
	privateOrders(purchaserClient, branchTwo, customerA, customerA, 404)
	privateOrders(kioskClient, branchOne, kiosk, kiosk, 404)

	execute(`INSERT INTO ledger.checkout_claim(sale_id,branch_id,actor_id,claim_digest,payer,payer_generation,payer_role,expires_at)
        VALUES($1,$2,$3,sha256('W09 claim'),decode(repeat('11',20),'hex'),1,1,now()+interval '1 minute')`, purchase, branchOne, customerB)
	if found := privateOrders(otherCustomerClient, branchOne, customerB, customerB, 200); len(found) != 1 || found[0].ID != purchase || found[0].AccessEndsAt == nil {
		t.Fatalf("authorized claimed order not listed: %+v", found)
	} else if currentOrderPage(found, *found[0].AccessEndsAt) != service.Forbidden {
		t.Fatal("claim expired while response waited for final scope validation")
	}
	// Slow only the disposable database reply after the actual product query has authorized and returned a real claim.
	execute("ALTER FUNCTION ledger.customer_order_rows(uuid,uuid,uuid,uuid) RENAME TO delayed_fixture_original")
	execute(`CREATE FUNCTION ledger.customer_order_rows(uuid,uuid,uuid,uuid)
      RETURNS TABLE(id uuid,state text,asset_ref text,amount text,access_ends_at timestamptz)
      LANGUAGE plpgsql SECURITY INVOKER SET search_path=pg_catalog,pg_temp AS $body$
      DECLARE observed int;
      BEGIN
        RETURN QUERY SELECT * FROM ledger.delayed_fixture_original($1,$2,$3,$4);
        GET DIAGNOSTICS observed = ROW_COUNT;
        IF observed<>1 THEN RAISE EXCEPTION 'expected one originally authorized sale'; END IF;
        PERFORM pg_sleep(1.8);
      END
      $body$`)
	execute("REVOKE ALL ON FUNCTION ledger.customer_order_rows(uuid,uuid,uuid,uuid) FROM PUBLIC")
	execute("GRANT EXECUTE ON FUNCTION ledger.customer_order_rows(uuid,uuid,uuid,uuid) TO nu54_p05_commerce")
	execute("UPDATE ledger.checkout_claim SET expires_at=clock_timestamp()+interval '1.4 seconds' WHERE sale_id=$1 AND actor_id=$2", purchase, customerB)
	began := time.Now()
	privateOrders(otherCustomerClient, branchOne, customerB, customerB, 404)
	if elapsed := time.Since(began); elapsed < 1750*time.Millisecond {
		t.Fatalf("expected post-query wait, observed %v", elapsed)
	} else {
		t.Logf("claim authorized at query, expired before TLS reply, withheld page after %v", elapsed)
	}
	execute("DROP FUNCTION ledger.customer_order_rows(uuid,uuid,uuid,uuid)")
	execute("ALTER FUNCTION ledger.delayed_fixture_original(uuid,uuid,uuid,uuid) RENAME TO customer_order_rows")
	if ownerRows := privateOrders(purchaserClient, branchOne, customerA, customerA, 200); len(ownerRows) != 1 || ownerRows[0].ID != purchase {
		t.Fatalf("original owner lost purchase")
	}
	execute("UPDATE ledger.checkout_claim SET expires_at=now()-interval '1 second' WHERE sale_id=$1 AND actor_id=$2", purchase, customerB)
	if found := privateOrders(otherCustomerClient, branchOne, customerB, customerB, 200); len(found) != 0 {
		t.Fatalf("expired claim leaked an order: %+v", found)
	}
	execute("UPDATE ledger.commerce_grant SET valid_until=now()-interval '1 second' WHERE branch_id=$1 AND actor_id=$2", branchOne, customerB)
	privateOrders(otherCustomerClient, branchOne, customerB, customerB, 404)

	execute(`WITH new_orders AS (
        INSERT INTO ledger.sale(branch_id,origin,origin_ref,asset_ref,amount)
          SELECT $1,'pos',gen_random_uuid(),$3,7 FROM generate_series(1,50) RETURNING id,branch_id
    ) INSERT INTO ledger.order_checkout(sale_id,branch_id,creator,expires_at,intent_digest)
      SELECT id,branch_id,$2,now()+interval '2 minutes',sha256('W09 order') FROM new_orders`, branchOne, customerA, asset)
	firstOrders := privateOrders(purchaserClient, branchOne, customerA, customerA, 200)
	if len(firstOrders) != 50 {
		t.Fatalf("first page length %d", len(firstOrders))
	}
	for index := 1; index < len(firstOrders); index++ {
		if firstOrders[index-1].ID >= firstOrders[index].ID {
			t.Fatal("unordered page")
		}
	}
	finalOrders := privateOrders(purchaserClient, branchOne, customerA, firstOrders[49].ID, 200)
	if len(finalOrders) != 1 || finalOrders[0].ID <= firstOrders[49].ID {
		t.Fatalf("bad final page %+v", finalOrders)
	}
	if end := privateOrders(purchaserClient, branchOne, customerA, finalOrders[0].ID, 200); len(end) != 0 {
		t.Fatalf("expected end %+v", end)
	}

	after, err := store.Catalog(ctx, authorize(t, purchaser, "catalog.read", branchOne, branchOne, "", map[string]string{"page": "0"}), 0)
	if err != nil || len(after) != 2 || after[0].Available != 1 || after[1].Available != 0 {
		t.Fatalf("option capacity: %v %v", after, err)
	}
	hotRetry := checkout
	hotRetry.OriginRef = operator
	retryRequest := authorize(t, purchaser, "order.create", branchOne, operator, publishKey, map[string]string{
		"asset": asset, "quantity": "1", "intentDigest": strings.Repeat("a", 64),
	})
	if _, err := store.Reserve(ctx, retryRequest, hotRetry); !errors.Is(err, ErrStock) {
		t.Fatalf("sold-out option accepted: %v", err)
	}
	t.Run("OriginalMenuDenialSurvivesRestock", func(t *testing.T) {
		var up, down string
		if err := admin.QueryRow(ctx, "SELECT gen_random_uuid()::text,gen_random_uuid()::text").Scan(&up, &down); err != nil {
			t.Fatal(err)
		}
		optionStock["requestKey"] = up
		call("POST", branchOne+"/option-stock", origin, optionStock, 200)
		reopened, err := Open(ctx, config.ConnString()+" user="+role)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		sale, err := reopened.Reserve(ctx, retryRequest, hotRetry)
		var rejection *reservationRejection
		if sale != "" || !errors.As(err, &rejection) || rejection.code != service.OutOfStock ||
			len(rejection.lines) != 1 || rejection.lines[0] != (Item{"tea", "hot", 1}) {
			t.Fatalf("original sold-out receipt changed after restock: %s %#v %v", sale, rejection, err)
		}
		changed := hotRetry
		changed.Items = []Item{{"tea", "cold", 1}}
		if _, err := reopened.Reserve(ctx, retryRequest, changed); !errors.Is(err, service.Conflict) {
			t.Fatalf("modified request replay: %v", err)
		}
		execute("UPDATE ledger.menu_reservation_outcome SET retire_after=now()-interval '1 second' WHERE actor_id=$1 AND request_key=$2", customerA, publishKey)
		if err := reopened.RetireTerminalReceipts(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := reopened.Reserve(ctx, retryRequest, hotRetry); !errors.Is(err, service.Expired) {
			t.Fatalf("expired receipt replayed the cart: %v", err)
		}
		optionStock["requestKey"], optionStock["delta"] = down, -1
		call("POST", branchOne+"/option-stock", origin, optionStock, 200)
		optionStock["delta"] = 1
	})
	if installedRunner != "" {
		t.Run("InstalledK10VirtualNearby", func(t *testing.T) {
			menuKioskClient := clientFor(menuKioskCert)
			defer menuKioskClient.CloseIdleConnections()
			fixture := installedMenuFixture{admin: admin, server: server, terminal: menuKioskClient, credentials: handler.Credentials,
				ca: cert, issuer: key, branch: branchOne, kiosk: menuKiosk, runner: installedRunner, signedMenu: signedMenu, ownerBoard: ownerBoard,
				moveHot: func(delta int) {
					change := map[string]any{"requestKey": uuidForCatalogFixture(t, ctx, admin), "version": 1, "sku": "tea", "option": "hot", "delta": delta}
					call("POST", branchOne+"/option-stock", origin, change, 200)
				},
			}
			if settlement != nil {
				fixture.finalizeOriginal = settlement.FinalizeOriginal
				fixture.expireOriginal = settlement.ExpireOriginal
				fixture.finance, fixture.financeOrigin = makerClient, origin
				fixture.financeCertificate = makerCert
			}
			if settlement != nil && settlement.DispatchOriginal != nil {
				dispatchClient := clientFor(dispatchCert)
				defer dispatchClient.CloseIdleConnections()
				execute("UPDATE ledger.finance_grant SET valid_until=now()+interval '8 minutes' WHERE branch_id=$1 AND actor_id=$2", branchOne, clerk)
				execute("UPDATE ledger.commerce_grant SET valid_until=now()+interval '8 minutes' WHERE branch_id=$1 AND actor_id=$2 AND role_code='customer'", branchOne, customerB)
				fixture.readyOriginal = installedPickupReady(admin, fixture.branch, server.URL, clerkClient, otherCustomerClient,
					func(ctx context.Context, sale string) error {
						return settlement.DispatchOriginal(ctx, dispatchClient, server.URL, sale)
					})
			}

			if renterProof != nil {
				fixture.runRenter(t, ctx, store, settlement, renterProof)
			} else {
				fixture.run(t, ctx)
			}
		})
		return
	}
	t.Run("NearbyPrivateReceiptAndPublicCheckout", func(t *testing.T) {
		menuKioskClient := clientFor(menuKioskCert)
		defer menuKioskClient.CloseIdleConnections()
		type arrival struct{ customer, origin, key string }
		var arrivals []arrival
		generated, err := admin.Query(ctx, `WITH buyers AS (
			INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until)
			SELECT $1,gen_random_uuid(),'customer',now()+interval '7 minutes' FROM generate_series(1,100)
			RETURNING actor_id) SELECT actor_id::text,gen_random_uuid()::text,gen_random_uuid()::text FROM buyers`, branchOne)
		if err != nil {
			t.Fatal(err)
		}
		for generated.Next() {
			var buyer arrival
			if err := generated.Scan(&buyer.customer, &buyer.origin, &buyer.key); err != nil {
				t.Fatal(err)
			}
			arrivals = append(arrivals, buyer)
		}
		if len(arrivals) != 100 || generated.Err() != nil {
			t.Fatal("could not provision 100 scoped buyers", generated.Err())
		}
		newRef := func() string {
			t.Helper()
			var ref string
			if err := admin.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&ref); err != nil {
				t.Fatal(err)
			}
			return ref
		}
		commerce := func(operation, target, key string, fields map[string]string, cart *Cart, expected int) result {
			t.Helper()
			var nonce [16]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				t.Fatal(err)
			}
			nonce[6] = nonce[6]&0x0f | 0x40
			nonce[8] = nonce[8]&0x3f | 0x80
			messageID := fmt.Sprintf("%x-%x-%x-%x-%x", nonce[:4], nonce[4:6], nonce[6:8], nonce[8:10], nonce[10:])
			command := service.Request{Version: 2, Contract: "C3", Operation: operation, Domain: branchOne,
				Object: target, RequestID: messageID, ExpiresAt: time.Now().Unix() + 20, IdempotencyKey: key, Body: fields}
			blob, err := json.Marshal(struct {
				Command service.Request `json:"command"`
				Cart    *Cart           `json:"cart,omitempty"`
			}{command, cart})
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequestWithContext(ctx, "POST", server.URL+"/v2/commerce", bytes.NewReader(blob))
			if err != nil {
				t.Fatal(err)
			}
			reply, err := menuKioskClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer reply.Body.Close()
			var response result
			readErr := json.NewDecoder(reply.Body).Decode(&response)
			if reply.StatusCode != expected || expected != 404 && readErr != nil {
				t.Fatalf("%s returned %d (expected %d), error=%s read=%v", operation, reply.StatusCode, expected, response.Error, readErr)
			}
			if expected != 404 {
				scope := service.Principal{Tenant: tenant, Subject: menuKiosk, Role: "terminal", ExpiresAt: time.Now().Unix() + 25,
					Grants: []service.Grant{{Domain: branchOne, Object: target, Operations: []string{"C3/" + operation}}}}
				bound, err := service.Authorize(command, scope, time.Now())
				if err != nil || service.ValidateReply(bound, response.Reply, time.Now()) != nil {
					t.Fatalf("mTLS commerce receipt is unbound: %v", err)
				}
			}
			return response
		}
		reserve := func(cart Cart, key string, expected int) result {
			t.Helper()
			raw, err := json.Marshal(cart)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(raw)
			return commerce("order.create", cart.OriginRef, key, map[string]string{
				"asset": asset, "quantity": "1", "intentDigest": hex.EncodeToString(digest[:]),
			}, &cart, expected)
		}
		session, publicKey := newRef(), newRef()
		execute("UPDATE ledger.command_rule SET receipt_window=interval '5 minutes' WHERE branch_id=$1", branchOne)
		defer execute("UPDATE ledger.command_rule SET receipt_window=interval '2 minutes' WHERE branch_id=$1", branchOne)
		execute("UPDATE ledger.stock SET available=available+1 WHERE branch_id=$1 AND sku='tea'", branchOne)
		started := time.Now()
		public := reserve(Cart{Origin: "menu", OriginRef: session, MenuVersion: 1, Items: []Item{{"tea", "cold", 1}}}, publicKey, 200)
		deadline := time.Now().Unix() + 295
		var concurrent sync.WaitGroup
		permits := make(chan struct{}, 8)
		for n, buyer := range arrivals {
			permits <- struct{}{}
			concurrent.Go(func() {
				defer func() { <-permits }()
				cart := Cart{Origin: "menu", OriginRef: buyer.origin, MenuVersion: 1, Items: []Item{{"tea", "hot", 1}},
					Nearby: &NearbyReservation{Customer: buyer.customer, ExpiresAt: deadline}}
				if n%2 == 1 {
					cart.MenuVersion = 2
				}
				denied := reserve(cart, buyer.key, 409)
				if !denied.ReservationFound ||
					n%2 == 0 && (denied.Error != service.OutOfStock || len(denied.Unavailable) != 1 ||
						denied.Unavailable[0] != (Item{"tea", "hot", 1})) ||
					n%2 == 1 && (denied.Error != service.CatalogChanged || len(denied.Unavailable) != 0) {
					t.Fatalf("wrong nearby receipt: %+v", denied)
				}
				if n%10 == 0 {
					publicDuring := commerce("order.status", public.ResultRef, "", map[string]string{}, nil, 200)
					if publicDuring.Order == nil || publicDuring.Order.State != "preview" {
						t.Fatal("private request disturbed the concurrent public cart", publicDuring)
					}
				}
			})
		}
		concurrent.Wait()
		first := arrivals[0]
		lookup := func(customer, key, byOrder string, expected int) result {
			return commerce("order.nearby-result", key, "", map[string]string{"customerRef": customer, "byOrder": byOrder}, nil, expected)
		}
		if lookup(arrivals[1].customer, first.key, "0", 200).ReservationFound {
			t.Fatal("foreign peer learned another reservation")
		}
		if got := lookup(first.customer, first.key, "0", 409); got.Error != service.OutOfStock || len(got.Unavailable) != 1 {
			t.Fatal("same peer could not recover the saved denial", got)
		}
		var sales, stockDenials, priceDenials, payments int
		if err := admin.QueryRow(ctx, `SELECT
		  (SELECT count(*) FROM ledger.sale WHERE branch_id=$1 AND origin='menu' AND id=$2),
		  (SELECT count(*) FROM ledger.menu_reservation_outcome WHERE branch_id=$1 AND nearby_customer IS NOT NULL AND rejection='OUT_OF_STOCK'),
		  (SELECT count(*) FROM ledger.menu_reservation_outcome WHERE branch_id=$1 AND nearby_customer IS NOT NULL AND rejection='CATALOG_CHANGED'),
		  (SELECT count(*) FROM ledger.quote_work WHERE sale_id=$2)`, branchOne, public.ResultRef).
			Scan(&sales, &stockDenials, &priceDenials, &payments); err != nil || sales != 1 || stockDenials != 50 || priceDenials != 50 || payments != 0 {
			t.Fatalf("parallel private/public ledger mismatch %d %d %d %d %v", sales, stockDenials, priceDenials, payments, err)
		}
		if status := commerce("order.status", public.ResultRef, "", map[string]string{}, nil, 200); status.Order == nil || status.Order.State != "preview" {
			t.Fatal("private requests disturbed the public cart", status)
		} else if remaining := time.Until(status.Order.Expires); remaining < 4*time.Minute || remaining > 5*time.Minute {
			t.Fatalf("public hold was not fenced by its configured five-minute receipt window: %v", remaining)
		}
		if liveHold {
			waitUntil := func(elapsed time.Duration) {
				t.Helper()
				timer := time.NewTimer(time.Until(started.Add(elapsed)))
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			for _, elapsed := range []time.Duration{30, 60, 90, 120, 150, 180, 210, 240, 270} {
				waitUntil(elapsed * time.Second)
				status := commerce("order.status", public.ResultRef, "", map[string]string{}, nil, 200)
				if status.Order == nil || status.Order.State != "preview" || status.Order.ID != public.ResultRef {
					t.Fatal("same public sale disappeared before the hold elapsed", status)
				}
				if elapsed == 150 {
					stockUpdate := map[string]any{"requestKey": newRef(), "version": 1, "sku": "tea", "option": "hot", "delta": 1}
					call("POST", branchOne+"/option-stock", origin, stockUpdate, 200)
					denied := lookup(first.customer, first.key, "0", 409)
					if denied.Error != service.OutOfStock || len(denied.Unavailable) != 1 {
						t.Fatal("replenishing stock changed the original receipt", denied)
					}
					nextBuyer := arrivals[1]
					fresh := Cart{Origin: "menu", OriginRef: newRef(), MenuVersion: 1, Items: []Item{{"tea", "hot", 1}},
						Nearby: &NearbyReservation{Customer: nextBuyer.customer, ExpiresAt: time.Now().Unix() + 120}}
					freshKey := newRef()
					received := reserve(fresh, freshKey, 200)
					if again := lookup(nextBuyer.customer, freshKey, "0", 200); !again.ReservationFound || again.ResultRef != received.ResultRef {
						t.Fatal("another customer could not recover their order while the public sale was open", again)
					}
					t.Logf("elapsed %.0fs: replenishment preserved the original denial and another customer created one independent sale", time.Since(started).Seconds())
				}
			}
			waitUntil(301 * time.Second)
			var elapsed, publicCount, privateCount, quoteCount int
			var publicSaleExpired bool
			if err := admin.QueryRow(ctx, `SELECT extract(epoch FROM (clock_timestamp()-r.created_at))::integer,
				(SELECT count(*) FROM ledger.menu_reservation_outcome WHERE branch_id=$1 AND sale_id=$2),
				(SELECT count(*) FROM ledger.menu_reservation_outcome WHERE branch_id=$1 AND nearby_customer IS NOT NULL AND sale_id IS NOT NULL),
				(SELECT count(*) FROM ledger.quote_work qw JOIN ledger.menu_reservation_outcome q ON q.sale_id=qw.sale_id WHERE q.branch_id=$1),
				c.expires_at<=clock_timestamp()
				FROM ledger.menu_reservation_outcome r JOIN ledger.order_checkout c ON c.sale_id=r.sale_id
				WHERE r.branch_id=$1 AND r.sale_id=$2`, branchOne, public.ResultRef).
				Scan(&elapsed, &publicCount, &privateCount, &quoteCount, &publicSaleExpired); err != nil ||
				elapsed < 300 || publicCount != 1 || privateCount != 1 || quoteCount != 0 || !publicSaleExpired {
				t.Fatalf("five-minute ledger mismatch elapsed=%d public=%d private=%d quotes=%d expired=%t err=%v",
					elapsed, publicCount, privateCount, quoteCount, publicSaleExpired, err)
			}
			commerce("order.cancel-public", session, publicKey, map[string]string{}, nil, 200)
			t.Logf("elapsed %ds in PostgreSQL: public sales=%d private sales=%d quote writes=%d", elapsed, publicCount, privateCount, quoteCount)
			return
		}
		commerce("order.cancel-public", session, publicKey, map[string]string{}, nil, 200)

		legacy := Cart{Origin: "menu", OriginRef: newRef(), MenuVersion: 1, Items: []Item{{"tea", "cold", 1}}}
		legacyKey := newRef()
		var legacySale string
		if err := admin.QueryRow(ctx, `SELECT ledger.reserve_order($1,$2,$3,$4,'menu',$5,1,$6,
			'[{"sku":"tea","option_key":"cold","quantity":1}]','terminal')::text`,
			tenant, branchOne, menuKiosk, legacyKey, legacy.OriginRef, asset).Scan(&legacySale); err != nil {
			t.Fatal(err)
		}
		retagged := legacy
		retagged.Nearby = &NearbyReservation{Customer: first.customer, ExpiresAt: deadline}
		reserve(retagged, legacyKey, 404)
		if replay := reserve(legacy, legacyKey, 200); replay.ResultRef != legacySale {
			t.Fatal("legacy public cart lost its exact original sale", replay)
		}
		commerce("order.cancel-public", legacy.OriginRef, legacyKey, map[string]string{}, nil, 200)

		ready := Cart{Origin: "menu", OriginRef: newRef(), MenuVersion: 1, Items: []Item{{"tea", "cold", 1}},
			Nearby: &NearbyReservation{Customer: first.customer, ExpiresAt: deadline}}
		key := newRef()
		accepted := reserve(ready, key, 200)
		if replay := reserve(ready, key, 200); replay.ResultRef != accepted.ResultRef {
			t.Fatal("replayed successful cart allocated another sale", replay)
		}
		if got := lookup(first.customer, key, "0", 200); !got.ReservationFound || got.ResultRef != accepted.ResultRef {
			t.Fatal("lost response could not recover server-owned sale", got)
		}
		if got := lookup(arrivals[1].customer, accepted.ResultRef, "1", 200); got.ReservationFound {
			t.Fatal("foreign nearby peer identified the successful sale", got)
		}
		commerce("order.cancel-public", ready.OriginRef, key, map[string]string{}, nil, 404)
		commerce("checkout.authorize", accepted.ResultRef, key, map[string]string{"customerRef": arrivals[1].customer}, nil, 404)
		execute("UPDATE ledger.order_checkout SET expires_at=now()-interval '1 second' WHERE sale_id=$1", accepted.ResultRef)
		if reclaimed, err := store.ReclaimExpired(ctx); err != nil || !reclaimed {
			t.Fatalf("nearby test stock did not reclaim: %t %v", reclaimed, err)
		}
		execute("UPDATE ledger.stock SET available=available-1 WHERE branch_id=$1 AND sku='tea'", branchOne)
	})
	if liveHold {
		return
	}
	catalog := call("GET", branchOne, origin, nil, 200)["catalog"].(map[string]any)
	for _, row := range catalog["rows"].([]any) {
		if row.(map[string]any)["available"] != float64(1) {
			t.Fatal("available inventory did not track the reservation")
		}
	}
	stock["requestKey"] = publishKey
	stock["delta"] = -3
	call("POST", branchOne+"/stock", origin, stock, 409)
	// Choose a closed interval far from this run's known time. Both versions
	// remain within their publication validity, which isolates the new guards.
	sealed := &operatorHours{Opens: "00:00", Closes: "01:00", Zone: "UTC"}
	if time.Now().UTC().Hour() < 2 {
		sealed.Opens, sealed.Closes = "12:00", "13:00"
	}
	publish["requestKey"], publish["version"], publish["hours"] = adjustKey, 1, sealed
	rows[0].Option = "standard"
	call("POST", branchOne+"/publish", origin, publish, 400)
	rows[0].Option = "hot"
	second := call("POST", branchOne+"/publish", origin, publish, 200)
	if second["version"] != float64(2) {
		t.Fatal(second)
	}
	sealed.Closes = "14:00"
	call("POST", branchOne+"/publish", origin, publish, 409)
	prices, err := store.Catalog(ctx, authorize(t, purchaser, "catalog.read", branchOne, branchOne, "", map[string]string{"page": "0"}), 0)
	if err != nil || len(prices) != 0 {
		t.Fatalf("closed menu available: %v %v", prices, err)
	}
	if resumed, err := store.Reserve(ctx, request, checkout); err != nil || resumed != purchase {
		t.Fatalf("lost known reservation on publication: %s %v", resumed, err)
	}
	newCheckout := checkout
	for _, version := range []int64{1, 2} {
		var key string
		if err := admin.QueryRow(ctx, "SELECT gen_random_uuid()::text,gen_random_uuid()::text").Scan(&newCheckout.OriginRef, &key); err != nil {
			t.Fatal(err)
		}
		newRequest := authorize(t, purchaser, "order.create", branchOne, newCheckout.OriginRef, key, map[string]string{
			"asset": asset, "quantity": "1", "intentDigest": strings.Repeat("a", 64),
		})
		newCheckout.MenuVersion = version
		if _, err := store.Reserve(ctx, newRequest, newCheckout); !errors.Is(err, service.CatalogChanged) {
			t.Fatalf("accepted reservation on superseded or closed version %d: %v", version, err)
		}
	}

	exerciseFirmwareHTTP(t, execute, call, branchOne, branchTwo, operator, origin, handler.Firmware[strings.Repeat("a", 64)])

	exerciseRentalHTTP(t, execute, call, branchOne, branchTwo, operator, origin, rental)
	exerciseExperienceHTTP(t, execute, call, branchOne, branchTwo, operator, customerA, origin, asset)
	if browserScript != "" && os.Getenv("NU54_CATALOG_K07_ONLY") != "1" {
		t.Run("IndependentBrowserPendingScopes", func(t *testing.T) {
			execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null)", branchTwo, operator)
			execute("INSERT INTO ledger.command_rule VALUES ($1,'catalog test','fixture','dispose',interval '2 minutes')", branchTwo)
			var branchTwoKey string
			if err := admin.QueryRow(ctx, "SELECT gen_random_uuid()::text").Scan(&branchTwoKey); err != nil {
				t.Fatal(err)
			}
			call("POST", branchTwo+"/publish", origin, map[string]any{
				"requestKey": branchTwoKey, "version": 0, "expiresAt": time.Now().Add(time.Hour).UTC(), "rows": rows,
			}, 200)
			dir := t.TempDir()
			private, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
			for _, part := range []struct {
				path, label string
				raw         []byte
			}{
				{certPath, "CERTIFICATE", der}, {keyPath, "PRIVATE KEY", private},
			} {
				if err := os.WriteFile(part.path, pem.EncodeToMemory(&pem.Block{Type: part.label, Bytes: part.raw}), 0600); err != nil {
					t.Fatal(err)
				}
			}
			browser := exec.CommandContext(ctx, "node", browserScript, origin, branchOne, branchTwo, certPath, keyPath)
			if out, err := browser.CombinedOutput(); err != nil {
				t.Fatalf("catalog Chrome: %v\n%s", err, out)
			} else {
				t.Logf("catalog Chrome: %s", out)
			}
			a := call("GET", branchOne, origin, nil, 200)["catalog"].(map[string]any)
			b := call("GET", branchTwo, origin, nil, 200)["catalog"].(map[string]any)
			if a["version"] != float64(3) || b["version"] != float64(2) {
				t.Fatalf("branches independently publish one revision: %v and %v", a["version"], b["version"])
			}
			if original, err := store.Status(ctx, admitted); err != nil || original.Amount != "7000000" {
				t.Fatal("original confirmed quote price changed")
			}
		})
	}

	t.Run("PublishedK07Menu", func(t *testing.T) {
		raw, err := os.ReadFile("../../../p04-merchant-kiosk/test/fixtures/k07-publication.json")
		if err != nil {
			t.Fatal(err)
		}
		var fixture struct {
			Rows  []operatorRow
			Stock int64
		}
		if err := json.Unmarshal(raw, &fixture); err != nil || len(fixture.Rows) != 3 {
			t.Fatalf("invalid K07 fixture: %v", err)
		}
		publicationKey := uuidForCatalogFixture(t, ctx, admin)
		current := call("GET", branchOne, origin, nil, 200)["catalog"].(map[string]any)["version"]
		for i := range fixture.Rows {
			fixture.Rows[i].Asset = asset
			if browserScript != "" {
				fixture.Rows[i].Metadata.OptionGroupLabels = nil
				fixture.Rows[i].Metadata.CrossContactAllergens = nil
				fixture.Rows[i].Metadata.CrossContactSource = ""
			}
		}
		published := call("POST", branchOne+"/publish", origin, map[string]any{
			"requestKey": publicationKey, "version": current, "expiresAt": time.Now().Add(time.Hour).UTC(),
			"rows": fixture.Rows,
		}, 200)
		skus := map[string]bool{}
		for _, row := range fixture.Rows {
			if !skus[row.SKU] {
				call("POST", branchOne+"/stock", origin, map[string]any{
					"requestKey": uuidForCatalogFixture(t, ctx, admin), "version": published["version"],
					"sku": row.SKU, "delta": fixture.Stock,
				}, 200)
				skus[row.SKU] = true
			}
		}
		if browserScript != "" {
			dir := t.TempDir()
			private, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
			if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0600); err != nil {
				t.Fatal(err)
			}
			browser := exec.CommandContext(ctx, "node", browserScript, origin, branchOne, branchTwo, certPath, keyPath, "k07")
			if out, err := browser.CombinedOutput(); err != nil {
				t.Fatalf("K07 O03 Chrome: %v\n%s", err, out)
			} else {
				t.Logf("K07 O03 Chrome: %s", out)
			}
			var oldFields int
			if err := admin.QueryRow(ctx,
				`SELECT count(*) FROM ledger.catalog_pricing p CROSS JOIN jsonb_array_elements(p.breakdown) AS entry
				 WHERE branch_id=$1 AND version=$2 AND
				 (entry->'metadata') ?| ARRAY['optionGroupLabels','crossContactSource','crossContactAllergens']`,
				branchOne, int64(published["version"].(float64))).Scan(&oldFields); err != nil || oldFields != 0 {
				t.Fatalf("prior publication changed: %d %v", oldFields, err)
			}
		}
		optionLimit := fixture.Stock - 2
		activeVersion := call("GET", branchOne, origin, nil, 200)["catalog"].(map[string]any)["version"]
		call("POST", branchOne+"/option-stock", origin, map[string]any{
			"requestKey": uuidForCatalogFixture(t, ctx, admin), "version": activeVersion,
			"sku": "drink-12", "option": "iced", "delta": optionLimit,
		}, 200)
		wire, err := json.Marshal(map[string]any{"command": service.Request{
			Version: 2, Contract: "C3", Operation: "catalog.read", Domain: branchOne, Object: branchOne,
			RequestID: publicationKey, ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{"page": "0"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		response, err := kioskClient.Post(server.URL+"/v2/commerce", "application/json", bytes.NewReader(wire))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var menu result
		if err = json.NewDecoder(response.Body).Decode(&menu); response.StatusCode != 200 || err != nil || len(menu.Catalog) != len(fixture.Rows) {
			t.Fatalf("terminal menu from publication: status %d, rows %d, error %v", response.StatusCode, len(menu.Catalog), err)
		}
		for _, row := range menu.Catalog {
			wantAvailable := fixture.Stock
			if row.SKU == "drink-12" && row.Option == "iced" {
				wantAvailable = optionLimit
			}
			if row.Available != wantAvailable || row.SKUAvailable != fixture.Stock || row.Metadata == nil {
				t.Fatalf("published row: %+v", row)
			}
			switch row.SKU {
			case "baked-09":
				if row.Metadata.Labels["ko"] != "크루아상" || row.Metadata.OptionRequired ||
					row.UnitPrice != "3000000" || row.Metadata.Allergens == nil ||
					strings.Join(*row.Metadata.Allergens, ",") != "wheat,milk,egg" || row.Metadata.CrossContact != "possible" ||
					row.Metadata.CrossContactAllergens == nil || strings.Join(*row.Metadata.CrossContactAllergens, ",") != "tree-nuts" ||
					row.Metadata.CrossContactSource != "shared-baking-equipment" {
					t.Fatalf("bakery disclosure or price differs: %+v", row)
				}
			case "drink-12":
				if !row.Metadata.OptionRequired || (row.Option != "iced" && row.Option != "hot") ||
					row.Metadata.OptionGroupLabels["ko"] != "온도" || row.Metadata.OptionGroupLabels["en"] != "Temperature" {
					t.Fatalf("required option not published: %+v", row)
				}
			default:
				t.Fatalf("unexpected publication row: %+v", row)
			}
		}
		if original, err := store.Status(ctx, admitted); err != nil || original.Amount != "7000000" {
			t.Fatalf("another customer's original order changed: %+v %v", original, err)
		}
	})

	t.Run("PublishedCategories", func(t *testing.T) {
		catalog := func(branch string) map[string]any {
			return call("GET", branch, origin, nil, 200)["catalog"].(map[string]any)
		}
		execute("INSERT INTO ledger.staff_grant VALUES ($1,$2,'operator',now()-interval '1 minute',null) ON CONFLICT DO NOTHING", branchTwo, operator)
		execute("INSERT INTO ledger.command_rule VALUES ($1,'catalog test','fixture','dispose',interval '2 minutes') ON CONFLICT DO NOTHING", branchTwo)
		execute("INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until) VALUES($1,$2,'customer',now()+interval '1 minute') ON CONFLICT DO NOTHING", branchTwo, customerA)
		publishAt := func(branch string, rows []operatorRow, categories []CatalogCategory, bindings []categoryBinding) map[string]any {
			return map[string]any{"requestKey": uuidForCatalogFixture(t, ctx, admin), "version": catalog(branch)["version"],
				"expiresAt": time.Now().Add(time.Hour).UTC(), "rows": rows, "categories": categories, "categoryBindings": bindings}
		}
		category := func(name string) CatalogCategory {
			return CatalogCategory{Key: "coffee", SortOrder: 2, Labels: map[string]string{"ko": "커피", "en": name}}
		}
		firstRows := make([]operatorRow, 52)
		for i := range firstRows {
			firstRows[i] = operatorRow{SKU: fmt.Sprintf("category-%02d", i), Option: "plain", Label: "Drink", Asset: asset, Price: "11"}
		}
		otherRows := []operatorRow{{SKU: "foreign-drink", Option: "plain", Label: "Other", Asset: asset, Price: "21"}}
		keys := []CatalogCategory{category("Branch one"), {Key: "bakery", SortOrder: 2, Labels: map[string]string{"ko": "빵", "en": "Bakery"}}}
		bindings := []categoryBinding{{SKU: "category-51", CategoryKey: "coffee"}, {SKU: "category-00", CategoryKey: "bakery"}}
		first := publishAt(branchOne, firstRows, keys, bindings)
		second := publishAt(branchTwo, otherRows, []CatalogCategory{category("Branch two")}, []categoryBinding{{SKU: "foreign-drink", CategoryKey: "coffee"}})
		call("POST", branchTwo+"/publish", origin, second, 200)
		const badKey = "does-not-exist"
		for _, invalid := range [][]categoryBinding{
			{{SKU: "category-51", CategoryKey: badKey}}, {{SKU: "foreign-drink", CategoryKey: "coffee"}},
			{{SKU: "category-51", CategoryKey: "coffee"}, {SKU: "category-51", CategoryKey: "bakery"}},
		} {
			first["categoryBindings"] = invalid
			call("POST", branchOne+"/publish", origin, first, 400)
		}
		first["categoryBindings"] = bindings
		first["categories"] = []CatalogCategory{keys[0], keys[0]}
		call("POST", branchOne+"/publish", origin, first, 400)
		first["categories"] = keys
		if catalog(branchOne)["version"] != first["version"] {
			t.Fatal("invalid publication advanced release")
		}
		published := call("POST", branchOne+"/publish", origin, first, 200)["version"]
		first["categories"] = []CatalogCategory{keys[1], keys[0]}
		first["categoryBindings"] = []categoryBinding{bindings[1], bindings[0]}
		if got := call("POST", branchOne+"/publish", origin, first, 200)["version"]; got != published {
			t.Fatal("retry advanced publication")
		}
		first["categories"] = []CatalogCategory{category("Reassigned"), keys[1]}
		call("POST", branchOne+"/publish", origin, first, 409)
		home := catalog(branchOne)
		if home["version"] != published || home["categories"].([]any)[0].(map[string]any)["key"] != "bakery" ||
			len(home["categoryBindings"].([]any)) != 2 {
			t.Fatalf("failed publication order or replay: %v", home)
		}
		window := func(branch string, page int32) ([]Price, *int64) {
			t.Helper()
			auth := authorize(t, purchaser, "catalog.read", branch, branch, "", map[string]string{"page": fmt.Sprint(page)})
			rows, version, err := store.catalogWindow(ctx, auth, page)
			if err != nil {
				t.Fatal(err)
			}
			return rows, version
		}
		before, _ := window(branchOne, 0)
		tail, version := window(branchOne, 1)
		empty, endVersion := window(branchOne, 2)
		other, _ := window(branchTwo, 0)
		if len(before) != 50 || len(tail) != 2 || len(empty) != 0 || version == nil || endVersion == nil || *version != *endVersion ||
			tail[0].Category != nil || tail[1].Category == nil || tail[1].Category.Labels["en"] != "Branch one" ||
			len(other) != 1 || other[0].Category == nil || other[0].Category.Labels["en"] != "Branch two" {
			t.Fatalf("category isolation or pagination failed: page 0=%d, tail=%+v, branch two=%+v", len(before), tail, other)
		}
		call("POST", branchOne+"/stock", origin, map[string]any{"requestKey": uuidForCatalogFixture(t, ctx, admin), "version": published, "sku": "category-50", "delta": 1}, 200)
		orderRef := uuidForCatalogFixture(t, ctx, admin)
		cart := Cart{Origin: "menu", OriginRef: orderRef, MenuVersion: *version, Items: []Item{{SKU: "category-50", Option: "plain", Qty: 1}}}
		auth := authorize(t, purchaser, "order.create", branchOne, orderRef, uuidForCatalogFixture(t, ctx, admin), map[string]string{
			"asset": asset, "quantity": "1", "intentDigest": strings.Repeat("a", 64),
		})
		sale, err := store.Reserve(ctx, auth, cart)
		if err != nil {
			t.Fatal(err)
		}
		var total string
		if err := admin.QueryRow(ctx, "SELECT amount::text FROM ledger.sale WHERE id=$1", sale).Scan(&total); err != nil || total != "11" {
			t.Fatalf("category changed unclassified order pricing: %s %v", total, err)
		}
	})

	execute("UPDATE ledger.staff_grant SET revoked_at=now() WHERE branch_id=$1 AND subject_ref=$2", branchOne, operator)
	call("GET", branchOne, origin, nil, 404)
	call("POST", branchOne+"/stock", origin, stock, 404)
}

func exerciseKioskStaffRealTLS(t *testing.T, ctx context.Context, base string, admin, kiosk, clerk, maker *http.Client,
	branch, otherBranch, adminID, kioskID, clerkID, makerID, console string, execute func(string, ...any)) {
	t.Helper()
	send := func(client *http.Client, fields map[string]any, expected int) result {
		t.Helper()
		raw, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, "POST", base+"/v2/commerce", strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != expected {
			t.Fatalf("got HTTP %d, want %d for %s", resp.StatusCode, expected, raw)
		}
		var out result
		if expected == 200 {
			if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	access := func(client *http.Client, domain, target string, expected int) *StaffAccess {
		return send(client, map[string]any{"command": service.Request{Version: 2, Contract: "C3", Operation: "staff.access",
			Domain: domain, Object: target, RequestID: other, ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{}}}, expected).Staff
	}
	expect := func(client *http.Client, who, permission, origin string) {
		t.Helper()
		value := access(client, branch, who, 200)
		if value == nil || value.BranchRef != branch || value.ActorRef != who || value.Console != origin ||
			len(value.Permissions) != 1 || value.Permissions[0] != permission {
			t.Fatalf("authorization %+v", value)
		}
	}
	expect(admin, adminID, "terminal_settings", "")
	expect(clerk, clerkID, "pickup", "")
	expect(maker, makerID, "refund_console", console)
	access(clerk, otherBranch, clerkID, 404)
	access(clerk, branch, adminID, 404)
	access(kiosk, branch, kioskID, 404)
	execute("INSERT INTO ledger.travel_pickup_terminal VALUES ($1,$2)", branch, kioskID)
	expect(kiosk, kioskID, "pickup", "")
	send(kiosk, map[string]any{"travel": map[string]string{"operation": "pickup-queue", "branch": branch}}, 200)
	send(kiosk, map[string]any{"travel": map[string]any{"operation": "pickup-queue", "branch": branch, "includeCafe": true}}, 404)
	execute("DELETE FROM ledger.travel_pickup_terminal WHERE branch_id=$1 AND actor_id=$2", branch, kioskID)
	queue := func(client *http.Client, domain string, expected int) {
		send(client, map[string]any{"travel": map[string]string{"operation": "pickup-queue", "branch": domain}}, expected)
	}
	queue(clerk, branch, 200)
	queue(clerk, otherBranch, 404)
	queue(admin, branch, 404)
	queue(maker, branch, 404)
	cancellation := service.Request{Version: 2, Contract: "C3", Operation: "order.cancel-public",
		Domain: branch, Object: "b689fbb7-26d8-47ed-a64d-ec515962b504", RequestID: other,
		IdempotencyKey: "cc119c10-ed9a-4ad8-9b75-0bbd25724f36", ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{}}
	send(clerk, map[string]any{"command": cancellation}, 404)
	for range 2 {
		if reply := send(kiosk, map[string]any{"command": cancellation}, 200); reply.ResultRef != cancellation.Object {
			t.Fatal("terminal mTLS cancel did not acknowledge the exact session")
		}
	}
	send(clerk, map[string]any{"command": service.Request{Version: 2, Contract: "C3", Operation: "terminal.state",
		Domain: branch, Object: kioskID, RequestID: other, ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{}}}, 404)
	device := func(client *http.Client, expected int) *TerminalDevice {
		return send(client, map[string]any{"command": service.Request{Version: 2, Contract: "C3", Operation: "terminal.device",
			Domain: branch, Object: kioskID, RequestID: other, ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{}}}, expected).TerminalDevice
	}
	configure := func(client *http.Client, key, value, signed string, revision, expected int) *TerminalDevice {
		digest := sha256.Sum256([]byte(signed))
		return send(client, map[string]any{"terminalValue": value, "command": service.Request{Version: 2, Contract: "C3", Operation: "terminal.device-change",
			Domain: branch, Object: kioskID, RequestID: other, IdempotencyKey: key, ExpiresAt: time.Now().Unix() + 20,
			Body: map[string]string{"name": "read-server", "valueDigest": hex.EncodeToString(digest[:]), "expectedRevision": strconv.Itoa(revision)}}}, expected).TerminalDevice
	}
	const settingKey, readServer = "f4c14a7b-3110-4c2b-89b7-726995a12eaf", "https://rpc.example/stable"
	if got := device(kiosk, 200); got == nil || got.Revision != 0 || got.Payment != nil || got.Changes != nil {
		t.Fatalf("kiosk settings %+v", got)
	}
	device(clerk, 404)
	device(maker, 404)
	for _, client := range []*http.Client{kiosk, clerk, maker} {
		configure(client, settingKey, readServer, readServer, 0, 404)
	}
	configure(admin, settingKey, "https://evil.example/", readServer, 0, 400)
	configure(admin, settingKey, "http://rpc.example/", "http://rpc.example/", 0, 400)
	for range 2 {
		if got := configure(admin, settingKey, readServer, readServer, 0, 200); got.Revision != 1 || got.ReadServer != readServer ||
			len(got.Changes) != 1 || got.Changes[0].Actor != adminID || got.Payment == nil || got.Payment.ChainID != 8283 || got.Payment.RegistrationExpiresAt == 0 {
			t.Fatalf("recorded setting %+v", got)
		}
	}
	configure(admin, "f5c14a7b-3110-4c2b-89b7-726995a12eaf", "", "", 0, 409)
	if got := device(kiosk, 200); got.Revision != 1 || got.ReadServer != readServer || got.Payment != nil || got.Changes != nil {
		t.Fatalf("kiosk settings %+v", got)
	}
	send(kiosk, map[string]any{"command": service.Request{Version: 2, Contract: "C3", Operation: "network.list",
		Domain: branch, Object: kioskID, RequestID: other, ExpiresAt: time.Now().Unix() + 20, Body: map[string]string{}}}, 400)
	execute("UPDATE ledger.finance_grant SET valid_until=clock_timestamp() WHERE branch_id=$1 AND actor_id=$2", branch, clerkID)
	access(clerk, branch, clerkID, 404)
	queue(clerk, branch, 404)
}

func exerciseTerminalModeRealTLS(t *testing.T, ctx context.Context, base string, staff, kiosk *http.Client, branch, otherBranch, terminal string) {
	t.Helper()
	const change = "e4c14a7b-3110-4c2b-89b7-726995a12eaf"
	const different = "04c14a7b-3110-4c2b-89b7-726995a12eaf"
	command := func(client *http.Client, op, domain, key string, body map[string]string, expected int) result {
		t.Helper()
		payload, err := json.Marshal(map[string]any{"command": service.Request{
			Version: 2, Contract: "C3", Operation: op, Domain: domain, Object: terminal,
			RequestID: different, ExpiresAt: time.Now().Unix() + 20, IdempotencyKey: key, Body: body,
		}})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, "POST", base+"/v2/commerce", strings.NewReader(string(payload)))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != expected {
			t.Fatalf("%s status %d, want %d", op, resp.StatusCode, expected)
		}
		var out result
		if expected == http.StatusOK {
			if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
		}
		return out
	}
	got := command(staff, "terminal.state", branch, "", map[string]string{}, 200).Terminal
	if got.Mode != "menu" || got.Revision != 1 || got.Transition != nil || got.OrderCount == nil || *got.OrderCount != 21 || len(got.OrderRefs) != 20 {
		t.Fatalf("initial %+v", got)
	}
	cursor := got.OrderRefs[19]
	page := map[string]string{"kind": "preview", "scopeRef": terminal, "sourceRevision": "1", "afterSale": cursor}
	preview := command(staff, "terminal.orders", branch, "", page, 200).TerminalOrders
	if len(preview) != 1 || preview[0] <= cursor {
		t.Fatalf("preview page: %v", preview)
	}
	command(staff, "terminal.orders", otherBranch, "", page, 404)
	update := map[string]string{"mode": "pos", "expectedRevision": "1"}
	for range 2 {
		got = command(staff, "terminal.mode", branch, change, update, 200).Terminal
		if got.Mode != "menu" || got.Transition == nil || got.Transition.RequestRef != change || got.Transition.State != "waiting" {
			t.Fatalf("staged %+v", got)
		}
	}
	page["kind"], page["scopeRef"] = "transition", change
	handed := command(staff, "terminal.orders", branch, "", page, 200).TerminalOrders
	if len(handed) != 1 || handed[0] != preview[0] {
		t.Fatalf("handoff page: %v", handed)
	}
	page["afterSale"] = different
	command(staff, "terminal.orders", branch, "", page, 404)
	body := map[string]string{"transitionRef": change, "targetRevision": "2"}
	command(staff, "terminal.handoff", otherBranch, change, body, 404)
	command(staff, "terminal.handoff", branch, different, body, 400)
	command(staff, "terminal.ack", branch, change, body, 404)
	command(kiosk, "terminal.handoff", branch, change, body, 404)
	got = command(staff, "terminal.handoff", branch, change, body, 200).Terminal
	if got.Revision != 1 || got.Transition == nil || !got.Transition.Handoff || got.Transition.Ack {
		t.Fatalf("handoff only %+v", got)
	}
	command(kiosk, "terminal.ack", branch, change, map[string]string{"transitionRef": change, "targetRevision": "3"}, 404)
	got = command(kiosk, "terminal.ack", branch, change, body, 200).Terminal
	if got.Mode != "pos" || got.Revision != 2 || got.Transition == nil || got.Transition.State != "active" || got.OrderCount != nil {
		t.Fatalf("activated %+v", got)
	}
	got = command(staff, "terminal.mode", branch, change, update, 200).Terminal
	if got.Revision != 2 || got.Transition == nil || got.Transition.State != "active" {
		t.Fatalf("replayed %+v", got)
	}
}
