package chainview

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/rentalv1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
)

const (
	generationHeaderBytes        = 277
	generationEntryBytes         = 126
	MaxGenerationProjectionBytes = generationHeaderBytes + 8*generationEntryBytes
)

type GenerationBinding struct {
	Generation                            uint64
	Branch, Board                         string
	Registration, Measurement, Deployment common.Hash
	Chain                                 uint64
	Settlement, Account                   common.Address
}

type generationEntry struct {
	generationUse
	burned bool
}

// VerifiedGenerationFreeze is the authority's fresh binding to the RJP1 bytes.
// A service must obtain it from its board receipt verifier, never request JSON.
type VerifiedGenerationFreeze struct {
	Binding               GenerationBinding
	Root, ReceiptDigest   common.Hash
	FirstNonce, NextNonce common.Hash
	EntryCount            uint64
	FrozenAt, ValidUntil  time.Time
}

func ParseGenerationProjection(data []byte) (GenerationBinding, common.Hash, error) {
	j, err := readGenerationJournal(data)
	return j.binding, j.root, err
}

type generationJournal struct {
	binding     GenerationBinding
	root        common.Hash
	first, next common.Hash
	entries     []generationEntry
}

func readGenerationJournal(data []byte) (generationJournal, error) {
	errInvalid := errors.New("invalid rental generation projection")
	var journal generationJournal
	if len(data) < generationHeaderBytes || len(data) > MaxGenerationProjectionBytes ||
		string(data[:4]) != "RJP1" || data[243] != 0 || len(data) != generationHeaderBytes+int(data[276])*generationEntryBytes {
		return journal, errInvalid
	}
	binding := data[4:212]
	chain := new(big.Int).SetBytes(binding[136:168])
	journal.binding = GenerationBinding{
		Generation: binary.BigEndian.Uint64(binding[:8]),
		Branch:     projectUUID(binding[8:24]), Board: projectUUID(binding[24:40]),
		Registration: common.Hash(binding[40:72]), Measurement: common.Hash(binding[72:104]),
		Deployment: common.Hash(binding[104:136]), Chain: chain.Uint64(),
		Settlement: common.Address(binding[168:188]), Account: common.Address(binding[188:208]),
	}
	b := journal.binding
	if b.Generation == 0 || !nonzeroUUID(b.Branch) || !nonzeroUUID(b.Board) ||
		b.Registration == (common.Hash{}) || b.Measurement == (common.Hash{}) || b.Deployment == (common.Hash{}) ||
		!chain.IsUint64() || b.Chain == 0 || b.Settlement == (common.Address{}) || b.Account == (common.Address{}) {
		return generationJournal{}, errInvalid
	}
	journal.first, journal.next = common.Hash(data[212:244]), common.Hash(data[244:276])
	expected := new(big.Int).SetBytes(journal.first[:])
	next := new(big.Int).SetBytes(journal.next[:])
	for entry := data[generationHeaderBytes:]; len(entry) != 0; entry = entry[generationEntryBytes:] {
		row := generationEntry{
			burned: entry[1] == 2,
			generationUse: generationUse{
				device: b.Account, nonce: common.Hash(entry[2:34]), expiry: binary.BigEndian.Uint64(entry[34:42]),
				merchant: common.Address(entry[42:62]), order: common.Hash(entry[62:94]), digest: common.Hash(entry[94:126]),
			},
		}
		switch entry[0] {
		case 1:
			row.kind = "payment"
			if row.merchant == (common.Address{}) || row.order == (common.Hash{}) {
				return generationJournal{}, errInvalid
			}
		case 2:
			row.kind = "limit"
			if row.merchant != (common.Address{}) || row.order != (common.Hash{}) {
				return generationJournal{}, errInvalid
			}
		default:
			return generationJournal{}, errInvalid
		}
		if (entry[1] != 1 && !row.burned) || row.expiry == 0 || row.digest == (common.Hash{}) ||
			expected.BitLen() > 256 || row.nonce != common.BytesToHash(expected.Bytes()) {
			return generationJournal{}, errInvalid
		}
		journal.entries = append(journal.entries, row)
		expected.Add(expected, big.NewInt(1))
	}
	if expected.Cmp(next) != 0 {
		return generationJournal{}, errInvalid
	}
	journal.root = crypto.Keccak256Hash(data)
	return journal, nil
}

func projectUUID(raw []byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[:4], raw[4:6], raw[6:8], raw[8:10], raw[10:])
}

func nonzeroUUID(value string) bool {
	return uuidPattern.MatchString(value) && value != "00000000-0000-0000-0000-000000000000"
}

type GenerationAssessment = rentalv1.Assessment

type generationOccurrence struct {
	use   generationUse
	block Header
}

type generationStateSource interface {
	Source
	generationDeploymentAt(context.Context, Profile, Header) bool
	generationNonceUsed(context.Context, common.Address, common.Hash, Header) (bool, error)
	generationUses(context.Context, common.Address, Header, map[common.Hash]bool) (map[common.Hash]generationOccurrence, error)
}

var errGenerationUnresolved = errors.New("generation evidence incomplete")

type journalAssessment struct {
	payments, limits int
	states           string
	origins          []Header
}

func assessGeneration(ctx context.Context, source generationStateSource, j generationJournal, anchor Header, now time.Time) (journalAssessment, error) {
	var result journalAssessment
	used := make(map[common.Hash]bool)
	for _, row := range j.entries {
		bit, err := source.generationNonceUsed(ctx, j.binding.Account, row.nonce, anchor)
		if err != nil || (bit && row.burned) {
			return journalAssessment{}, errGenerationUnresolved
		}
		if bit {
			used[row.nonce] = true
		}
	}
	uses := make(map[common.Hash]generationOccurrence)
	if len(used) != 0 {
		var err error
		uses, err = source.generationUses(ctx, j.binding.Account, anchor, used)
		if err != nil {
			return journalAssessment{}, errGenerationUnresolved
		}
	}
	states := make([]byte, len(j.entries))
	for i, row := range j.entries {
		if row.burned {
			states[i] = 'B'
			continue
		}
		usedAtAnchor := used[row.nonce]
		if usedAtAnchor {
			matched, known := uses[row.nonce]
			if !known || matched.use != row.generationUse {
				return journalAssessment{}, errGenerationUnresolved
			}
			result.origins = append(result.origins, matched.block)
		}
		if row.kind == "payment" {
			paid, err := source.Paid(ctx, lower(row.merchant), row.order.Hex(), anchor)
			if err != nil || paid != usedAtAnchor {
				return journalAssessment{}, errGenerationUnresolved
			}
		}
		switch {
		case usedAtAnchor:
			states[i] = 'U'
		case uint64(anchor.Time.Unix()) > row.expiry:
			states[i] = 'E'
		case uint64(now.Unix()) <= row.expiry:
			states[i] = 'P'
			if row.kind == "payment" {
				result.payments++
			} else {
				result.limits++
			}
		default:
			return journalAssessment{}, errGenerationUnresolved
		}
	}
	result.states = string(states)
	return result, nil
}

func generationDigest(profile Profile, freeze VerifiedGenerationFreeze, anchor Header, previous *rentalv1.Anchor, states string, validUntil time.Time) string {
	payload := fmt.Sprintf("nu54-rental-reconciliation-v1\n%s\n%s\n%s\n%s\n%s\n%d\n%s\n%s\n%d",
		profile.StreamID, profile.GenerationID, profile.Manifest, freeze.Root.Hex(), freeze.ReceiptDigest.Hex(), anchor.Height,
		anchor.Hash.Hex(), states, validUntil.Unix())
	if previous != nil {
		payload += fmt.Sprintf("\n%d\n%s", previous.Height, previous.Hash)
	}
	digest := sha256.Sum256([]byte(payload))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func generationAnchor(ctx context.Context, db absenceQuery, profile Profile) (Header, error) {
	var anchor Header
	var hash, parent string
	err := db.QueryRow(ctx, `SELECT g.finalized_height,b.block_hash,b.parent_hash,b.block_time
		FROM chainview.generation g JOIN chainview.read_target t ON t.generation_id=g.id
		JOIN chainview.stream s ON s.id=t.stream_id
		JOIN chainview.deployment_profile p ON p.stream_id=s.id
		JOIN chainview.block b ON b.generation_id=g.id AND b.height=g.finalized_height AND b.block_hash=g.finalized_hash
		WHERE g.id=$1 AND s.id=$2 AND g.state='ready' AND b.canonical AND p.version=1
		AND s.chain_ref=$3 AND s.contract_ref=$4 AND s.abi_hash=$5 AND s.manifest_ref=$6
		AND p.asset_ref=$7 AND p.code_hash=$8 AND p.start_height=$9 AND p.start_hash=$10
		AND g.base_height=p.start_height AND chainview.complete_prefix(g.id,g.finalized_height)
		AND NOT EXISTS (SELECT FROM chainview.profile_hold pending
			WHERE pending.stream_id=t.stream_id AND pending.generation_id=g.id)
		AND NOT EXISTS (SELECT FROM chainview.incident i JOIN chainview.generation affected ON affected.id=i.generation_id
			WHERE affected.stream_id=t.stream_id AND i.resolved_at IS NULL)`,
		profile.GenerationID, profile.StreamID, strconv.FormatUint(profile.ChainID, 10), lower(profile.Contract),
		profile.ABIHash, profile.Manifest, lower(profile.Asset), profile.CodeHash.Hex(), profile.StartHeight,
		profile.StartHash.Hex()).Scan(&anchor.Height, &hash, &parent, &anchor.Time)
	if err != nil || !hexKey(hash, 64) || !hexKey(parent, 64) || anchor.Time.Unix() <= 0 {
		return Header{}, errGenerationUnresolved
	}
	anchor.Hash, anchor.Parent = common.HexToHash(hash), common.HexToHash(parent)
	return anchor, nil
}

// ReconcileGeneration does not write proof rows or alter an active read target.
func (r *Reader) ReconcileGeneration(ctx context.Context, data []byte, freeze VerifiedGenerationFreeze, previous *rentalv1.Anchor, clock func() time.Time) GenerationAssessment {
	journal, err := readGenerationJournal(data)
	profile := r.Profile
	now := clock()
	if err != nil || journal.binding != freeze.Binding || journal.root != freeze.Root ||
		journal.first != freeze.FirstNonce || journal.next != freeze.NextNonce || uint64(len(journal.entries)) != freeze.EntryCount ||
		freeze.ReceiptDigest == (common.Hash{}) || freeze.FrozenAt.Unix() <= 0 || freeze.FrozenAt.After(now) || !freeze.ValidUntil.After(now) {
		return GenerationAssessment{Status: "unknown"}
	}
	source, ok := r.Source.(generationStateSource)
	if profile.Version != 1 || !ok || r.DB == nil ||
		journal.binding.Deployment.Hex()[2:] != profile.Manifest || journal.binding.Chain != profile.ChainID ||
		journal.binding.Settlement != profile.Contract {
		return GenerationAssessment{Status: "incomplete"}
	}
	anchor, err := generationAnchor(ctx, r.DB.db, profile)
	if err != nil {
		return GenerationAssessment{Status: "incomplete"}
	}
	_, final, err := source.Heads(ctx)
	if err != nil || final.Height < anchor.Height || anchor.Time.Before(freeze.FrozenAt) ||
		verifyAnchor(ctx, source, anchor) != nil || !source.generationDeploymentAt(ctx, profile, anchor) {
		return GenerationAssessment{Status: "incomplete"}
	}
	assessment, err := assessGeneration(ctx, source, journal, anchor, clock())
	if err != nil {
		return GenerationAssessment{Status: "incomplete"}
	}
	err = pgx.BeginFunc(ctx, r.DB.db, func(tx pgx.Tx) error {
		// Fence only the late checkpoint validation, after receipt and state RPC.
		var locked bool
		if err := tx.QueryRow(ctx, `SELECT chainview.lock_absence_anchor($1,$2,$3,$4)`,
			profile.StreamID, profile.GenerationID, anchor.Height, anchor.Hash.Hex()).Scan(&locked); err != nil || !locked {
			return errGenerationUnresolved
		}
		reloaded, err := generationAnchor(ctx, tx, profile)
		if err != nil || reloaded.Hash != anchor.Hash || reloaded.Height != anchor.Height || !reloaded.Time.Equal(anchor.Time) {
			return errGenerationUnresolved
		}
		if previous != nil {
			if !previous.Valid() || previous.Height < profile.StartHeight || previous.Height > anchor.Height {
				return errGenerationUnresolved
			}
			var exists bool
			err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM chainview.block
				WHERE generation_id=$1 AND height=$2 AND block_hash=$3 AND canonical)`,
				profile.GenerationID, previous.Height, previous.Hash).Scan(&exists)
			canonical, rpcErr := source.Header(ctx, previous.Height)
			if err != nil || !exists || rpcErr != nil || canonical.Hash.Hex() != previous.Hash {
				return errGenerationUnresolved
			}
		}
		for _, origin := range assessment.origins {
			var exists bool
			err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT FROM chainview.block
				WHERE generation_id=$1 AND block_hash=$2 AND height=$3 AND canonical AND height<=$4)`,
				profile.GenerationID, origin.Hash.Hex(), origin.Height, anchor.Height).Scan(&exists)
			if err != nil || !exists {
				return errGenerationUnresolved
			}
		}
		_, final, err := source.Heads(ctx)
		if err != nil || final.Height < anchor.Height || verifyAnchor(ctx, source, anchor) != nil {
			return errGenerationUnresolved
		}
		return nil
	})
	now = clock()
	if err != nil || anchor.Time.After(now) || !freeze.ValidUntil.After(now) {
		return GenerationAssessment{Status: "incomplete"}
	}
	validUntil := minTime(now.Add(absenceTTL), freeze.ValidUntil)
	var pendingPaymentEntries uint8
	for i, state := range assessment.states {
		if state == 'P' && journal.entries[i].kind == "payment" {
			pendingPaymentEntries |= 1 << i
		}
		if state == 'P' && journal.entries[i].expiry < math.MaxInt64 {
			// At the exact expiry second the v1 contract still accepts the approval.
			validUntil = minTime(validUntil, time.Unix(int64(journal.entries[i].expiry)+1, 0))
		}
	}
	if validUntil.Unix() <= now.Unix() {
		return GenerationAssessment{Status: "incomplete"}
	}
	status := "verified"
	pending := assessment.payments + assessment.limits
	if pending > 0 {
		status = "pending"
	}
	return GenerationAssessment{Status: status, PendingCount: &pending, PendingPayments: &assessment.payments,
		PendingLimits: &assessment.limits, PendingPaymentEntries: &pendingPaymentEntries, AnchorHeight: &anchor.Height, AnchorHash: anchor.Hash.Hex(),
		AnchorTime: anchor.Time.Unix(), Previous: previous, Binding: rentalv1.Binding{
			Stream: profile.StreamID, StreamGeneration: profile.GenerationID, Deployment: "0x" + profile.Manifest,
			FreezeReceipt: freeze.ReceiptDigest.Hex(), JournalRoot: freeze.Root.Hex(),
		},
		Digest: generationDigest(profile, freeze, anchor, previous, assessment.states, validUntil), ValidUntil: validUntil.Unix()}
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
