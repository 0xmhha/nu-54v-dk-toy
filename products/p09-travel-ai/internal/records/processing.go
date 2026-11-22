package records

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProcessRequest struct {
	Job       string `json:"job"`
	Source    string `json:"source"`
	Revision  int64  `json:"consentRevision"`
	Recipient string `json:"recipient"`
}

func (s *Service) queue(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.principal(r)
	if !ok || actor.Role != "private-user" || s.content == nil || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	var cmd ProcessRequest
	if !strictBody(w, r, 4096, &cmd) || !uuid.MatchString(cmd.Job) || !uuid.MatchString(cmd.Source) || !recipientName.MatchString(cmd.Recipient) ||
		cmd.Revision < 1 || cmd.Revision > 9007199254740991 {
		reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_processing_job"})
		return
	}
	var id string
	err := s.mutationRow(r, actor, s.pool, `SELECT travel_private.queue_transcription($1,$2,$3,$4,$5)::text`,
		actor.Subject, cmd.Job, cmd.Source, cmd.Revision, cmd.Recipient).Scan(&id)
	if err != nil {
		authFailure(w, err)
		return
	}
	reply(w, http.StatusOK, map[string]string{"job": id})
}

type Worker struct {
	pool      *pgxpool.Pool
	transport *http.Transport
	client    *http.Client
	vault     *vault
	endpoint  string
	recipient string
}

func OpenWorker(ctx context.Context, dsn, endpoint, recipient, pin string, roots *x509.CertPool, clientIdentity tls.Certificate, key []byte) (*Worker, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.Path != "/v1/work" || !recipientName.MatchString(recipient) ||
		!fingerprint.MatchString(pin) || roots == nil || len(clientIdentity.Certificate) == 0 || clientIdentity.PrivateKey == nil {
		return nil, errors.New("approved recipient certificate and endpoint required")
	}
	vault, err := newVault(key)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, errors.New("processing capability unavailable")
	}
	var allowed bool
	err = pool.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p09_processor','USAGE')
	  AND NOT pg_has_role(current_user,'nu54_p09_private_user','MEMBER')
	  AND NOT has_table_privilege('travel_private.record_object','SELECT,INSERT,UPDATE,DELETE')
	  AND NOT EXISTS (SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&allowed)
	if err != nil || !allowed {
		pool.Close()
		return nil, errors.New("processing capability unavailable")
	}
	transport := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, Certificates: []tls.Certificate{clientIdentity},
			VerifyConnection: func(state tls.ConnectionState) error {
				if len(state.PeerCertificates) == 0 {
					return errors.New("recipient certificate unavailable")
				}
				digest := sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo)
				if hex.EncodeToString(digest[:]) != pin {
					return errors.New("recipient certificate mismatch")
				}
				return nil
			}},
	}
	return &Worker{pool: pool, transport: transport, client: &http.Client{Transport: transport, Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
		vault: vault, endpoint: endpoint, recipient: recipient}, nil
}
func (w *Worker) Close() { w.transport.CloseIdleConnections(); w.pool.Close() }

type Work struct {
	Handle  string    `json:"handle"`
	Action  string    `json:"action"`
	Expires time.Time `json:"expiresAt"`
	Sample  string    `json:"sample,omitempty"`
}
type WorkAck struct {
	Handle     string     `json:"handle"`
	Event      string     `json:"event"`
	Ended      *time.Time `json:"endedAt,omitempty"`
	Deleted    *time.Time `json:"deletedAt,omitempty"`
	Transcript string     `json:"transcript,omitempty"`
}

func (w *Worker) Step(ctx context.Context) error {
	var work Work
	var actor, job, source string
	var sealed []byte
	err := w.pool.QueryRow(ctx, `SELECT subject_ref::text,job_id::text,source_token::text,handle::text,action,source,expires_at
	  FROM travel_private.next_processor_work($1)`, w.recipient).Scan(&actor, &job, &source, &work.Handle, &work.Action, &sealed, &work.Expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errors.New("processor work unavailable")
	}
	if work.Action == "send" {
		sample, err := w.vault.open(actor+":record:"+source, sealed)
		if err != nil {
			return errors.New("encrypted recording unavailable")
		}
		work.Sample = base64.StdEncoding.EncodeToString(sample)
		clear(sample)
	}
	payload, err := json.Marshal(work)
	if err != nil {
		return errors.New("processor request unavailable")
	}
	defer clear(payload)
	request, err := http.NewRequestWithContext(ctx, "POST", w.endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("processor request unavailable")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cache-Control", "no-store")
	response, err := w.client.Do(request)
	if err != nil {
		return errors.New("processor did not acknowledge action")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("processor did not acknowledge action")
	}
	var answer WorkAck
	decoder := json.NewDecoder(http.MaxBytesReader(nil, response.Body, 16384))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&answer); err != nil || answer.Handle != work.Handle ||
		(answer.Event != "accepted" && answer.Event != "terminal" && answer.Event != "erased") {
		return errors.New("invalid recipient acknowledgment")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("invalid recipient acknowledgment")
	}
	if answer.Transcript != "" {
		if answer.Event != "terminal" || work.Action == "cancel" || work.Action == "delete" || len(answer.Transcript) > 8000 {
			return errors.New("unexpected processor output")
		}
		raw := []byte(answer.Transcript)
		defer clear(raw)
		scope := actor + ":record:" + job
		written, err := w.vault.seal(scope, raw)
		if err != nil {
			return errors.New("output vault unavailable")
		}
		var stored bool
		err = w.pool.QueryRow(ctx, `SELECT travel_private.store_provider_result($1,$2,$3,$4)`, work.Handle, w.recipient,
			w.vault.dedup(scope, raw), written).Scan(&stored)
		if err != nil {
			return errors.New("processor output unavailable")
		}
	}
	var applied bool
	if err := w.pool.QueryRow(ctx, `SELECT travel_private.provider_ack($1,$2,$3,$4,$5)`,
		work.Handle, w.recipient, answer.Event, answer.Ended, answer.Deleted).Scan(&applied); err != nil {
		return errors.New("processor acknowledgment remains pending")
	}
	return nil
}
