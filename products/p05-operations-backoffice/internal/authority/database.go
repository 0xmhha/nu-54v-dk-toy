package authority

import (
	"context"
	"time"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool retains pgxpool behavior for workers. On authenticated HTTP requests,
// statements that can write a result are executed in a transaction whose
// commit is gated by the still-current authority. Query is for list reads.
type Pool struct {
	*pgxpool.Pool
}

func (db Pool) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if _, secured := ctx.Value(contextKey{}).(boundary); !secured {
		return db.Pool.Exec(ctx, sql, args...)
	}
	var tag pgconn.CommandTag
	err := db.run(ctx, func(tx pgx.Tx) error {
		var err error
		tag, err = tx.Exec(ctx, sql, args...)
		return err
	}, nil)
	return tag, err
}

func (db Pool) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if _, secured := ctx.Value(contextKey{}).(boundary); !secured {
		return db.Pool.QueryRow(ctx, sql, args...)
	}
	return guardedRow{db: db, ctx: ctx, sql: sql, args: args}
}

// QueryRowBeforeCommit adds a local admission check after the statement and
// authenticated-scope checks, while its database effects can still roll back.
func (db Pool) QueryRowBeforeCommit(ctx context.Context, check func() error, sql string, args ...any) pgx.Row {
	return guardedRow{db: db, ctx: ctx, sql: sql, args: args, beforeCommit: check}
}

type scopeKey struct{}

type scope struct {
	principal    service.Principal
	branch       string
	capabilities []string
}

// Scope rechecks the selected current branch membership or one of the
// expressly accepted capabilities within each database transaction.
func Scope(ctx context.Context, principal service.Principal, branch string, capabilities ...string) context.Context {
	if capabilities == nil {
		capabilities = []string{}
	}
	return context.WithValue(ctx, scopeKey{}, scope{principal, branch, capabilities})
}

func checkScope(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	value, ok := ctx.Value(scopeKey{}).(scope)
	if !ok {
		return nil
	}
	var current bool
	err := db.QueryRow(ctx, "SELECT ledger.request_authority_live($1,$2,$3,$4,$5)",
		value.principal.Tenant, value.branch, value.principal.Subject, value.principal.Role, value.capabilities).Scan(&current)
	if err != nil {
		return err
	}
	if !current {
		return service.Forbidden
	}
	return nil
}

type guardedRow struct {
	db           Pool
	ctx          context.Context
	sql          string
	args         []any
	beforeCommit func() error
}

func (q guardedRow) Scan(dest ...any) error {
	return q.db.run(q.ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(q.ctx, q.sql, q.args...).Scan(dest...)
	}, q.beforeCommit)
}

func (db Pool) run(ctx context.Context, action func(pgx.Tx) error, beforeCommit func() error) error {
	if err := Check(ctx); err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		// Cancellation must never leave an open transaction holding locks.
		rollback, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		_ = tx.Rollback(rollback)
	}()
	if err = Check(ctx); err != nil {
		return err
	}
	if err = action(tx); err != nil {
		return err
	}
	if err = checkScope(ctx, tx); err != nil {
		return err
	}
	if beforeCommit != nil {
		if err = beforeCommit(); err != nil {
			return err
		}
	}
	if err = Check(ctx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (db Pool) CheckRead(ctx context.Context) error {
	if err := Check(ctx); err != nil {
		return err
	}
	return checkScope(ctx, db.Pool)
}
