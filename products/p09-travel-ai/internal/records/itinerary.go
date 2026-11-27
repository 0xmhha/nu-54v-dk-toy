package records

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p09-travel-ai/internal/places"
)

type catalogLoader func(context.Context) (places.Catalog, error)

func (s *Service) EnableCatalog(load func(context.Context) (places.Catalog, error)) { s.catalog = load }

type StopInput struct {
	Place string    `json:"place"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}
type Plan struct {
	ID         string      `json:"id"`
	Consent    int64       `json:"consentRevision"`
	Catalog    uint64      `json:"catalogVersion"`
	Expires    time.Time   `json:"expiresAt"`
	Budget     string      `json:"budgetLimit"`
	BudgetUnit string      `json:"budgetUnit,omitempty"`
	Stops      []StopInput `json:"stops"`
}
type stopDetail struct {
	Name     string                 `json:"name"`
	Category string                 `json:"category,omitempty"`
	Opening  places.OpeningInterval `json:"opening"`
	Source   string                 `json:"source"`
	Updated  time.Time              `json:"updatedAt"`
	Cost     *string                `json:"estimatedCost,omitempty"`
}

type proposal struct {
	Plan
	Sources   []string     `json:"sources"`
	Warnings  []string     `json:"warnings"`
	Projected string       `json:"projectedCost,omitempty"`
	Details   []stopDetail `json:"details"`
}
type ApplyPlan struct {
	Plan
	Expected int64  `json:"expectedRevision"`
	Request  string `json:"requestKey"`
	Confirm  string `json:"confirmedPreview"`
}

func validatePlan(catalog places.Catalog, input Plan, now time.Time) (proposal, error) {
	result := proposal{Plan: input, Warnings: []string{"straight_line_travel_estimate"}}
	budget, err := strconv.ParseUint(input.Budget, 10, 64)
	if err != nil || strconv.FormatUint(budget, 10) != input.Budget || budget == 0 || !uuid.MatchString(input.ID) ||
		input.Consent < 1 || input.Consent > 9007199254740991 || input.Catalog != catalog.Version ||
		!input.Expires.After(now) || input.Expires.After(now.AddDate(0, 1, 0)) || len(input.Stops) < 1 || len(input.Stops) > 14 {
		return result, errors.New("plan scope unavailable")
	}
	if input.BudgetUnit != "" && input.BudgetUnit != "KRW" && input.BudgetUnit != "TestUSDC" {
		return result, errors.New("invalid budget asset")
	}
	var projected uint64
	unknown := false
	index := make(map[string]places.Place, len(catalog.Places))
	for _, p := range catalog.Places {
		index[p.ID] = p
	}
	for i, stop := range input.Stops {
		p, ok := index[stop.Place]
		if !ok || stop.Start.Before(now) || !stop.End.After(stop.Start) || stop.End.After(input.Expires) ||
			stop.End.Sub(stop.Start) > 12*time.Hour {
			return result, errors.New("stop time or place unavailable")
		}
		var opening *places.OpeningInterval
		for _, interval := range p.Opening {
			zone, err := time.LoadLocation(interval.Timezone)
			if err != nil {
				continue
			}
			start, end := stop.Start.In(zone), stop.End.In(zone)
			if int(start.Weekday()) == interval.Weekday && start.YearDay() == end.YearDay() &&
				start.Format("15:04") >= interval.Opens && end.Format("15:04") <= interval.Closes {
				opening = &interval
				break
			}
		}
		if opening == nil {
			return result, errors.New("published opening does not cover stop")
		}
		detail := stopDetail{Name: p.Name, Category: p.Category, Opening: *opening, Source: p.Source, Updated: p.UpdatedAt}
		if input.BudgetUnit == "" || p.PlanningUnit == nil || p.PlanningCost == nil || *p.PlanningUnit != input.BudgetUnit {
			unknown = true
		} else {
			amount, err := strconv.ParseUint(*p.PlanningCost, 10, 64)
			if err != nil || amount > budget-projected {
				return result, errors.New("published budget exceeded")
			}
			projected += amount
			detail.Cost = p.PlanningCost
		}
		result.Details = append(result.Details, detail)
		result.Sources = append(result.Sources, p.Source+" @ "+p.UpdatedAt.UTC().Format(time.RFC3339))
		if i == 0 {
			continue
		}
		previous := input.Stops[i-1]
		distance, available := kilometers(index[previous.Place], p)
		if stop.Start.Before(previous.End) || available && stop.Start.Sub(previous.End).Hours() < distance/100 {
			return result, errors.New("insufficient travel interval")
		}
		if !available {
			result.Warnings = append(result.Warnings, "travel_route_unconfirmed")
		}
	}
	if unknown {
		result.Warnings = append([]string{"price_unconfirmed"}, result.Warnings...)
	} else {
		result.Projected = strconv.FormatUint(projected, 10)
	}
	return result, nil
}

func kilometers(a, b places.Place) (float64, bool) {
	if a.Latitude == nil || a.Longitude == nil || b.Latitude == nil || b.Longitude == nil {
		return 0, false
	}
	const radians = math.Pi / 180
	dlat, dlon := (*a.Latitude-*b.Latitude)*radians, (*a.Longitude-*b.Longitude)*radians
	x := math.Pow(math.Sin(dlat/2), 2) + math.Cos(*a.Latitude*radians)*math.Cos(*b.Latitude*radians)*math.Pow(math.Sin(dlon/2), 2)
	return 12742 * math.Asin(math.Sqrt(math.Max(0, math.Min(1, x)))), true
}

type savedItinerary struct {
	Revision int64           `json:"revision"`
	Plan     json.RawMessage `json:"plan"`
}

type itinerarySummary struct {
	ID       string    `json:"id"`
	Revision int64     `json:"revision"`
	Name     string    `json:"name,omitempty"`
	Place    string    `json:"place"`
	Start    time.Time `json:"start"`
	Timezone string    `json:"timezone,omitempty"`
}

func (s *Service) itinerarySummaries(ctx context.Context, subject string) ([]itinerarySummary, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text,revision,sealed FROM travel_private.list_itineraries($1)`, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]itinerarySummary, 0)
	for rows.Next() {
		var summary itinerarySummary
		var sealed []byte
		if err := rows.Scan(&summary.ID, &summary.Revision, &sealed); err != nil {
			return nil, err
		}
		body, err := s.content.open(subject+":itinerary:"+summary.ID, sealed)
		if err != nil {
			return nil, err
		}
		var plan proposal
		err = json.Unmarshal(body, &plan)
		clear(body)
		if err != nil || len(plan.Stops) == 0 {
			return nil, errors.New("invalid saved itinerary")
		}
		summary.Place, summary.Start = plan.Stops[0].Place, plan.Stops[0].Start
		if len(plan.Details) > 0 {
			summary.Name, summary.Timezone = plan.Details[0].Name, plan.Details[0].Opening.Timezone
		}
		result = append(result, summary)
	}
	return result, rows.Err()
}

func (s *Service) itinerary(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.principal(r)
	if !ok || actor.Role != "private-user" || s.content == nil || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	if id := r.PathValue("id"); r.Method == http.MethodGet {
		if !uuid.MatchString(id) {
			http.NotFound(w, r)
			return
		}
		var sealed []byte
		var revision int64
		if err := s.pool.QueryRow(ctx, `SELECT revision,sealed FROM travel_private.read_itinerary($1,$2)`, actor.Subject, id).Scan(&revision, &sealed); err != nil {
			authFailure(w, err)
			return
		}
		body, err := s.content.open(actor.Subject+":itinerary:"+id, sealed)
		if err != nil {
			authFailure(w, err)
			return
		}
		defer clear(body)
		reply(w, http.StatusOK, savedItinerary{revision, body})
		return
	}
	if s.catalog == nil {
		http.NotFound(w, r)
		return
	}
	var input ApplyPlan
	if !strictBody(w, r, 8192, &input) || input.Expected < 0 || input.Expected >= 9007199254740991 {
		reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_itinerary"})
		return
	}
	catalog, err := s.catalog(ctx)
	if err != nil {
		authFailure(w, err)
		return
	}
	preview, err := validatePlan(catalog, input.Plan, time.Now())
	if err != nil {
		reply(w, http.StatusConflict, map[string]string{"error": "itinerary_needs_comparison"})
		return
	}
	raw, err := json.Marshal(preview)
	if err != nil {
		authFailure(w, err)
		return
	}
	scope := actor.Subject + ":itinerary:" + input.ID
	confirmation := s.content.dedup(scope+":"+strconv.FormatInt(input.Expected, 10), raw)
	code := base64.RawURLEncoding.EncodeToString(confirmation)
	if r.URL.Path == "/v1/private/itineraries/preview" {
		if input.Confirm != "" || input.Request != "" {
			reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_itinerary"})
			return
		}
		if err := s.recommendationAllowed(ctx, actor.Subject, input.Consent, input.ID, input.Expires); err != nil {
			authFailure(w, err)
			return
		}
		reply(w, http.StatusOK, struct {
			Preview proposal `json:"preview"`
			Code    string   `json:"confirmation"`
		}{preview, code})
		return
	}
	if !uuid.MatchString(input.Request) || input.Confirm != code {
		reply(w, http.StatusConflict, map[string]string{"error": "review_current_preview"})
		return
	}
	sealed, err := s.content.seal(scope, raw)
	if err != nil {
		authFailure(w, err)
		return
	}
	digest := sha256.Sum256(confirmation)
	var revision int64
	err = s.mutationRow(r, actor, s.pool, `SELECT travel_private.apply_itinerary($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		actor.Subject, input.ID, input.Expected, input.Consent, input.Catalog, input.Request, digest[:], sealed, input.Expires).Scan(&revision)
	if err != nil {
		authFailure(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]int64{"revision": revision})
}
