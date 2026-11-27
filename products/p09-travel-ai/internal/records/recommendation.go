package records

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/places"
)

type rankCandidate struct {
	ID       string `json:"id"`
	Category string `json:"category"`
}
type Ranker struct {
	client          *http.Client
	endpoint, model string
}

var localModel = regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,80}$`)

func OpenLocalRanker(endpoint, model string) (*Ranker, error) {
	value, err := url.Parse(endpoint)
	if err != nil || value.Scheme != "http" || value.Hostname() != "127.0.0.1" || value.User != nil || value.RawQuery != "" ||
		value.Fragment != "" || value.Path != "/api/generate" || !localModel.MatchString(model) {
		return nil, errors.New("approved loopback ranker required")
	}
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 60 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	return &Ranker{client, endpoint, model}, nil
}
func (r *Ranker) rank(ctx context.Context, candidates []rankCandidate, preference string) ([]string, error) {
	// Only approved public identifiers and a coarse category enter local inference.
	public, err := json.Marshal(struct {
		Candidates []rankCandidate `json:"publicCandidates"`
		Category   string          `json:"categoryPreference"`
	}{candidates, preference})
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"model": r.model, "prompt": "Rank these public place IDs for the category preference. Use only listed IDs. Return JSON {\"order\":[\"id\"]}. Include every listed ID once. " + string(public),
		"stream": false, "format": "json", "keep_alive": 0, "options": map[string]any{"num_predict": 256, "temperature": 0}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", r.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := r.client.Do(req)
	if err != nil {
		return nil, errors.New("local ranker unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("local ranker unavailable")
	}
	var output struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}
	decoder := json.NewDecoder(io.LimitReader(res.Body, 65536))
	if err := decoder.Decode(&output); err != nil || !output.Done || len(output.Response) > 8192 {
		return nil, errors.New("inference incomplete")
	}
	var result struct {
		Order []string `json:"order"`
	}
	decoder = json.NewDecoder(bytes.NewBufferString(output.Response))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return nil, errors.New("invalid model candidate output")
	}
	if _, err := decoder.Token(); err != io.EOF || len(result.Order) == 0 || len(result.Order) > len(candidates) {
		return nil, errors.New("invalid model candidate output")
	}
	allowed := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		allowed[candidate.ID] = true
	}
	for _, id := range result.Order {
		if !allowed[id] {
			return nil, errors.New("unknown or duplicate model candidate")
		}
		delete(allowed, id)
	}
	return result.Order, nil
}

type GeneratePlan struct {
	ID         string    `json:"id"`
	Region     string    `json:"region"`
	Preference string    `json:"preference"`
	Consent    int64     `json:"consentRevision"`
	Catalog    uint64    `json:"catalogVersion"`
	Expected   int64     `json:"expectedRevision"`
	Expires    time.Time `json:"expiresAt"`
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	Budget     string    `json:"budgetLimit"`
	BudgetUnit string    `json:"budgetUnit"`
}

func (s *Service) EnableLocalRanker(ranker *Ranker) { s.ranker = ranker }

func schedule(catalog places.Catalog, cmd GeneratePlan, order []string, now time.Time) (proposal, error) {
	plan := Plan{ID: cmd.ID, Consent: cmd.Consent, Catalog: cmd.Catalog, Expires: cmd.Expires, Budget: cmd.Budget, BudgetUnit: cmd.BudgetUnit}
	for _, id := range order {
		if len(plan.Stops) == 3 {
			break
		}
		begins := cmd.Start
		if n := len(plan.Stops); n > 0 {
			begins = plan.Stops[n-1].End.Add(45 * time.Minute)
		}
		for start := begins; !start.Add(45 * time.Minute).After(cmd.End); start = start.Add(15 * time.Minute) {
			trial := plan
			trial.Stops = append(append([]StopInput(nil), plan.Stops...), StopInput{Place: id, Start: start, End: start.Add(45 * time.Minute)})
			if _, err := validatePlan(catalog, trial, now); err == nil {
				plan = trial
				break
			}
		}
	}
	return validatePlan(catalog, plan, now)
}

func (s *Service) recommendationAllowed(ctx context.Context, subject string, revision int64, id string, until time.Time) error {
	var expires time.Time
	if err := s.pool.QueryRow(ctx, `SELECT travel_private.recommendation_allowed($1,$2,$3)`, subject, revision, id).Scan(&expires); err != nil {
		return err
	}
	if until.After(expires) {
		return errors.New("recommendation scope unavailable")
	}
	return nil
}

func (s *Service) generateItinerary(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.principal(r)
	if !ok || actor.Role != "private-user" || s.content == nil || s.catalog == nil || s.ranker == nil || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	var cmd GeneratePlan
	now := time.Now()
	budgetOK := func() bool {
		v, err := strconv.ParseUint(cmd.Budget, 10, 64)
		return err == nil && v > 0 && strconv.FormatUint(v, 10) == cmd.Budget
	}
	if !strictBody(w, r, 8192, &cmd) || !uuid.MatchString(cmd.ID) || !placeCode.MatchString(cmd.Region) ||
		(cmd.Preference != "all" && cmd.Preference != "restaurant" && cmd.Preference != "attraction") || cmd.Consent < 1 || cmd.Expected < 0 ||
		cmd.Consent > 9007199254740991 || cmd.Expected >= 9007199254740991 || !budgetOK() || (cmd.BudgetUnit != "KRW" && cmd.BudgetUnit != "TestUSDC") ||
		cmd.Start.Before(now) || cmd.End.Sub(cmd.Start) < 45*time.Minute || cmd.End.Sub(cmd.Start) > 24*time.Hour || cmd.End.After(cmd.Expires) {
		reply(w, 400, map[string]string{"error": "invalid_recommendation"})
		return
	}
	ctx := r.Context()
	allowed := func() error {
		return s.recommendationAllowed(ctx, actor.Subject, cmd.Consent, cmd.ID, cmd.Expires)
	}
	if err := allowed(); err != nil {
		authFailure(w, err)
		return
	}
	catalog, err := s.catalog(ctx)
	if err != nil || catalog.Version != cmd.Catalog {
		reply(w, 409, map[string]string{"error": "catalog_needs_comparison"})
		return
	}
	var candidates []rankCandidate
	localCatalog := places.Catalog{Version: catalog.Version}
	for _, place := range catalog.Places {
		if place.Region != cmd.Region || len(place.Opening) == 0 || place.PlanningUnit == nil || *place.PlanningUnit != cmd.BudgetUnit || place.PlanningCost == nil {
			continue
		}
		for start := cmd.Start; !start.Add(45 * time.Minute).After(cmd.End); start = start.Add(15 * time.Minute) {
			one := []StopInput{{Place: place.ID, Start: start, End: start.Add(45 * time.Minute)}}
			mini := places.Catalog{Version: catalog.Version, Places: []places.Place{place}}
			plan := Plan{ID: cmd.ID, Consent: cmd.Consent, Catalog: cmd.Catalog, Expires: cmd.Expires, Budget: cmd.Budget, BudgetUnit: cmd.BudgetUnit, Stops: one}
			if _, err := validatePlan(mini, plan, now); err == nil {
				localCatalog.Places = append(localCatalog.Places, place)
				candidates = append(candidates, rankCandidate{place.ID, place.Category})
				break
			}
		}
		if len(candidates) == 12 {
			break
		}
	}
	if len(candidates) == 0 {
		reply(w, 409, map[string]string{"error": "no_open_public_places"})
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(65 * time.Second))
	if err := allowed(); err != nil {
		authFailure(w, err)
		return
	}
	if !s.stillAuthorized(r, actor) {
		http.NotFound(w, r)
		return
	}
	ordered, err := s.ranker.rank(ctx, candidates, cmd.Preference)
	if authErr := allowed(); authErr != nil {
		authFailure(w, authErr)
		return
	}
	if err != nil {
		reply(w, 503, map[string]string{"error": "local_provider_unavailable"})
		return
	}
	proposed, err := schedule(localCatalog, cmd, ordered, now)
	if err != nil {
		reply(w, 409, map[string]string{"error": "no_valid_generated_itinerary"})
		return
	}
	raw, err := json.Marshal(proposed)
	if err != nil {
		authFailure(w, err)
		return
	}
	if err := allowed(); err != nil {
		authFailure(w, err)
		return
	}
	code := s.content.dedup(actor.Subject+":itinerary:"+cmd.ID+":"+strconv.FormatInt(cmd.Expected, 10), raw)
	reply(w, 200, struct {
		Preview  proposal `json:"preview"`
		Code     string   `json:"confirmation"`
		Provider string   `json:"provider"`
	}{
		proposed, base64.RawURLEncoding.EncodeToString(code), "local/" + s.ranker.model,
	})
}
