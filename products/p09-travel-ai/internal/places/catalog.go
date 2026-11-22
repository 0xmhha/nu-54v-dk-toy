package places

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"syscall"
	"time"
)

const maxCatalog = 2 * 1024 * 1024

var slug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,47}$`)
var planningCost = regexp.MustCompile(`^(0|[1-9][0-9]{0,11})$`)
var branch = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

type OpeningInterval struct {
	Weekday  int    `json:"weekday"`
	Timezone string `json:"timezone"`
	Opens    string `json:"opens"`
	Closes   string `json:"closes"`
}

type Place struct {
	ID           string            `json:"id"`
	Region       string            `json:"region"`
	RegionName   string            `json:"regionName,omitempty"`
	Category     string            `json:"category,omitempty"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Longitude    *float64          `json:"longitude,omitempty"`
	Latitude     *float64          `json:"latitude,omitempty"`
	Source       string            `json:"source"`
	UpdatedAt    time.Time         `json:"updatedAt"`
	BranchRef    string            `json:"branchRef,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	PlanningCost *string           `json:"planningCost,omitempty"`
	PlanningUnit *string           `json:"planningUnit,omitempty"`
	Opening      []OpeningInterval `json:"opening,omitempty"`
}

type Catalog struct {
	Version     uint64    `json:"version"`
	Provider    string    `json:"provider"`
	Environment string    `json:"environment"`
	UpdatedAt   time.Time `json:"updatedAt"`
	Places      []Place   `json:"places"`
}

type sealed struct {
	Document  []byte `json:"document"`
	Signature []byte `json:"signature"`
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}

func parse(key ed25519.PublicKey, input sealed) (Catalog, error) {
	var catalog Catalog
	if len(key) != ed25519.PublicKeySize || len(input.Document) == 0 || len(input.Document) > maxCatalog ||
		!ed25519.Verify(key, input.Document, input.Signature) {
		return catalog, errors.New("catalog signature invalid")
	}
	if err := decodeStrict(input.Document, &catalog); err != nil {
		return catalog, err
	}
	if catalog.Version == 0 || catalog.Version > (1<<53)-1 || !slug.MatchString(catalog.Provider) || catalog.UpdatedAt.IsZero() || catalog.UpdatedAt.Nanosecond()%1000 != 0 ||
		(catalog.Environment != "synthetic" && catalog.Environment != "public") || len(catalog.Places) > 10000 {
		return catalog, errors.New("invalid catalog metadata")
	}
	seen := make(map[string]bool)
	regionNames := make(map[string]string)
	for _, place := range catalog.Places {
		if len(place.RegionName) > 120 || (place.Category != "" && place.Category != "restaurant" && place.Category != "attraction") ||
			!slug.MatchString(place.ID) || !slug.MatchString(place.Region) || seen[place.ID] || len(place.Name) == 0 ||
			len(place.Name) > 180 || len(place.Address) == 0 || len(place.Address) > 360 || len(place.Source) == 0 ||
			len(place.Source) > 180 || place.UpdatedAt.IsZero() || place.UpdatedAt.Nanosecond()%1000 != 0 || place.UpdatedAt.After(catalog.UpdatedAt) ||
			(place.Latitude == nil) != (place.Longitude == nil) ||
			(place.Latitude != nil && (*place.Latitude < -90 || *place.Latitude > 90 ||
				*place.Longitude < -180 || *place.Longitude > 180)) ||
			(place.BranchRef == "") != (len(place.Capabilities) == 0) ||
			(place.BranchRef != "" && !branch.MatchString(place.BranchRef)) {
			return catalog, fmt.Errorf("invalid place metadata at %d", len(seen))
		}
		if (place.PlanningCost == nil) != (place.PlanningUnit == nil) ||
			(place.PlanningCost != nil && (!planningCost.MatchString(*place.PlanningCost) ||
				(*place.PlanningUnit != "KRW" && *place.PlanningUnit != "TestUSDC"))) {
			return catalog, errors.New("invalid published planning budget")
		}
		if prior, found := regionNames[place.Region]; found && prior != place.RegionName {
			return catalog, errors.New("inconsistent published region label")
		}
		regionNames[place.Region] = place.RegionName
		capabilities := make(map[string]bool)
		for _, capability := range place.Capabilities {
			if (capability != "booking" && capability != "pickup") || capabilities[capability] {
				return catalog, errors.New("invalid place capability")
			}
			capabilities[capability] = true
		}
		if len(place.Opening) > 70 {
			return catalog, errors.New("too many opening intervals")
		}
		for i := range place.Opening {
			interval := &place.Opening[i]
			_, validZone := time.LoadLocation(interval.Timezone)
			opens, validOpens := time.Parse("15:04", interval.Opens)
			closes, validCloses := time.Parse("15:04", interval.Closes)
			if interval.Weekday < 0 || interval.Weekday > 6 || validZone != nil || interval.Timezone == "Local" ||
				validOpens != nil || validCloses != nil || !opens.Before(closes) {
				return catalog, errors.New("invalid public opening interval")
			}
			interval.Opens, interval.Closes = opens.Format("15:04"), closes.Format("15:04")
		}
		sort.Slice(place.Opening, func(i, j int) bool {
			a, b := place.Opening[i], place.Opening[j]
			if a.Weekday != b.Weekday {
				return a.Weekday < b.Weekday
			}
			return a.Opens < b.Opens
		})
		for i := 1; i < len(place.Opening); i++ {
			a, b := place.Opening[i-1], place.Opening[i]
			if a.Weekday == b.Weekday && a.Closes > b.Opens {
				return catalog, errors.New("overlapping opening intervals")
			}
		}
		seen[place.ID] = true
	}
	sort.Slice(catalog.Places, func(i, j int) bool { return catalog.Places[i].ID < catalog.Places[j].ID })
	return catalog, nil
}

func Open(directory string, key ed25519.PublicKey) (Catalog, error) {
	var input sealed
	file, err := os.Open(filepath.Join(directory, "catalog.json"))
	if err != nil {
		return Catalog{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 3*maxCatalog+1))
	if err != nil || len(raw) > 3*maxCatalog {
		return Catalog{}, errors.New("catalog storage unavailable")
	}
	if err := decodeStrict(raw, &input); err != nil {
		return Catalog{}, err
	}
	return parse(key, input)
}

func Install(directory string, key ed25519.PublicKey, raw, signature []byte, now time.Time) error {
	input := sealed{raw, signature}
	next, err := parse(key, input)
	if err != nil {
		return err
	}
	if next.UpdatedAt.After(now) {
		return errors.New("catalog claimed a future update")
	}
	guard, err := os.OpenFile(filepath.Join(directory, "catalog.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer guard.Close()
	if err := syscall.Flock(int(guard.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	old, err := Open(directory, key)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && (next.Version <= old.Version || next.UpdatedAt.Before(old.UpdatedAt)) {
		return errors.New("catalog version or source time regressed")
	}
	file, err := os.CreateTemp(directory, ".catalog-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	err = json.NewEncoder(file).Encode(input)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}
	if err := os.Rename(name, filepath.Join(directory, "catalog.json")); err != nil {
		return err
	}
	parent, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

func (c Catalog) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/places", func(w http.ResponseWriter, r *http.Request) {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(r.URL.RawQuery) > 192 || len(query["region"]) != 1 || !slug.MatchString(query.Get("region")) ||
			len(query["view"]) > 1 || (query.Has("view") && query.Get("view") != "list" && query.Get("view") != "map") ||
			len(query) != 1+len(query["view"]) {
			http.Error(w, "invalid region search", http.StatusBadRequest)
			return
		}
		result := c
		result.Places = make([]Place, 0)
		for _, place := range c.Places {
			if place.Region == query.Get("region") {
				result.Places = append(result.Places, place)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=60")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("GET /v1/region-options", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			http.Error(w, "invalid region request", http.StatusBadRequest)
			return
		}
		type option struct {
			Code string `json:"code"`
			Name string `json:"name"`
		}
		set := make(map[string]string)
		for _, place := range c.Places {
			name := place.RegionName
			if name == "" {
				name = place.Region
			}
			set[place.Region] = name
		}
		options := make([]option, 0, len(set))
		for code, name := range set {
			options = append(options, option{code, name})
		}
		sort.Slice(options, func(i, j int) bool { return options[i].Code < options[j].Code })
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=60")
		_ = json.NewEncoder(w).Encode(options)
	})
	mux.HandleFunc("GET /v1/regions", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			http.Error(w, "invalid region request", http.StatusBadRequest)
			return
		}
		set := make(map[string]bool)
		for _, place := range c.Places {
			set[place.Region] = true
		}
		regions := make([]string, 0, len(set))
		for region := range set {
			regions = append(regions, region)
		}
		sort.Strings(regions)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(regions)
	})
}
