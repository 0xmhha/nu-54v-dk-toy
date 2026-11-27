package chainview

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// AnswerAbsence proves requested absences with ProveAbsence until none is due or the budget is spent. A request
// is due when the active generation has a finalized block at or after its signed expiry. The claim keeps other
// readers away while one works, and a failure puts the request back with a growing delay.
func (r *Reader) AnswerAbsence(ctx context.Context, budget time.Duration) error {
	if _, err := r.DB.db.Exec(ctx, `DELETE FROM chainview.absence_request
		WHERE stream_id=$1 AND requested_at<clock_timestamp()-interval '7 days'`, r.Profile.StreamID); err != nil {
		return err
	}
	for start := time.Now(); time.Since(start) < budget; {
		var merchant, order string
		var expiry int64
		var answered bool
		err := r.DB.db.QueryRow(ctx, `UPDATE chainview.absence_request q
			SET attempts=attempts+1,next_attempt_at=clock_timestamp()+interval '30 seconds'
			WHERE (stream_id,merchant_key,order_key,quote_expires)=(
				SELECT d.stream_id,d.merchant_key,d.order_key,d.quote_expires FROM chainview.absence_request d
				JOIN chainview.read_target t ON t.stream_id=d.stream_id
				JOIN chainview.generation g ON g.id=t.generation_id AND g.state='ready'
				JOIN chainview.block anchor ON anchor.generation_id=g.id AND anchor.block_hash=g.finalized_hash
				WHERE d.stream_id=$1 AND g.id=$2 AND d.next_attempt_at<=clock_timestamp()
				  AND anchor.block_time>=to_timestamp(d.quote_expires)
				ORDER BY d.next_attempt_at LIMIT 1 FOR UPDATE OF d SKIP LOCKED)
			RETURNING merchant_key,order_key,quote_expires,
				chainview.absence_answered(stream_id,merchant_key,order_key,quote_expires)`,
			r.Profile.StreamID, r.Profile.GenerationID).Scan(&merchant, &order, &expiry, &answered)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if !answered {
			if _, err := r.ProveAbsence(ctx, merchant, order, time.Unix(expiry, 0)); err != nil {
				// The caller's deadline may be the reason; the claim expires by itself if this fails too.
				late, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
				defer cancel()
				_, _ = r.DB.db.Exec(late, `UPDATE chainview.absence_request
					SET next_attempt_at=clock_timestamp()+make_interval(secs=>least(300,power(2,least(attempts,9))))
					WHERE (stream_id,merchant_key,order_key,quote_expires)=($1,$2,$3,$4)`, r.Profile.StreamID, merchant, order, expiry)
				return err
			}
		}
		if _, err := r.DB.db.Exec(ctx, `DELETE FROM chainview.absence_request
			WHERE (stream_id,merchant_key,order_key,quote_expires)=($1,$2,$3,$4)`, r.Profile.StreamID, merchant, order, expiry); err != nil {
			return err
		}
	}
	return nil
}
