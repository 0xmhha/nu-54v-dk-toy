package commerce

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/jackc/pgx/v5"
)

func TestTravelPublicLookupAndLastSeatsTLS(t *testing.T) {
	dsn, publicURL := os.Getenv("P05_TRAVEL_TEST_DATABASE_URL"), os.Getenv("P09_TRAVEL_TEST_PUBLIC_URL")
	if dsn == "" && publicURL == "" {
		t.Skip("requires disposable commerce database and P09 service")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(cfg.Database, "nu54_commerce_") ||
		!(strings.HasPrefix(cfg.Host, "/tmp/") || strings.HasPrefix(cfg.Host, "/private/tmp/")) ||
		!strings.HasPrefix(publicURL, "http://127.0.0.1:") {
		t.Fatal("use disposable local P05 and P09 fixtures")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, "GET", publicURL+"/v1/places?region=seoul", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var catalog struct {
		Environment string
		Places      []struct {
			ID, BranchRef, Source, UpdatedAt string
			Capabilities                     []string
		}
	}
	if err := json.NewDecoder(response.Body).Decode(&catalog); err != nil || response.StatusCode != 200 ||
		catalog.Environment != "synthetic" || len(catalog.Places) != 2 {
		t.Fatalf("unexpected P09 result: %+v %v", catalog, err)
	}
	var branchID string
	for _, place := range catalog.Places {
		switch place.ID {
		case "landmark":
			if place.BranchRef != "" || len(place.Capabilities) != 0 || place.Source == "" || place.UpdatedAt == "" {
				t.Fatalf("unexpected landmark %+v", place)
			}
		case "merchant":
			if len(place.Capabilities) == 1 && place.Capabilities[0] == "booking" {
				branchID = place.BranchRef
			}
		}
	}
	if !travelUUID.MatchString(branchID) {
		t.Fatal("published merchant lacks booking reference")
	}
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	login := "travel_" + strings.TrimPrefix(cfg.Database, "nu54_commerce_")
	role := pgx.Identifier{login}.Sanitize()
	exec("CREATE ROLE " + role + " LOGIN IN ROLE nu54_p05_commerce")
	defer admin.Exec(context.Background(), "DROP ROLE "+role)
	store, err := Open(ctx, dsn+" user="+login)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	const slot = "5eb86d05-907a-4602-ab91-f85dd71d845c"
	const zero = "568299aa-19ef-492e-9bd3-eca7eec67d0b"
	const key = "d41513c7-8036-43e5-8339-724afb603b69"
	const zeroKey = "552678a9-5cb4-4d8d-9d36-c2d2cb41fde9"
	asset := "0x" + strings.Repeat("2", 40)
	exec("INSERT INTO ledger.branch VALUES ($1,$2,'Asia/Seoul','04:00')", branchID, tenant)
	defer admin.Exec(context.Background(), "DELETE FROM ledger.branch WHERE id=$1", branchID)
	exec(`INSERT INTO ledger.commerce_grant(branch_id,actor_id,role_code,valid_until)
		VALUES ($1,$2,'customer',now()+interval '5 minutes'),($1,$3,'customer',now()+interval '5 minutes')`, branchID, customerA, customerB)
	defer admin.Exec(context.Background(), "DELETE FROM ledger.commerce_grant WHERE branch_id=$1", branchID)
	exec(`INSERT INTO ledger.travel_slot(branch_id,id,starts_at,capacity,terms_version,cancellation_terms,asset_ref,deposit,balance,
		price_version,no_show_terms,chain_id,cancellation_cutoff_seconds)
		VALUES ($1,$2,now()+interval '2 hours',3,3,'Partial refund follows policy v3',$4,5000000,10000000,2,'Deposit is retained for a no-show',8283,1800),
		($1,$3,now()+interval '3 hours',1,1,'Deposit is full price',$4,10,0,NULL,NULL,NULL,0)`, branchID, slot, zero, asset)
	defer admin.Exec(context.Background(), "DELETE FROM ledger.travel_slot WHERE branch_id=$1", branchID)
	defer admin.Exec(context.Background(), "DELETE FROM ledger.travel_booking WHERE branch_id=$1", branchID)

	roots := x509.NewCertPool()
	credentials := make(map[string]service.Principal)
	var certificates []tls.Certificate
	for idx, customer := range []string{customerA, customerB} {
		pub, private, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		template := &x509.Certificate{SerialNumber: big.NewInt(int64(idx + 1)), IsCA: true, BasicConstraintsValid: true,
			NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Minute),
			KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
		der, err := x509.CreateCertificate(rand.Reader, template, template, pub, private)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		roots.AddCert(cert)
		certificates = append(certificates, tls.Certificate{Certificate: [][]byte{der}, PrivateKey: private})
		hash := sha256.Sum256(der)
		credentials[hex.EncodeToString(hash[:])] = service.Principal{Tenant: tenant, Subject: customer, Role: "customer", ExpiresAt: time.Now().Add(time.Minute).Unix()}
	}
	mux := http.NewServeMux()
	HTTP{Store: store, Credentials: credentials}.Register(mux)
	server := httptest.NewUnstartedServer(mux)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	server.StartTLS()
	defer server.Close()

	type reply struct {
		Slots               []TravelSlot
		Booking             TravelBooking
		CancellationPreview *TravelCancellationPreview
		Error               string
		Code                int
		Err                 error
	}
	call := func(customer int, operation TravelRequest) reply {
		config := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
		config.Certificates = []tls.Certificate{certificates[customer]}
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: config, DisableKeepAlives: true}}
		defer client.CloseIdleConnections()
		if operation.Branch == "" {
			operation.Branch = branchID
		}
		raw, err := json.Marshal(map[string]TravelRequest{"travel": operation})
		if err != nil {
			return reply{Err: err}
		}
		req, err := http.NewRequestWithContext(ctx, "POST", server.URL+"/v2/commerce", bytes.NewReader(raw))
		if err != nil {
			return reply{Err: err}
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return reply{Err: err}
		}
		defer res.Body.Close()
		result := reply{Code: res.StatusCode}
		result.Err = json.NewDecoder(res.Body).Decode(&result)
		return result
	}
	check := func(r reply, status int, label string) {
		t.Helper()
		if r.Err != nil || r.Code != status || r.Error != label {
			t.Fatalf("want %d %q, got %+v", status, label, r)
		}
	}
	options := TravelRequest{Operation: "options"}
	available := call(0, options)
	check(available, 200, "")
	if len(available.Slots) != 2 || available.Slots[0].Remaining != 3 || available.Slots[1].Balance != "0" {
		t.Fatalf("wrong slots %+v", available)
	}
	hold := TravelRequest{Operation: "hold", Slot: slot, Key: key, Party: 2, Terms: 3, Deposit: "10000000", Balance: "20000000", Asset: asset}
	stale := hold
	stale.Terms = 2
	check(call(0, stale), 409, "terms_changed")
	responses := []chan reply{make(chan reply, 1), make(chan reply, 1)}
	start := make(chan struct{})
	for idx := range responses {
		go func() { <-start; responses[idx] <- call(idx, hold) }()
	}
	close(start)
	outcomes := []reply{<-responses[0], <-responses[1]}
	winner := 0
	if outcomes[1].Code == 200 {
		winner = 1
	}
	winning := outcomes[winner]
	check(winning, 200, "")
	check(outcomes[1-winner], 409, "sold_out")
	if winning.Booking.Party != 2 || winning.Booking.Deposit != "10000000" || winning.Booking.Balance != "20000000" {
		t.Fatalf("wrong accepted terms %+v", winning)
	}
	booked := winning.Booking
	preview := TravelRequest{Operation: "cancel-preview", Booking: booked.ID}
	view := call(winner, preview)
	check(view, 200, "")
	if view.CancellationPreview == nil || !view.CancellationPreview.CanCancel ||
		!view.CancellationPreview.Deadline.Equal(booked.Cutoff) || len(view.CancellationPreview.Legs) != 0 {
		t.Fatal("an unpaid hold must not invent a refund or change its terms")
	}
	check(call(1-winner, preview), 404, "booking_unavailable")
	foreign := preview
	foreign.Branch = "61cc1ee1-e4db-4da4-8fcf-22657622dd88"
	check(call(winner, foreign), 404, "booking_unavailable")
	exec("UPDATE ledger.travel_booking SET cancellation_deadline=clock_timestamp() WHERE branch_id=$1 AND id=$2", branchID, booked.ID)
	atCutoff := call(winner, preview)
	check(atCutoff, 200, "")
	if atCutoff.CancellationPreview == nil || atCutoff.CancellationPreview.CanCancel {
		t.Fatal("an elapsed deadline must disable cancellation")
	}
	exec("UPDATE ledger.travel_booking SET cancellation_deadline=$3 WHERE branch_id=$1 AND id=$2", branchID, booked.ID, booked.Cutoff)
	first := available.Slots[0]
	if booked.Terms != 3 || booked.PriceVersion == nil || *booked.PriceVersion != 2 ||
		booked.ChainID == nil || *booked.ChainID != 8283 || booked.NoShow == nil ||
		*booked.NoShow != "Deposit is retained for a no-show" || booked.Starts == nil ||
		!booked.Starts.Equal(first.Starts) || !booked.Cutoff.Equal(first.Starts.Add(-30*time.Minute)) {
		t.Fatalf("review terms did not match original slot: %+v", booked)
	}
	if _, err := admin.Exec(ctx, `UPDATE ledger.travel_slot SET no_show_terms='Changed' WHERE branch_id=$1 AND id=$2`, branchID, slot); err == nil {
		t.Fatal("published terms changed without advancing the version")
	}
	exec(`UPDATE ledger.travel_slot SET starts_at=starts_at+interval '10 minutes',terms_version=4,price_version=3,
		no_show_terms='New no-show terms',cancellation_terms='New cancellation terms',cancellation_cutoff_seconds=0
		WHERE branch_id=$1 AND id=$2`, branchID, slot)
	same := call(winner, TravelRequest{Operation: "reopen", Key: key})
	check(same, 200, "")
	originalJSON, _ := json.Marshal(booked)
	reopenedJSON, _ := json.Marshal(same.Booking)
	if !bytes.Equal(originalJSON, reopenedJSON) {
		t.Fatal("reconnect changed original booking terms")
	}
	check(call(1-winner, TravelRequest{Operation: "reopen", Booking: same.Booking.ID}), 404, "booking_unavailable")
	replayed := call(winner, hold)
	check(replayed, 200, "")
	replayedJSON, _ := json.Marshal(replayed.Booking)
	if !bytes.Equal(originalJSON, replayedJSON) {
		t.Fatal("retry changed original booking terms")
	}
	changed := hold
	changed.Party = 1
	check(call(winner, changed), 409, "request_conflict")
	available = call(winner, options)
	check(available, 200, "")
	if available.Slots[0].Remaining != 1 {
		t.Fatal("capacity must count the accepted party")
	}
	exec("UPDATE ledger.travel_booking SET expires_at=now()-interval '1 second' WHERE branch_id=$1 AND id=$2", branchID, same.Booking.ID)
	available = call(winner, options)
	check(available, 200, "")
	if available.Slots[0].Remaining != 3 {
		t.Fatal("unissued expired hold should free seats")
	}
	exec("UPDATE ledger.travel_booking SET issuance_started_at=now()-interval '1 minute' WHERE branch_id=$1 AND id=$2", branchID, same.Booking.ID)
	available = call(winner, options)
	check(available, 200, "")
	if available.Slots[0].Remaining != 1 {
		t.Fatal("expiry cannot release capacity after attempted issuance")
	}
	fullDeposit := call(winner, TravelRequest{Operation: "hold", Slot: zero, Key: zeroKey, Party: 1, Terms: 1, Deposit: "10", Balance: "0", Asset: asset})
	check(fullDeposit, 200, "")
	if fullDeposit.Booking.Balance != "0" {
		t.Fatalf("full deposit cannot create a balance obligation: %+v", fullDeposit)
	}
	// Simulate a row written before the review fields were introduced. Publishing
	// a richer slot cannot retroactively establish the missing booking evidence.
	exec(`UPDATE ledger.travel_booking SET starts_at=NULL WHERE branch_id=$1 AND id=$2`, branchID, fullDeposit.Booking.ID)
	exec(`UPDATE ledger.travel_slot SET terms_version=2,price_version=1,
		no_show_terms='Published after hold',chain_id=8283 WHERE branch_id=$1 AND id=$2`, branchID, zero)
	historic := call(winner, TravelRequest{Operation: "reopen", Booking: fullDeposit.Booking.ID})
	check(historic, 200, "")
	if historic.Booking.PriceVersion != nil || historic.Booking.NoShow != nil || historic.Booking.ChainID != nil ||
		historic.Booking.Starts != nil || historic.Booking.Cancellation != fullDeposit.Booking.Cancellation ||
		!historic.Booking.Cutoff.Equal(fullDeposit.Booking.Cutoff) {
		t.Fatal("reopen substituted later slot terms for original evidence")
	}
}
