package records

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/places"
)

func TestLocalRankerResultCannotOverridePublishedBudget(t *testing.T) {
	var status atomic.Value
	status.Store("ok")
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var command map[string]any
		if err := json.NewDecoder(r.Body).Decode(&command); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		prompt, _ := command["prompt"].(string)
		if command["keep_alive"] != float64(0) || !strings.Contains(prompt, "publicCandidates") ||
			strings.Contains(prompt, "private-subject") || strings.Contains(prompt, "budgetLimit") {
			t.Error("unexpected public inference inputs")
		}
		switch status.Load().(string) {
		case "ok":
			reply(w, 200, map[string]any{"response": `{"order":["park","restaurant"]}`, "done": true})
		case "invented":
			reply(w, 200, map[string]any{"response": `{"order":["unlisted"]}`, "done": true})
		default:
			w.WriteHeader(503)
		}
	}))
	defer fixture.Close()
	ranker, err := OpenLocalRanker(fixture.URL+"/api/generate", "test-model")
	if err != nil {
		t.Fatal(err)
	}
	candidates := []rankCandidate{{"park", "attraction"}, {"restaurant", "restaurant"}}
	order, err := ranker.rank(context.Background(), candidates, "all")
	if err != nil || len(order) != 2 {
		t.Fatalf("approved rank unavailable: %v", err)
	}
	status.Store("invented")
	if _, err := ranker.rank(context.Background(), candidates, "all"); err == nil {
		t.Fatal("model accepted a hallucinated place")
	}
	status.Store("down")
	if _, err := ranker.rank(context.Background(), candidates, "all"); err == nil {
		t.Fatal("unavailable provider reported an inference")
	}
	if _, err := OpenLocalRanker("https://places.example/api/generate", "test-model"); err == nil {
		t.Fatal("external URL would carry compute inputs")
	}
	zero, over, unit := "0", "8500", "KRW"
	at := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	catalog := places.Catalog{Version: 1, Places: []places.Place{
		{ID: "park", PlanningCost: &zero, PlanningUnit: &unit, UpdatedAt: at, Source: "signed fixture",
			Opening: []places.OpeningInterval{{Weekday: int(at.Weekday()), Timezone: "Asia/Seoul", Opens: "09:00", Closes: "18:00"}}},
		{ID: "restaurant", PlanningCost: &over, PlanningUnit: &unit, UpdatedAt: at, Source: "signed fixture",
			Opening: []places.OpeningInterval{{Weekday: int(at.Weekday()), Timezone: "Asia/Seoul", Opens: "09:00", Closes: "18:00"}}},
	}}
	params := GeneratePlan{ID: "a4aa517d-b258-4f88-bea1-00630c790cd5", Consent: 1, Catalog: 1, Budget: "5000", BudgetUnit: "KRW",
		Start: at.Add(15 * time.Minute), End: at.Add(5 * time.Hour), Expires: at.Add(6 * time.Hour)}
	value, err := schedule(catalog, params, order, at)
	if err != nil || len(value.Stops) != 1 || value.Stops[0].Place != "park" || value.Projected != "0" {
		t.Fatalf("model rank bypassed trusted budget: %#v %v", value, err)
	}
}
