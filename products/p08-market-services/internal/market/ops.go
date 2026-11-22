package market

import (
	"context"
	"html/template"
	"math/big"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type operatorView struct {
	State, Deployment, Venue, ChainClock string
	Height, Observed                     uint64
	Prices                               []operatorPrice
	Jobs                                 []WorkStatus
	Current                              map[string]string
	Duplicates                           *int64
	DatabaseUnavailable                  bool
}

type operatorPrice struct {
	Value, State  string
	Observed, Age uint64
}

func operatorHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func buildOperatorView(ctx context.Context, chain *Chain, db *pgxpool.Pool) operatorView {
	p := chain.Profile
	out := operatorView{State: "chain-unavailable", Deployment: p.BlockHash.Hex(), Venue: p.Venue.Address.Hex(), Current: map[string]string{}}
	databaseCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	out.Jobs, out.Duplicates = readOperatorWork(databaseCtx, db, p)
	cancel()
	out.DatabaseUnavailable = out.Duplicates == nil
	chainCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	head, err := chain.Head(chainCtx)
	if err != nil {
		return out
	}
	out.Height, out.Observed = head.Number.Uint64(), head.Time
	out.ChainClock = tipClock(head.Time, time.Now())
	out.State = "price-unavailable"
	for i := int64(0); i < 2; i++ {
		fields, err := chain.Venue(chainCtx, head.Hash(), "observations", big.NewInt(i))
		if err != nil {
			out.State = "chain-unavailable"
			return out
		}
		seen := fields[1].(*big.Int).Uint64()
		source := operatorPrice{Value: fields[0].(*big.Int).String(), Observed: seen, State: "missing"}
		if seen > head.Time {
			source.State = "future"
		} else if seen != 0 {
			source.Age = head.Time - seen
			source.State = "fresh"
			if source.Age > 120 {
				source.State = "stale"
			}
		}
		out.Prices = append(out.Prices, source)
	}
	if _, err := chain.Venue(chainCtx, head.Hash(), "validPrice"); err == nil {
		out.State = "ready"
		if out.ChainClock != "current" {
			out.State = "head-clock-" + out.ChainClock
		}
	} else if out.Prices[0].State == "fresh" && out.Prices[1].State == "fresh" {
		out.State = "price-disagreement"
	}
	if flags, err := chain.Venue(chainCtx, head.Hash(), "paused"); err != nil {
		out.State = "chain-unavailable"
	} else if flags[0].(bool) {
		out.State = "paused"
	}
	for _, job := range out.Jobs {
		if _, seen := out.Current[job.Position]; seen {
			continue
		}
		out.Current[job.Position] = "unknown"
		id, err := units(job.Position)
		if err != nil {
			continue
		}
		position, err := chain.Position(chainCtx, head.Hash(), id)
		if err == nil && position.Revision != "0" {
			out.Current[job.Position] = position.Revision
			if position.Closed {
				out.Current[job.Position] += " · closed"
			}
		}
	}
	return out
}

func tipClock(at uint64, wall time.Time) string {
	if wall.Unix() < 1 {
		return "unavailable"
	}
	now := uint64(wall.Unix())
	if at > now && at-now > 10 {
		return "ahead"
	}
	if now > at && now-at > 30 {
		return "stalled"
	}
	return "current"
}

func readOperatorWork(ctx context.Context, db *pgxpool.Pool, p Profile) ([]WorkStatus, *int64) {
	work, err := WorkHistory(ctx, db, p)
	if err != nil {
		return nil, nil
	}
	var duplicates int64
	err = db.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT position,kind,period FROM market.keeper_attempt WHERE `+venueFilter+` AND state='confirmed'
		GROUP BY position,kind,period HAVING count(*) > 1) duplicate_jobs`, p.Genesis.Bytes(), p.Venue.Address.Bytes(), p.BlockHash.Bytes()).Scan(&duplicates)
	if err != nil {
		return work, nil
	}
	return work, &duplicates
}

var operatorTemplate = template.Must(template.New("ops").Parse(`<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>P08 Market Operations</title>
{{template "styles"}}
<main>{{template "observation" .}}</main></html>
{{define "styles"}}<style>
:root{color-scheme:light dark}body{font:16px system-ui;max-width:76rem;margin:2rem auto;padding:0 1rem}p{overflow-wrap:anywhere}
table{width:100%;border-collapse:collapse}th,td{text-align:left;padding:.55rem;border-bottom:1px solid gray;vertical-align:top}td code{overflow-wrap:anywhere}
.scroll{overflow-x:auto}small{font-size:.9rem}
button{font:inherit;padding:.45rem .75rem}
</style>{{end}}
{{define "console"}}<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>P08 Market Operations</title>
{{template "styles"}}
<script defer src="/operator.js"></script>
<main>
<section id="authorization" role="status">
<h1 id="access-title">Checking market operator access</h1>
<p id="access-message">Price and keeper evidence requires an approved market operator grant.</p>
</section>
<section id="observation" hidden></section>
<button id="recheck" type="button" hidden>Check operator access again</button>
<p><a href="nu54://market-learning" rel="noreferrer">Return to market learning</a>. Requires the installed NU54 app. You can also open the app and select Test Market Learning.</p>
<noscript><p>Enable JavaScript to verify access and keep operator evidence current in this tab.</p></noscript>
</main></html>{{end}}
{{define "observation"}}<h1>P08 Market Operations</h1>
<p>Isolated test chain 8283 · <strong id="market-state">{{.State}}</strong></p>
<p>Deployment <code>{{.Deployment}}</code><br>Venue <code>{{.Venue}}</code></p>
{{if .Height}}<p>Observed block {{.Height}} · chain timestamp {{.Observed}} · head clock <strong>{{.ChainClock}}</strong></p>{{else}}<p>Live chain unavailable. Keeper records show their last persisted state.</p>{{end}}
<h2>Price sources</h2>
{{if .Prices}}<div class="scroll"><table><thead><tr><th>Source</th><th>LAQ base units per LAB</th><th>Observed timestamp</th><th>Age at observed block (s)</th><th>State</th></tr></thead><tbody>
{{range $i,$price := .Prices}}<tr><th scope="row">Source {{if $i}}B{{else}}A{{end}}</th><td>{{.Value}}</td><td>{{if .Observed}}{{.Observed}}{{else}}—{{end}}</td><td>{{if .Observed}}{{.Age}}{{else}}—{{end}}</td><td>{{.State}}</td></tr>{{end}}
</tbody></table></div>{{else}}<p>No price observation available.</p>{{end}}
<h2>Keeper reconciliation</h2>
{{if .DatabaseUnavailable}}<p>Keeper counters unavailable.</p>{{else}}<p>Duplicate work keys with multiple confirmed effects: <strong id="duplicate-count">{{.Duplicates}}</strong></p>{{end}}
{{if .Jobs}}<div class="scroll"><table><thead><tr><th>Position</th><th>Operation / period</th><th>Expected revision / current</th><th>Price version</th><th>Original transaction hash</th><th>Last persisted state</th></tr></thead><tbody>
{{range .Jobs}}<tr><th scope="row">{{.Position}}</th><td>{{.Kind}} / {{.Period}}</td><td>{{.Revision}} / {{index $.Current .Position}}</td><td>{{.Version}}</td>
<td><code>{{.Hash.Hex}}</code></td><td>{{.State}}{{if .Block}} · block {{.Block}}<br><small>{{.BlockHash.Hex}}</small>{{end}}</td></tr>{{end}}
</tbody></table></div>{{else}}<p>No keeper attempts in this deployment snapshot.</p>{{end}}
{{end}}`))
