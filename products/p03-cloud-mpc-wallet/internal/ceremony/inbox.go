package ceremony

import (
	"crypto/sha256"
	"errors"
	"slices"
	"sync"
)

type Frame struct {
	Session   [32]byte
	From      string
	To        string
	Round     string
	Broadcast bool
	// Source and Digest are present only on broadcast echoes.
	Source string
	Digest [32]byte
	Data   []byte
}

func (f Frame) Hash() [32]byte { return sha256.Sum256(f.Data) }

type messageKey struct {
	source string
	round  string
}

type broadcast struct {
	body      Frame
	seen      map[string][32]byte
	delivered bool
}

type Inbox struct {
	mu         sync.Mutex
	local      string
	peers      []string
	direct     map[messageKey][32]byte
	broadcasts map[messageKey]*broadcast
}

func NewInbox(local string, peers []string) *Inbox {
	return &Inbox{
		local: local, peers: slices.Clone(peers),
		direct:     make(map[messageKey][32]byte),
		broadcasts: make(map[messageKey]*broadcast),
	}
}

// Receive expects the caller to have already checked the session, round and
// protocol routing and to provide the TLS-authenticated peer ID.
func (i *Inbox) Receive(peer string, frame Frame) (Frame, bool, error) {
	return i.receive(peer, frame, i.peers)
}

func (i *Inbox) receive(peer string, frame Frame, recipients []string) (Frame, bool, error) {
	var empty Frame
	var zero [32]byte
	if peer != frame.From || peer == i.local || frame.To != i.local || !slices.Contains(i.peers, peer) {
		return empty, false, errors.New("unexpected frame route")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	key := messageKey{peer, frame.Round}
	if !frame.Broadcast {
		if frame.Source != "" || frame.Digest != zero || len(frame.Data) == 0 {
			return empty, false, errors.New("invalid unicast")
		}
		digest := frame.Hash()
		if prior, ok := i.direct[key]; ok {
			return empty, false, compareDigest(prior, digest)
		}
		i.direct[key] = digest
		return frame, true, nil
	}
	digest := frame.Digest
	if frame.Source == "" {
		if len(frame.Data) == 0 || digest != zero {
			return empty, false, errors.New("invalid broadcast body")
		}
		digest = frame.Hash()
	} else {
		if frame.Source == peer || frame.Source == i.local || !slices.Contains(i.peers, frame.Source) || !slices.Contains(recipients, peer) || len(frame.Data) != 0 || digest == zero {
			return empty, false, errors.New("invalid broadcast echo")
		}
		key.source = frame.Source
	}
	entry := i.broadcasts[key]
	if entry == nil {
		entry = &broadcast{seen: make(map[string][32]byte)}
		i.broadcasts[key] = entry
	}
	for _, prior := range entry.seen {
		if err := compareDigest(prior, digest); err != nil {
			return empty, false, err
		}
	}
	entry.seen[peer] = digest
	if frame.Source == "" {
		entry.body = frame
	}
	if entry.delivered || len(entry.body.Data) == 0 || len(entry.seen) != len(recipients)-1+broadcastSourceOutside(recipients, key.source) {
		return empty, false, nil
	}
	entry.delivered = true
	return entry.body, true, nil
}

func compareDigest(first, other [32]byte) error {
	if first != other {
		return errors.New("conflicting round message")
	}
	return nil
}

func broadcastSourceOutside(recipients []string, source string) int {
	if slices.Contains(recipients, source) {
		return 0
	}
	return 1
}
