package chainview

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

var (
	ErrFinalizedConflict = errors.New("finalized checkpoint conflict")
	uuidPattern          = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	hashPattern          = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const (
	batchSize     = 32
	maxReorgDepth = 512
	maxPollEvents = 5000 // a crowded block ends the batch, so one poll stays within its deadline
)

func (p Profile) validate() error {
	if _, err := NewDecoder(p); err != nil {
		return err
	}
	if !uuidPattern.MatchString(p.StreamID) || !uuidPattern.MatchString(p.GenerationID) ||
		!hashPattern.MatchString(p.Manifest) || p.CodeHash == (common.Hash{}) ||
		p.StartHash == (common.Hash{}) || p.StartHeight < 1 {
		return ErrDeploymentIdentity
	}
	return nil
}

type Header struct {
	Height       int64
	Hash, Parent common.Hash
	Time         time.Time
}

type Block struct {
	Header
	Events []Event
}

type Source interface {
	Heads(context.Context) (latest, finalized Header, err error)
	Header(context.Context, int64) (Header, error)
	Block(context.Context, Header) (Block, error)
	Paid(context.Context, string, string, Header) (bool, error)
}

type Reader struct {
	Profile  Profile
	Source   Source
	DB       *Repository
	isolated bool
}

func (r *Reader) Poll(ctx context.Context) error {
	l, err := r.DB.claim(ctx, r.Profile.GenerationID)
	if err != nil {
		return err
	}
	defer func() {
		release, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		r.DB.release(release, l)
	}()

	latest, reportedFinal, err := r.Source.Heads(ctx)
	if err != nil {
		return err
	}
	if latest.Height < reportedFinal.Height || latest.Hash == (common.Hash{}) || reportedFinal.Hash == (common.Hash{}) {
		return errors.New("inconsistent chain heads")
	}
	if l.finalized != nil {
		if reportedFinal.Height < *l.finalized {
			return r.freeze(ctx, l, *l.finalized, *l.finalizedHash, reportedFinal.Hash.Hex())
		}
		canonical, err := r.Source.Header(ctx, *l.finalized)
		if err != nil {
			return err
		}
		if canonical.Hash.Hex() != *l.finalizedHash {
			return r.freeze(ctx, l, *l.finalized, *l.finalizedHash, canonical.Hash.Hex())
		}
	}
	if latest.Height < r.Profile.StartHeight {
		return nil
	}
	start, err := r.Source.Header(ctx, r.Profile.StartHeight)
	if err != nil {
		return err
	}
	if start.Hash != r.Profile.StartHash {
		return r.freeze(ctx, l, r.Profile.StartHeight, r.Profile.StartHash.Hex(), start.Hash.Hex())
	}
	anchor, err := r.Source.Header(ctx, reportedFinal.Height)
	if err != nil {
		return err
	}
	if anchor.Hash != reportedFinal.Hash {
		return errors.New("reported finality is not on the RPC canonical branch")
	}

	ancestor, parent, err := r.ancestor(ctx, l, latest.Height)
	if err != nil {
		return err
	}
	end := latest.Height
	if latest.Height-ancestor > batchSize {
		end = ancestor + batchSize
	}
	blocks := make([]Block, 0, end-ancestor)
	events := 0
	for height := ancestor + 1; height <= end; height++ {
		head, err := r.Source.Header(ctx, height)
		if err != nil {
			return err
		}
		if head.Height != height || head.Hash == (common.Hash{}) ||
			(height == r.Profile.StartHeight && head.Hash != r.Profile.StartHash) ||
			(height > r.Profile.StartHeight && head.Parent != parent) {
			return errors.New("block parent or deployment anchor changed")
		}
		block, err := r.Source.Block(ctx, head)
		if err != nil {
			return err
		}
		if block.Header != head {
			return errors.New("receipt block does not match its requested header")
		}
		blocks = append(blocks, block)
		parent = block.Hash
		if events += len(block.Events); events >= maxPollEvents {
			end = height
		}
	}
	last, err := r.Source.Header(ctx, end)
	if err != nil {
		return err
	}
	if last.Hash != parent {
		return errors.New("canonical block changed before commit")
	}
	if err := verifyAnchor(ctx, r.Source, reportedFinal); err != nil {
		return errors.New("finalized head changed before commit")
	}
	var final *Header
	finalHeight := min(end, reportedFinal.Height)
	if finalHeight >= r.Profile.StartHeight && (l.finalized == nil || finalHeight > *l.finalized) {
		checkpoint, err := r.Source.Header(ctx, finalHeight)
		if err != nil {
			return err
		}
		final = &checkpoint
	}
	if l.tip != nil && *l.tip == ancestor && len(blocks) == 0 && final == nil {
		return r.DB.observe(ctx, l, reportedFinal.Height)
	}
	return r.DB.advance(ctx, l, ancestor, blocks, final, r.Profile.Manifest, reportedFinal.Height)
}

func (r *Reader) ancestor(ctx context.Context, l lease, latest int64) (int64, common.Hash, error) {
	base := r.Profile.StartHeight
	if l.tip != nil {
		from := min(*l.tip, latest)
		for height := from; height >= base; height-- {
			if from-height >= maxReorgDepth {
				return 0, common.Hash{}, errors.New("fork exceeds bounded replay search")
			}
			remote, err := r.Source.Header(ctx, height)
			if err != nil {
				return 0, common.Hash{}, err
			}
			local, err := r.DB.canonical(ctx, l.generation, height)
			if err != nil {
				return 0, common.Hash{}, err
			}
			if remote.Hash == local {
				return height, local, nil
			}
			if l.finalized != nil && height <= *l.finalized {
				return 0, common.Hash{}, errors.New("RPC view changed while searching finalized boundary")
			}
		}
	}
	return base - 1, common.Hash{}, nil
}

func (r *Reader) freeze(ctx context.Context, l lease, height int64, oldHash, newHash string) error {
	if r.isolated {
		return errReplayDiverged
	}
	if oldHash == newHash {
		return errors.New("finality regression without conflicting block hash")
	}
	if err := r.DB.freeze(ctx, l, height, oldHash, newHash); err != nil {
		return err
	}
	return fmt.Errorf("%w: %s at height %d", ErrFinalizedConflict, r.Profile.GenerationID, height)
}
