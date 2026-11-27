package records

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type vault struct {
	aead      cipher.AEAD
	digestKey []byte
}

func newVault(secret []byte) (*vault, error) {
	if len(secret) != 32 {
		return nil, errors.New("32-byte P09 private record key required")
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	derive := hmac.New(sha256.New, secret)
	derive.Write([]byte("NU54-P09-record-idempotency-v1"))
	return &vault{aead: aead, digestKey: derive.Sum(nil)}, nil
}
func (s *Service) EnableVault(secret []byte) error {
	v, err := newVault(secret)
	if err == nil {
		s.content = v
	}
	return err
}
func (v *vault) seal(scope string, body []byte) ([]byte, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return v.aead.Seal(nonce, nonce, body, []byte(scope)), nil
}
func (v *vault) open(scope string, body []byte) ([]byte, error) {
	size := v.aead.NonceSize()
	if len(body) < size+v.aead.Overhead() {
		return nil, errors.New("invalid encrypted record")
	}
	return v.aead.Open(nil, body[:size], body[size:], []byte(scope))
}
func (v *vault) dedup(scope string, body []byte) []byte {
	mac := hmac.New(sha256.New, v.digestKey)
	mac.Write([]byte(scope))
	mac.Write([]byte{0})
	mac.Write(body)
	return mac.Sum(nil)
}

type Record struct {
	ID       string    `json:"id"`
	Purpose  string    `json:"purpose"`
	Revision int64     `json:"consentRevision"`
	Parent   *string   `json:"parent,omitempty"`
	DeleteAt time.Time `json:"deleteAfter"`
}
type RecordInput struct {
	Record
	Recipient string `json:"recipient"`
	Data      string `json:"data"`
}

func validRecordData(purpose string, body []byte) bool {
	if purpose == "voice" {
		return len(body) >= 16 && string(body[4:8]) == "ftyp" &&
			(string(body[8:12]) == "isom" || string(body[8:12]) == "M4A " || string(body[8:12]) == "mp42" || string(body[8:12]) == "mp41")
	}
	if len(body) > 128 {
		return false
	}
	var position struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	}
	input := json.NewDecoder(bytes.NewReader(body))
	input.DisallowUnknownFields()
	if err := input.Decode(&position); err != nil || position.Latitude == nil || position.Longitude == nil {
		return false
	}
	_, err := input.Token()
	return err == io.EOF && *position.Latitude >= -90 && *position.Latitude <= 90 && *position.Longitude >= -180 && *position.Longitude <= 180
}

func (s *Service) records(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.principal(r)
	if !ok || actor.Role != "private-user" || s.content == nil || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	if r.Method == http.MethodPost {
		var item struct {
			RecordInput
			Projection string `json:"projection,omitempty"`
		}
		if !strictBody(w, r, 1_400_000, &item) {
			reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_optional_record"})
			return
		}
		if item.Projection == "saved-itineraries" && item.RecordInput == (RecordInput{}) {
			plans, err := s.itinerarySummaries(ctx, actor.Subject)
			if err != nil {
				authFailure(w, err)
				return
			}
			if current, ok := s.principal(r); !ok || current != actor {
				http.NotFound(w, r)
				return
			}
			reply(w, http.StatusOK, map[string][]itinerarySummary{"itineraries": plans})
			return
		}
		if item.Projection != "" || !uuid.MatchString(item.ID) || !recipientName.MatchString(item.Recipient) ||
			(item.Parent != nil && !uuid.MatchString(*item.Parent)) || item.Revision < 1 || item.Revision > 9007199254740991 ||
			(item.Purpose != "voice" && item.Purpose != "location") {
			reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_optional_record"})
			return
		}
		body, err := base64.StdEncoding.Strict().DecodeString(item.Data)
		if err != nil || len(body) == 0 || len(body) > 1<<20 || !validRecordData(item.Purpose, body) {
			reply(w, http.StatusBadRequest, map[string]string{"error": "invalid_optional_record"})
			return
		}
		defer clear(body)
		scope := actor.Subject + ":record:" + item.ID
		sealed, err := s.content.seal(scope, body)
		if err != nil {
			authFailure(w, err)
			return
		}
		var id string
		err = s.mutationRow(r, actor, s.pool, `SELECT travel_private.write_record($1,$2,$3,$4,$5,$6,$7,$8,$9)::text`,
			actor.Subject, item.ID, item.Purpose, item.Revision, item.Recipient, item.Parent, item.DeleteAt,
			s.content.dedup(scope, body), sealed).Scan(&id)
		if err != nil {
			authFailure(w, err)
			return
		}
		reply(w, http.StatusOK, map[string]string{"record": id})
		return
	}
	if id := r.PathValue("id"); id != "" {
		if !uuid.MatchString(id) {
			http.NotFound(w, r)
			return
		}
		var purpose string
		var sealed []byte
		err := s.pool.QueryRow(ctx, `SELECT purpose,sealed FROM travel_private.read_record($1,$2)`, actor.Subject, id).
			Scan(&purpose, &sealed)
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			authFailure(w, err)
			return
		}
		plain, err := s.content.open(actor.Subject+":record:"+id, sealed)
		if err != nil {
			authFailure(w, err)
			return
		}
		defer clear(plain)
		reply(w, http.StatusOK, map[string]string{"id": id, "purpose": purpose, "data": base64.StdEncoding.EncodeToString(plain)})
		return
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text,purpose,revision,parent::text,delete_after FROM travel_private.my_records($1)`, actor.Subject)
	if err != nil {
		authFailure(w, err)
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Record, error) {
		var item Record
		err := row.Scan(&item.ID, &item.Purpose, &item.Revision, &item.Parent, &item.DeleteAt)
		return item, err
	})
	if err != nil {
		authFailure(w, err)
		return
	}
	if list == nil {
		list = []Record{}
	}
	reply(w, http.StatusOK, map[string][]Record{"records": list})
}
