package chainview

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var errReplayDiverged = errors.New("isolated replay anchor diverged")

type replaySource struct {
	Source
	anchor Header
}

func (s replaySource) Heads(context.Context) (Header, Header, error) {
	return s.anchor, s.anchor, nil
}

// PollReplay advances at most one disjoint replay for the configured active
// writer. The source still checks canonical headers and real receipt roots.
func (r *Reader) PollReplay(ctx context.Context) error {
	var job string
	var height int64
	var hash string
	err := r.DB.db.QueryRow(ctx, `SELECT id::text,anchor_height,anchor_hash FROM chainview.replay_job
		WHERE previous_id=$1 AND stream_id=$2 AND state='running' ORDER BY requested_at LIMIT 1`,
		r.Profile.GenerationID, r.Profile.StreamID).Scan(&job, &height, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	anchor, err := r.Source.Header(ctx, height)
	if err != nil {
		return err
	}
	diverged := anchor.Hash.Hex() != hash
	if !diverged {
		profile := r.Profile
		profile.GenerationID = job
		replay := Reader{Profile: profile, Source: replaySource{r.Source, anchor}, DB: r.DB, isolated: true}
		err = replay.Poll(ctx)
		diverged = errors.Is(err, errReplayDiverged)
		if err != nil && !diverged {
			return err
		}
	}
	_, err = r.DB.db.Exec(ctx, "SELECT chainview.finish_replay($1,$2)", job, diverged)
	return err
}
