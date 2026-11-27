package service

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"time"
)

var (
	ErrRecoveryFenced = errors.New("scoped service generation is fenced")
	recoveryScope     = regexp.MustCompile(`^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)
	recoveryManifest  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// RecoveryGate leases the currently active, trusted deployment generation for
// the lifetime of one ordinary operation. Operators drain these leases before
// making a scope unavailable or publishing a new generation.
type RecoveryGate struct {
	db       *sql.DB
	scope    string
	manifest []byte
}

func OpenRecoveryGate(driver, dsn, scope, manifest string) (*RecoveryGate, error) {
	if dsn == "" && scope == "" && manifest == "" {
		return nil, nil
	}
	if dsn == "" || !recoveryScope.MatchString(scope) || !recoveryManifest.MatchString(manifest) {
		return nil, errors.New("recovery requires a control database, canonical scope and deployment manifest")
	}
	digest, err := hex.DecodeString(manifest)
	if err != nil {
		return nil, errors.New("invalid recovery manifest")
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, errors.New("invalid recovery control database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var allowed bool
	err = db.QueryRowContext(ctx, `SELECT pg_has_role(current_user,'nu54_p10_recovery_route_only','USAGE')
		AND NOT pg_has_role(current_user,'nu54_p10_recovery_coordinator','MEMBER')
		AND NOT pg_has_role(current_user,'nu54_p10_recovery_status','MEMBER')
		AND NOT pg_has_role(current_user,'nu54_p10_recovery_target','MEMBER')
		AND NOT EXISTS (SELECT FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb))`).Scan(&allowed)
	if err != nil || !allowed {
		_ = db.Close()
		return nil, errors.New("recovery service lease requires a route-only login")
	}
	db.SetMaxOpenConns(32)
	db.SetMaxIdleConns(4)
	return &RecoveryGate{db: db, scope: scope, manifest: digest}, nil
}

func (g *RecoveryGate) Close() {
	if g != nil {
		_ = g.db.Close()
	}
}

func (g *RecoveryGate) Run(ctx context.Context, operation func(context.Context) error) error {
	if g == nil {
		return operation(ctx)
	}
	tx, err := g.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ErrRecoveryFenced
	}
	defer tx.Rollback()
	admit, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	var gotLock, allowed bool
	if err = tx.QueryRowContext(admit, `SELECT pg_try_advisory_xact_lock_shared(hashtextextended($1,440054))`, g.scope).Scan(&gotLock); err != nil || !gotLock {
		return ErrRecoveryFenced
	}
	if err = tx.QueryRowContext(admit, `SELECT active_job IS NULL AND current_manifest=$2
		FROM platform.recovery_route WHERE scope_ref=$1::uuid`, g.scope, g.manifest).Scan(&allowed); err != nil || !allowed {
		return ErrRecoveryFenced
	}
	return operation(ctx)
}

func (g *RecoveryGate) Handler(next http.Handler) http.Handler {
	if g == nil {
		return next
	}
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := g.Run(request.Context(), func(ctx context.Context) error {
			next.ServeHTTP(response, request.WithContext(ctx))
			return nil
		}); err != nil {
			response.Header().Set("Cache-Control", "no-store")
			response.Header().Set("Retry-After", "5")
			http.Error(response, "service recovery in progress", http.StatusServiceUnavailable)
		}
	})
}
