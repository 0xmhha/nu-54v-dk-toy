package records

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RestorePublicationConfig struct {
	ControlDSN      string
	ErasureGuardDSN string
	Scope           string
	Manifest        string
}

// PublicationGuard combines the scoped P10 manifest lease with the live
// erasure journal's shared writer barrier. Restored reads and writes never
// continue against a checkpoint that has missed a later fence.
type PublicationGuard struct {
	local    *pgxpool.Pool
	control  *pgxpool.Pool
	journal  *pgxpool.Pool
	scope    string
	manifest []byte
}

func (s *Service) RestorePublication(ctx context.Context, config RestorePublicationConfig) (*PublicationGuard, error) {
	return openPublicationGuard(ctx, s.pool, config)
}

func (r *ReviewReader) RestorePublication(ctx context.Context, config RestorePublicationConfig) (*PublicationGuard, error) {
	return openPublicationGuard(ctx, r.pool, config)
}

func openPublicationGuard(ctx context.Context, local *pgxpool.Pool, config RestorePublicationConfig) (*PublicationGuard, error) {
	guard := &PublicationGuard{local: local, scope: config.Scope}
	if config == (RestorePublicationConfig{}) {
		_, _, restored, err := restoreCheckpoint(ctx, local)
		if err != nil || restored {
			return nil, errors.New("restored workload requires a live erasure barrier and recovery route")
		}
		return guard, nil
	}
	if config.ControlDSN == "" || config.ErasureGuardDSN == "" || config.ControlDSN == config.ErasureGuardDSN ||
		!uuid.MatchString(config.Scope) || !fingerprint.MatchString(config.Manifest) {
		return nil, errors.New("incomplete restore publication enrollment")
	}
	guard.manifest, _ = hex.DecodeString(config.Manifest)
	control, err := pgxpool.New(ctx, config.ControlDSN)
	if err != nil {
		return nil, errors.New("restore control unavailable")
	}
	guard.control = control
	var routeOnly bool
	err = control.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p10_recovery_route_only','USAGE')
		AND NOT pg_has_role(current_user,'nu54_p10_recovery_coordinator','MEMBER')
		AND NOT pg_has_role(current_user,'nu54_p10_recovery_status','MEMBER')
		AND NOT pg_has_role(current_user,'nu54_p10_recovery_target','MEMBER')
		AND NOT EXISTS (SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&routeOnly)
	if err != nil || !routeOnly {
		guard.Close()
		return nil, errors.New("restore control requires a route-only login")
	}
	journal, err := pgxpool.New(ctx, config.ErasureGuardDSN)
	if err != nil {
		guard.Close()
		return nil, errors.New("live erasure guard unavailable")
	}
	guard.journal = journal
	var allowed bool
	err = journal.QueryRow(ctx, `SELECT pg_has_role(current_user,'nu54_p09_erasure_guard','USAGE')
		AND NOT pg_has_role(current_user,'nu54_p09_erasure_append','MEMBER')
		AND NOT pg_has_role(current_user,'nu54_p09_erasure_export','MEMBER')
		AND NOT has_schema_privilege('travel_private','USAGE')
		AND NOT has_table_privilege('travel_erasure.fence','SELECT,INSERT,UPDATE,DELETE')
		AND NOT EXISTS (SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&allowed)
	if err != nil || !allowed {
		guard.Close()
		return nil, errors.New("live erasure guard capability unavailable")
	}
	return guard, nil
}

func (g *PublicationGuard) Close() {
	if g.control != nil {
		g.control.Close()
	}
	if g.journal != nil {
		g.journal.Close()
	}
}

type checkpointReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func restoreCheckpoint(ctx context.Context, local checkpointReader) (string, int64, bool, error) {
	var namespace string
	var high int64
	err := local.QueryRow(ctx, `SELECT namespace::text, applied_through FROM travel_private.restore_cursor()`).Scan(&namespace, &high)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, nil
	}
	return namespace, high, err == nil, err
}

func (g *PublicationGuard) Handler(normal http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.admit(r.Context(), func() { normal.ServeHTTP(w, r) }) {
			w.Header().Set("Cache-Control", "no-store")
			reply(w, http.StatusServiceUnavailable, map[string]string{"error": "restore_fenced"})
		}
	})
}

func (g *PublicationGuard) admit(ctx context.Context, use func()) bool {
	check, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	if g.control == nil {
		return g.admitUnrestored(check, use)
	}
	control, err := g.control.BeginTx(check, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return false
	}
	defer rollbackGuard(control)
	var locked, active bool
	if err = control.QueryRow(check, `SELECT pg_try_advisory_xact_lock_shared(hashtextextended($1,440054))`, g.scope).Scan(&locked); err != nil || !locked {
		return false
	}
	if err = control.QueryRow(check, `SELECT active_job IS NULL AND current_manifest=$2 FROM platform.recovery_route
		WHERE scope_ref=$1::uuid`, g.scope, g.manifest).Scan(&active); err != nil || !active {
		return false
	}
	namespace, high, restored, err := restoreCheckpoint(check, g.local)
	if err != nil || !restored {
		return false
	}
	journal, err := g.journal.BeginTx(check, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return false
	}
	defer rollbackGuard(journal)
	var current bool
	if err = journal.QueryRow(check, `SELECT travel_erasure.guard_restore($1,$2)`, namespace, high).Scan(&current); err != nil || !current {
		return false
	}
	use()
	return true
}

func (g *PublicationGuard) admitUnrestored(ctx context.Context, use func()) bool {
	local, err := g.local.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return false
	}
	defer rollbackGuard(local)
	var locked bool
	if err = local.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock_shared(900022)`).Scan(&locked); err != nil || !locked {
		return false
	}
	_, _, restored, err := restoreCheckpoint(ctx, local)
	if err != nil || restored {
		return false
	}
	use()
	return true
}

func rollbackGuard(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
