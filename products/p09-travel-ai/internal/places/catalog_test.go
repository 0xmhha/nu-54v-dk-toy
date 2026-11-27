package places

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSignedRegionLookupHasSameMapListPlaceIDs(t *testing.T) {
	key, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"version":4,"provider":"local_fixture","environment":"synthetic","updatedAt":"2026-11-20T00:00:00Z","places":[
{"id":"landmark","region":"jongno","name":"Sample landmark","address":"Sample plaza","longitude":126.97,"latitude":37.57,"source":"lab gazetteer","updatedAt":"2026-11-20T00:00:00Z","opening":[{"weekday":1,"timezone":"Asia/Seoul","opens":"9:00","closes":"10:00"}]},
{"id":"merchant","region":"jongno","name":"Sample cafe","address":"Sample corner","branchRef":"11111111-1111-4111-8111-111111111111","capabilities":["booking","pickup"],"source":"lab operator","updatedAt":"2026-11-20T00:00:00Z"}
]}`)
	directory := t.TempDir()
	if err = Install(directory, key, raw, ed25519.Sign(private, raw), time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	catalog, err := Open(directory, key)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	catalog.Register(mux)
	request := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/places?"+query, nil))
		return rec
	}
	list, pins := request("region=jongno&view=list"), request("region=jongno&view=map")
	if list.Code != 200 || !bytes.Equal(list.Body.Bytes(), pins.Body.Bytes()) || !bytes.Contains(list.Body.Bytes(), []byte("landmark")) ||
		!bytes.Contains(list.Body.Bytes(), []byte("branchRef")) || !bytes.Contains(list.Body.Bytes(), []byte(`"opens":"09:00"`)) {
		t.Fatalf("regional projection mismatch: %d %s", list.Code, list.Body)
	}
	if result := request("region=jongno&userLatitude=37"); result.Code != http.StatusBadRequest {
		t.Fatal("position input accepted")
	}
	if err = Install(directory, key, raw, ed25519.Sign(private, raw), time.Now()); err == nil {
		t.Fatal("older or equal version overwrote active catalog")
	}
	if err = os.WriteFile(filepath.Join(directory, "catalog.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(directory, key); err == nil {
		t.Fatal("untrusted store opened")
	}
}

func TestSignedCatalogRejectsVersionAboveWirePrecision(t *testing.T) {
	key, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"version":9007199254740992,"provider":"local_fixture","environment":"synthetic","updatedAt":"2026-11-20T00:00:00Z","places":[]}`)
	if err := Install(t.TempDir(), key, raw, ed25519.Sign(private, raw), time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("catalog accepted a version above the wire's exact integer range")
	}
}
