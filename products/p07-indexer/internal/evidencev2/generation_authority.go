package evidencev2

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"
	"github.com/0xmhha/nu-54v-dk-toy/products/p07-indexer/internal/chainview"
	"github.com/ethereum/go-ethereum/common"
)

// RemoteGenerationAuthority uses the configured P05 authority. The authenticated
// request is a lookup only. Its tuple is checked before verifying the board.
type RemoteGenerationAuthority struct {
	Client  *http.Client
	URL     string
	Profile chainview.Profile
	Clock   func() time.Time
}

func (a RemoteGenerationAuthority) GenerationReceiptAuthority(ctx context.Context, grant GenerationGrant, challenge common.Hash) (chainview.GenerationReceiptAuthority, error) {
	denied := chainview.ErrGenerationFreeze
	if a.Client == nil || a.Clock == nil || grant.Stream != a.Profile.StreamID || !fixedLowerHex(grant.ClientFingerprint, 32) ||
		grant.RequestedGeneration == 0 || challenge == (common.Hash{}) {
		return chainview.GenerationReceiptAuthority{}, denied
	}
	query := rentalv1.AuthorityRequest{Stream: grant.Stream, StreamGeneration: a.Profile.GenerationID,
		Branch: grant.Branch, Board: grant.Board, Generation: strconv.FormatUint(grant.RequestedGeneration, 10),
		Challenge: challenge.Hex(), CallerFingerprint: grant.ClientFingerprint}
	raw, err := json.Marshal(query)
	if err != nil {
		return chainview.GenerationReceiptAuthority{}, denied
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.URL, bytes.NewReader(raw))
	if err != nil {
		return chainview.GenerationReceiptAuthority{}, denied
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "no-store")
	reply, err := a.Client.Do(req)
	if err != nil {
		return chainview.GenerationReceiptAuthority{}, denied
	}
	defer reply.Body.Close()
	if reply.StatusCode != http.StatusOK {
		return chainview.GenerationReceiptAuthority{}, denied
	}
	body, err := io.ReadAll(io.LimitReader(reply.Body, 4097))
	if err != nil || len(body) > 4096 {
		return chainview.GenerationReceiptAuthority{}, denied
	}
	var current rentalv1.AuthorityResponse
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&current) != nil || !errors.Is(decoder.Decode(new(any)), io.EOF) || current.AuthorityRequest != query {
		return chainview.GenerationReceiptAuthority{}, denied
	}
	now := a.Clock()
	server, ok := service.CurrentVerifiedClient(reply.TLS, now)
	if !ok || current.ValidUntil > current.ChallengeUntil {
		return chainview.GenerationReceiptAuthority{}, denied
	}
	current.ValidUntil = min(current.ValidUntil, server.ExpiresAt.Unix())
	return authorityForProfile(current, a.Profile, grant.RequestedGeneration, challenge, now)
}

func authorityForProfile(record rentalv1.AuthorityResponse, profile chainview.Profile, generation uint64, challenge common.Hash, now time.Time) (chainview.GenerationReceiptAuthority, error) {
	denied := chainview.ErrGenerationFreeze
	if record.ChainID != profile.ChainID || record.ChainID != 8283 ||
		record.Deployment != "0x"+profile.Manifest || record.Settlement != strings.ToLower(profile.Contract.Hex()) ||
		record.RegisteredAt <= 0 || record.RegisteredAt > now.Unix() ||
		record.ChallengeUntil <= now.Unix() || record.ValidUntil <= now.Unix() ||
		record.ValidUntil > record.ChallengeUntil {
		return chainview.GenerationReceiptAuthority{}, denied
	}
	for _, field := range []string{record.Registration, record.Measurement} {
		if !strings.HasPrefix(field, "0x") || !fixedLowerHex(field[2:], 32) || common.HexToHash(field) == (common.Hash{}) {
			return chainview.GenerationReceiptAuthority{}, denied
		}
	}
	for _, field := range []string{record.Account, record.BoardSigner, record.Operator} {
		if !strings.HasPrefix(field, "0x") || !fixedLowerHex(field[2:], 20) || common.HexToAddress(field) == (common.Address{}) {
			return chainview.GenerationReceiptAuthority{}, denied
		}
	}
	if record.BoardSigner == record.Operator || record.BoardSigner == record.Account {
		return chainview.GenerationReceiptAuthority{}, denied
	}
	binding := chainview.GenerationBinding{
		Branch: record.Branch, Board: record.Board, Generation: generation, Chain: record.ChainID,
		Registration: common.HexToHash(record.Registration), Measurement: common.HexToHash(record.Measurement),
		Deployment: common.HexToHash(record.Deployment), Settlement: profile.Contract, Account: common.HexToAddress(record.Account),
	}
	if _, err := chainview.GenerationBindingDigest(binding); err != nil {
		return chainview.GenerationReceiptAuthority{}, denied
	}
	return chainview.GenerationReceiptAuthority{Binding: binding, BoardSigner: common.HexToAddress(record.BoardSigner),
		Operator: common.HexToAddress(record.Operator), Challenge: challenge, RegisteredAt: time.Unix(record.RegisteredAt, 0),
		ChallengeUntil: time.Unix(record.ChallengeUntil, 0), AuthorityUntil: time.Unix(record.ValidUntil, 0)}, nil
}

func fixedLowerHex(s string, size int) bool {
	if len(s) != size*2 || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
