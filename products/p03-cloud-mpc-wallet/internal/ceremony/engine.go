package ceremony

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"errors"
	"math/big"
	"slices"
	"strings"
	"time"

	"github.com/bnb-chain/tss-lib/v4/common"
	"github.com/bnb-chain/tss-lib/v4/ecdsa/keygen"
	"github.com/bnb-chain/tss-lib/v4/ecdsa/resharing"
	"github.com/bnb-chain/tss-lib/v4/ecdsa/signing"
	"github.com/bnb-chain/tss-lib/v4/tss"
)

type engine struct {
	session     Session
	id          string
	party       tss.Party
	ids         map[string]*tss.PartyID
	out         chan tss.Message
	shares      chan *keygen.LocalPartySaveData
	signatures  chan *common.SignatureData
	fingerprint [32]byte
}

func partyIDs(peers []Peer, epoch uint64) tss.SortedPartyIDs {
	var ids []*tss.PartyID
	for _, peer := range peers {
		if peer.Epoch == epoch {
			hash := sha256.Sum256([]byte("nu54/p03/party/v1\x00" + peer.ID))
			ids = append(ids, tss.NewPartyID(peer.ID, string(peer.Role), new(big.Int).SetBytes(hash[:])))
		}
	}
	return tss.SortPartyIDs(ids)
}

func newEngine(s Session, id string, fingerprint, secret [32]byte, pre *keygen.LocalPreParams, share *keygen.LocalPartySaveData) (*engine, error) {
	newIDs := partyIDs(s.Peers, s.Epoch)
	e := &engine{
		session: s, id: id, fingerprint: fingerprint, ids: make(map[string]*tss.PartyID),
		out: make(chan tss.Message, 64), shares: make(chan *keygen.LocalPartySaveData, 1),
		signatures: make(chan *common.SignatureData, 1),
	}
	for _, p := range newIDs {
		e.ids[p.Id] = p
	}
	curve := tss.S256()
	sessionNonce := new(big.Int).SetBytes(secret[:])
	current := tss.NewPeerContext(newIDs)
	switch s.Kind {
	case DKG:
		if e.ids[id] == nil || pre == nil || !pre.ValidateWithProof() {
			return nil, errors.New("local DKG parameters unavailable")
		}
		params := tss.NewParameters(curve, current, e.ids[id], len(newIDs), 1)
		params.SetSessionNonce(sessionNonce)
		e.party = keygen.NewLocalParty(params, e.out, e.shares, *pre)
	case Sign:
		if e.ids[id] == nil || !shareContains(share, newIDs) {
			return nil, errors.New("local signing share unavailable")
		}
		params := tss.NewParameters(curve, current, e.ids[id], len(newIDs), 1)
		params.SetSessionNonce(sessionNonce)
		// The upstream interface accepts a scalar; the full length retains leading zero bytes.
		digest := new(big.Int).SetBytes(s.Digest[:])
		if digest.Cmp(params.EC().Params().N) >= 0 {
			return nil, errors.New("signing digest exceeds supported range")
		}
		e.party = signing.NewLocalParty(digest, params, *share, e.out, e.signatures, len(s.Digest))
	case Refresh:
		oldIDs := partyIDs(s.Peers, s.Epoch-1)
		next := e.ids[id] != nil
		for _, p := range oldIDs {
			e.ids[p.Id] = p
		}
		if e.ids[id] == nil {
			return nil, errors.New("local participant is absent")
		}
		if next {
			if pre == nil || !pre.ValidateWithProof() {
				return nil, errors.New("replacement parameters unavailable")
			}
			save := keygen.NewLocalPartySaveData(len(newIDs))
			save.LocalPreParams = *pre
			share = &save
		} else if !shareContains(share, oldIDs) {
			return nil, errors.New("old committee share unavailable")
		}
		params := tss.NewReSharingParameters(curve, tss.NewPeerContext(oldIDs), current, e.ids[id], len(oldIDs), 1, len(newIDs), 1)
		params.SetSessionNonce(sessionNonce)
		e.party = resharing.NewLocalParty(params, *share, e.out, e.shares)
	default:
		return nil, errors.New("unknown ceremony")
	}
	return e, nil
}

func shareContains(share *keygen.LocalPartySaveData, ids tss.SortedPartyIDs) bool {
	if share == nil || share.Xi == nil || share.ShareID == nil || share.ECDSAPub == nil {
		return false
	}
	for _, id := range ids {
		if !slices.ContainsFunc(share.Ks, func(k *big.Int) bool { return k != nil && k.Cmp(id.KeyInt()) == 0 }) {
			return false
		}
	}
	return true
}

func (e *engine) encode(msg tss.Message) ([]Frame, error) {
	body, routing, err := msg.WireBytes()
	if err != nil {
		return nil, err
	}
	to := routing.To
	if len(to) == 0 && routing.IsBroadcast {
		for _, p := range e.session.Peers {
			to = append(to, e.ids[p.ID])
		}
	}
	frames := make([]Frame, 0, len(to))
	for _, recipient := range to {
		if recipient.Id != e.id {
			frames = append(frames, Frame{Session: e.fingerprint, From: e.id, To: recipient.Id, Round: msg.Type(), Broadcast: routing.IsBroadcast, Data: body})
		}
	}
	return frames, nil
}

func (e *engine) parse(frame Frame) (tss.ParsedMessage, error) {
	source := frame.From
	if frame.Source != "" {
		source = frame.Source
	}
	if frame.Session != e.fingerprint || e.ids[source] == nil {
		return nil, errors.New("frame outside session or round")
	}

	recipients, valid := e.route(source, frame.Round, frame.Broadcast)
	if !valid || !slices.Contains(recipients, frame.To) || frame.Source != "" && !slices.Contains(recipients, frame.From) {
		return nil, errors.New("frame outside round routing")
	}
	if frame.Source != "" {
		return nil, nil
	}

	msg, err := tss.ParseWireMessage(frame.Data, e.ids[source], frame.Broadcast)
	if err != nil || msg.Type() != frame.Round || !msg.ValidateBasic() {
		return nil, errors.New("invalid round payload")
	}
	return msg, nil
}

func prepare(ctx context.Context) (*keygen.LocalPreParams, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	return keygen.GeneratePreParamsWithContext(ctx, 2)
}

func publicKey(share *keygen.LocalPartySaveData) []byte {
	if share == nil || share.ECDSAPub == nil {
		return nil
	}
	return elliptic.Marshal(tss.S256(), share.ECDSAPub.X(), share.ECDSAPub.Y())
}

func VerifySignature(public []byte, digest [32]byte, der []byte) bool {
	x, y := elliptic.Unmarshal(tss.S256(), public)
	return x != nil && ecdsa.VerifyASN1(&ecdsa.PublicKey{Curve: tss.S256(), X: x, Y: y}, digest[:], der)
}

type direction struct {
	sender, recipient string
	broadcast         bool
}

var rounds = map[Kind]map[string]direction{
	DKG: {
		"KGRound1Message": {"all", "all", true}, "KGRound2Message1": {"all", "all", false},
		"KGRound2Message2": {"all", "all", true}, "KGRound3Message": {"all", "all", true},
	},
	Sign: {
		"SignRound1Message1": {"all", "all", false}, "SignRound1Message2": {"all", "all", true},
		"SignRound2Message": {"all", "all", false}, "SignRound3Message": {"all", "all", true},
		"SignRound4Message": {"all", "all", true}, "SignRound5Message": {"all", "all", true},
		"SignRound6Message": {"all", "all", true}, "SignRound7Message": {"all", "all", true},
		"SignRound8Message": {"all", "all", true}, "SignRound9Message": {"all", "all", true},
	},
	Refresh: {
		"DGRound1Message": {"old", "new", true}, "DGRound2Message1": {"new", "new", true},
		"DGRound2Message2": {"new", "old", true}, "DGRound3Message1": {"old", "new", false},
		"DGRound3Message2": {"old", "new", true}, "DGRound4Message1": {"new", "new", false},
		"DGRound4Message2": {"new", "all", true},
	},
}
var stages = map[Kind]string{DKG: "keygen", Sign: "signing", Refresh: "resharing"}

func (e *engine) route(source, round string, broadcast bool) ([]string, bool) {
	name, ok := strings.CutPrefix(round, "binance.tsslib.ecdsa."+stages[e.session.Kind]+".")
	if !ok {
		return nil, false
	}
	route, ok := rounds[e.session.Kind][name]
	if !ok || route.broadcast != broadcast {
		return nil, false
	}
	in := func(p Peer, group string) bool {
		return group == "all" || group == "new" && p.Epoch == e.session.Epoch ||
			group == "old" && p.Epoch == e.session.Epoch-1
	}
	var recipients []string
	var validSource bool
	for _, peer := range e.session.Peers {
		if peer.ID == source {
			validSource = in(peer, route.sender)
		}
		if in(peer, route.recipient) {
			recipients = append(recipients, peer.ID)
		}
	}
	return recipients, validSource
}
