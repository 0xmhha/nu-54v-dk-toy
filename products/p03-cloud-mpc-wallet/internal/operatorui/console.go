// Package operatorui serves the recovery operations console on a separate HTTPS origin.
package operatorui

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/products/p03-cloud-mpc-wallet/internal/wallet"
	"github.com/google/uuid"
)

//go:embed console.css
var stylesheet string

const sessionCookie = "__Host-nu54-recovery-ops"

type session struct {
	token, verifier, code, link string
	until                       time.Time
}

type console struct {
	origin, issuer string
	remote         *http.Client
	api            http.Handler
	verify         func(string) (time.Time, error)
	mu             sync.Mutex
	sessions       map[string]session
}

func secureOrigin(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}

func sameHost(first, second string) bool {
	a, e1 := url.Parse(first)
	b, e2 := url.Parse(second)
	return e1 == nil && e2 == nil && strings.EqualFold(strings.TrimSuffix(a.Hostname(), "."), strings.TrimSuffix(b.Hostname(), "."))
}

func New(origin, issuer string, remote *http.Client, api http.Handler, verify func(string) (time.Time, error)) (http.Handler, error) {
	if !secureOrigin(origin) || !secureOrigin(issuer) || sameHost(origin, issuer) || remote == nil || api == nil || verify == nil {
		return nil, errors.New("invalid recovery console configuration")
	}
	// Never forward a device code or verifier to a redirected origin.
	client := *remote
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c := &console{origin: origin, issuer: issuer, remote: &client, api: api, verify: verify, sessions: make(map[string]session)}
	return c, nil
}

func secret() string {
	var b [32]byte
	rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func (c *console) replace(w http.ResponseWriter, old string, next session) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if old != "" {
		if previous, ok := c.sessions[old]; !ok || !now.Before(previous.until) {
			return false
		}
		delete(c.sessions, old)
	}
	for k, v := range c.sessions {
		if !now.Before(v.until) {
			delete(c.sessions, k)
		}
	}
	if len(c.sessions) >= 512 {
		return false
	}
	id := secret()
	c.sessions[id] = next
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: id, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(time.Until(next.until).Seconds()) + 1})
	return true
}

func (c *console) lookup(r *http.Request) (string, session) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", session{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.sessions[cookie.Value]
	if !ok || !time.Now().Before(s.until) {
		delete(c.sessions, cookie.Value)
		return "", session{}
	}
	return cookie.Value, s
}

func (c *console) authReply(ctx context.Context, path string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.remote.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return errors.New("independent realm response unavailable")
	}
	return json.NewDecoder(io.LimitReader(res.Body, 16385)).Decode(out)
}

func (c *console) begin(w http.ResponseWriter, r *http.Request, id string) {
	verifier := secret()
	hash := sha256.Sum256([]byte(verifier))
	var out struct {
		Code    string `json:"deviceCode"`
		URL     string `json:"verificationURL"`
		Seconds int    `json:"expiresIn"`
	}
	err := c.authReply(r.Context(), "/p03/device/start", map[string]any{"request": map[string]string{"action": "operator", "audience": wallet.Audience},
		"codeChallenge": base64.RawURLEncoding.EncodeToString(hash[:]), "codeChallengeMethod": "S256"}, &out)
	link, parseErr := url.Parse(out.URL)
	if err != nil || parseErr != nil || link.Scheme+"://"+link.Host != c.issuer || link.User != nil || link.Fragment != "" || len(out.Code) < 24 || len(out.Code) > 256 || out.Seconds < 15 || out.Seconds > 300 {
		c.render(w, 503, page{Message: "독립 운영자 인증에 연결할 수 없습니다. 다시 시작하세요."})
		return
	}
	next := session{verifier: verifier, code: out.Code, link: link.String(), until: time.Now().Add(time.Duration(out.Seconds) * time.Second)}
	if !c.replace(w, id, next) {
		c.render(w, 503, page{Message: "인증 세션을 시작할 수 없습니다."})
		return
	}
	c.render(w, 200, page{Link: next.link})
}

func (c *console) finish(w http.ResponseWriter, r *http.Request, id string, s session) {
	if s.code == "" {
		c.render(w, 401, page{Message: "운영자 인증을 시작하세요."})
		return
	}
	var out struct{ State, Token string }
	err := c.authReply(r.Context(), "/p03/device/poll", map[string]string{"deviceCode": s.code, "codeVerifier": s.verifier}, &out)
	if active, _ := c.lookup(r); active != id {
		c.render(w, 401, page{Message: "다시 인증한 뒤 결과를 조회하세요."})
		return
	}
	if err != nil {
		c.render(w, 503, page{Link: s.link, Message: "확인 결과를 가져올 수 없습니다. 같은 인증을 다시 확인하세요."})
		return
	}
	if out.State == "pending" {
		c.render(w, 200, page{Link: s.link, Message: "독립 인증을 마친 뒤 결과를 확인하세요."})
		return
	}
	until, err := c.verify(out.Token)
	if out.State != "ready" || err != nil || !time.Now().Before(until) || !c.replace(w, id, session{token: out.Token, until: until}) {
		c.render(w, 401, page{Message: "복구 운영 권한이 필요합니다."})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (c *console) discard(w http.ResponseWriter, id string) {
	c.mu.Lock()
	delete(c.sessions, id)
	c.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

func (c *console) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method == http.MethodPost {
		if r.Header.Get("Origin") != c.origin {
			c.render(w, 403, page{Message: "이 운영 화면에서 다시 시도하세요."})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := r.ParseForm(); err != nil {
			c.render(w, 400, page{Message: "입력을 확인하세요."})
			return
		}
	}
	id, s := c.lookup(r)
	switch r.Method + " " + r.URL.Path {
	case "GET /operator.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = io.WriteString(w, stylesheet)
	case "GET /":
		if s.token != "" {
			if _, err := c.verify(s.token); err != nil {
				c.discard(w, id)
				c.render(w, 401, page{Message: "다시 인증한 뒤 결과를 조회하세요."})
				return
			}
			c.render(w, 200, page{SignedIn: true})
			return
		}
		c.render(w, 200, page{Link: s.link})
	case "POST /login":
		c.begin(w, r, id)
	case "POST /session":
		c.finish(w, r, id, s)
	case "POST /logout":
		c.discard(w, id)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	case "GET /status", "POST /incident":
		c.inspect(w, r, id, s)
	default:
		http.NotFound(w, r)
	}
}

func (c *console) inspect(w http.ResponseWriter, r *http.Request, id string, s session) {
	if s.token == "" {
		c.render(w, 401, page{Message: "별도 복구 운영자 인증이 필요합니다."})
		return
	}
	if _, err := c.verify(s.token); err != nil {
		c.discard(w, id)
		c.render(w, 401, page{Message: "운영 권한이 만료됐습니다."})
		return
	}
	ref := r.FormValue("wallet")
	parsed, err := uuid.Parse(ref)
	if err != nil || parsed == uuid.Nil || parsed.String() != ref {
		c.render(w, 400, page{SignedIn: true, Message: "지갑 참조를 확인하세요."})
		return
	}
	path := "/v1/recovery/status?wallet=" + url.QueryEscape(ref)
	var body []byte
	if r.Method == http.MethodPost {
		if r.PostForm.Get("confirm") != "yes" {
			c.render(w, 400, page{SignedIn: true, Message: "정족수 유출과 새 서명 동결을 확인하세요."})
			return
		}
		path = "/v1/recovery/incidents"
		body, _ = json.Marshal(map[string]string{"wallet": ref, "kind": "quorum_exposure"})
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, path, bytes.NewReader(body))
	if err != nil {
		c.render(w, 500, page{SignedIn: true})
		return
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	rec := httptest.NewRecorder()
	c.api.ServeHTTP(rec, req)
	active, latest := c.lookup(r)
	if active != id || latest.token != s.token {
		c.render(w, 401, page{Message: "다시 인증한 뒤 결과를 조회하세요."})
		return
	}
	if _, err := c.verify(s.token); err != nil {
		c.discard(w, id)
		c.render(w, 401, page{Message: "다시 인증한 뒤 결과를 조회하세요."})
		return
	}
	if rec.Code != 200 {
		msg := "현재 상태를 확인할 수 없습니다. 동일 지갑을 다시 조회하세요."
		if rec.Code == 401 || rec.Code == 403 {
			msg = "이 지갑의 운영 권한을 확인할 수 없습니다."
		}
		c.render(w, rec.Code, page{SignedIn: true, Message: msg})
		return
	}
	var state wallet.RecoveryStatus
	if json.Unmarshal(rec.Body.Bytes(), &state) != nil || state.Wallet != ref {
		c.render(w, 502, page{SignedIn: true})
		return
	}
	if r.Method == http.MethodPost {
		http.Redirect(w, r, "/status?wallet="+url.QueryEscape(ref)+"&view=breach", http.StatusSeeOther)
		return
	}
	view := r.URL.Query().Get("view")
	switch view {
	case "outage", "breach", "audit":
	default:
		view = "status"
	}
	c.render(w, 200, page{SignedIn: true, State: &state, View: view})
}

type page struct {
	SignedIn            bool
	Link, Message, View string
	State               *wallet.RecoveryStatus
}

func (c *console) render(w http.ResponseWriter, status int, data page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = layout.Execute(w, data)
}

var layout = template.Must(template.New("operator").Funcs(template.FuncMap{
	"stamp": func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04:05 UTC") },
	"availability": func(s string) string {
		switch s {
		case "reachable":
			return "응답 확인"
		case "unavailable":
			return "응답 미확인"
		default:
			return "확인 전"
		}
	},
}).Parse(strings.TrimSpace(`
<!doctype html><html lang="ko"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>클라우드 지갑 복구 운영</title><link rel="stylesheet" href="/operator.css"></head>
<body><main><h1>클라우드 지갑 복구 운영</h1>
{{if .Message}}<p role="status">{{.Message}}</p>{{end}}
{{if .SignedIn}}
<form method="get" action="/status"><label>지갑 참조 <input name="wallet" required maxlength="36" {{with .State}}value="{{.Wallet}}"{{end}}></label><button>서비스 상태 조회</button></form>
<form method="post" action="/logout"><button>운영자 로그아웃</button></form>
{{with .State}}<nav aria-label="복구 운영"><a href="/status?wallet={{.Wallet}}">사고 분류</a> · <a href="/status?wallet={{.Wallet}}&amp;view=outage">서비스 장애</a> ·
<a href="/status?wallet={{.Wallet}}&amp;view=breach">정족수 유출 신고</a> · <a href="/status?wallet={{.Wallet}}&amp;view=audit">감사 기록</a></nav>
<p>지갑 {{.Wallet}} · 세대 {{.Epoch}} · 정책 {{.Policy}}</p>
<p>서비스 서명 — {{if .QuorumExposure}}사고로 동결{{else if eq .State "active"}}활성 정책 적용{{else}}사용 대기 또는 종료{{end}}</p>{{end}}
{{if eq .View "status"}}<h2>참여자 서비스 상태</h2><ul>{{range .State.Participants}}<li>참여자 {{.Role}} — {{availability .State}} · {{stamp .CheckedAt}}</li>{{else}}<li>활성 참여자 확인 정보 없음</li>{{end}}</ul>{{end}}
{{if eq .View "outage"}}<h2>검증된 대체 경로</h2>
<p>A+C 우회 — 사용 불가. 기존 정책을 유지하고 서비스 복구를 기다리세요.</p>
<p>최근 세션 {{.State.LatestSession}}</p><a href="/status?wallet={{.State.Wallet}}&amp;view=outage">서비스 재조회</a>
<ul>{{range .State.Participants}}<li>참여자 {{.Role}} — {{availability .State}} · {{stamp .CheckedAt}}</li>{{end}}</ul>{{end}}
{{if eq .View "breach"}}<h2>과거 정족수 자료 유출 신고</h2><p>일반 세대 갱신 뒤에도 과거 키는 외부에서 사용될 수 있습니다. 새 키와 사용자가 동의한 자산 이전, 기존 권한 회수를 별도로 검토하세요.</p>
<p>자동 자산 이동 · {{.State.AssetMoves}}건</p>
{{if .State.QuorumExposure}}<p>새 서비스 서명 동결 · 과거 키 노출 위험 유지</p>{{with .State.IncidentRef}}<p>사고 참조 {{.}}</p>{{end}}{{else}}
<form method="post" action="/incident"><input type="hidden" name="wallet" value="{{.State.Wallet}}"><label><input type="checkbox" name="confirm" value="yes" required> 정족수 유출을 신고하고 새 서비스 서명을 동결합니다.</label><button>신고 및 동결</button></form>{{end}}{{end}}
{{if eq .View "audit"}}<h2>감사 타임라인</h2><ul>{{range .State.Evidence}}<li>{{stamp .At}} · {{.Kind}} · 기록 {{.Reference}}{{if .Operation}} · 세션 {{.Operation}}{{end}}{{if .Digest}} · 증거 digest {{.Digest}}{{end}}</li>{{else}}<li>기록 없음</li>{{end}}</ul>{{end}}
{{else}}{{if .Link}}<p><a href="{{.Link}}" target="_blank" rel="noopener noreferrer">독립 운영자 확인 열기</a></p><form method="post" action="/session"><button>인증 결과 확인</button></form>{{end}}
<form method="post" action="/login"><button>별도 복구 운영자 인증</button></form>{{end}}
</main></body></html>
`)))
