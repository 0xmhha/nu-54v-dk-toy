package ceremony

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
)

type Role string

const (
	A Role = "A"
	B Role = "B"
	C Role = "C"
)

type Kind string

const (
	DKG     Kind = "dkg"
	Sign    Kind = "sign"
	Refresh Kind = "refresh"
)

type Peer struct {
	ID    string
	Role  Role
	Epoch uint64
}

// Session is the public contract a participant verifies before opening a round.
// Digest is the exact signing digest. Intent binds the independently reviewed request.
type Session struct {
	ID         string
	Wallet     string
	Kind       Kind
	Epoch      uint64
	KeySession string
	Expires    int64
	Digest     [32]byte
	Intent     [32]byte
	Peers      []Peer
}

var uuid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var peerID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

func (s Session) Fingerprint(now time.Time) ([32]byte, error) {
	var empty [32]byte
	if !uuid.MatchString(s.ID) || !uuid.MatchString(s.Wallet) || s.Epoch == 0 || now.Unix() >= s.Expires || s.Expires > now.Add(30*time.Minute).Unix() {
		return empty, errors.New("invalid session or expiration")
	}
	if s.Kind == DKG && s.KeySession != "" || s.Kind != DKG && !uuid.MatchString(s.KeySession) {
		return empty, errors.New("invalid source share session")
	}
	if s.Kind == Sign && (s.Digest == empty || s.Intent == empty) || s.Kind != Sign && s.Digest != empty {
		return empty, errors.New("invalid signing digest or intent")
	}
	expected := map[uint64]string{}
	switch s.Kind {
	case DKG:
		expected[s.Epoch] = "ABC"
	case Sign:
		expected[s.Epoch] = "AB"
	case Refresh:
		if s.Epoch < 2 {
			return empty, errors.New("invalid replacement epoch")
		}
		expected[s.Epoch-1], expected[s.Epoch] = "BC", "ABC"
	default:
		return empty, errors.New("unknown ceremony")
	}
	seen := make(map[string]bool)
	for _, p := range s.Peers {
		if !peerID.MatchString(p.ID) || seen[p.ID] || len(p.Role) != 1 || !strings.Contains(expected[p.Epoch], string(p.Role)) {
			return empty, errors.New("unexpected participant")
		}
		seen[p.ID] = true
		expected[p.Epoch] = strings.Replace(expected[p.Epoch], string(p.Role), "", 1)
	}
	for _, missing := range expected {
		if missing != "" {
			return empty, errors.New("missing participants")
		}
	}
	s.Peers = slices.Clone(s.Peers)
	slices.SortFunc(s.Peers, func(a, b Peer) int { return strings.Compare(a.ID, b.ID) })
	encoded, err := json.Marshal(s)
	if err != nil {
		return empty, err
	}
	return sha256.Sum256(append([]byte("nu54/p03/ceremony/v1\x00"), encoded...)), nil
}
