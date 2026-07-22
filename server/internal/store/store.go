package store

import (
	"context"
	"fmt"
	"strconv"

	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

type Store struct {
	Pool *pgxpool.Pool
	Q    *db.Queries
}

func New(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 2
	// Escanea numeric ↔ shopspring/decimal.Decimal (dinero/costos/cantidades exactos, sin float64).
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		pgxdecimal.Register(conn.TypeMap())
		return nil
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{Pool: pool, Q: db.New(pool)}, nil
}

func (s *Store) Close() { s.Pool.Close() }

// tenantConnKey ata una conexión con contexto de tenant al ctx del request.
type tenantConnKey struct{}

// tenantConn es una conexión del pool, tomada por request, con app.company_id fijado a nivel
// de SESIÓN (is_local=false) → toda query sobre ella queda aislada por RLS a un solo tenant.
type tenantConn struct {
	conn *pgxpool.Conn
	q    *db.Queries
}

// AcquireTenant toma una conexión del pool y le fija el GUC app.company_id, devolviendo un ctx
// que la transporta y una función de release. El middleware de tenant la llama por request
// autenticado y hace defer del release. RESET al soltar: nunca hereda el tenant al siguiente que
// tome esa conexión del pool.
func (s *Store) AcquireTenant(ctx context.Context, companyID int64) (context.Context, func(), error) {
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return ctx, func() {}, err
	}
	if _, err := conn.Exec(ctx, "select set_config('app.company_id', $1, false)", strconv.FormatInt(companyID, 10)); err != nil {
		conn.Release()
		return ctx, func() {}, err
	}
	tc := &tenantConn{conn: conn, q: db.New(conn)}
	release := func() {
		// Fail-closed: si el RESET falla, NO devolver la conexión al pool con el GUC del tenant
		// "pegado" (otro request la tomaría y leería datos del tenant anterior). Se destruye.
		if _, err := conn.Exec(context.Background(), "reset app.company_id"); err != nil {
			_ = conn.Conn().Close(context.Background())
		}
		conn.Release()
	}
	return context.WithValue(ctx, tenantConnKey{}, tc), release, nil
}

func tenantFrom(ctx context.Context) *tenantConn {
	tc, _ := ctx.Value(tenantConnKey{}).(*tenantConn)
	return tc
}

// QC devuelve las Queries del tenant del request (fijadas por AcquireTenant) o, si no hay, las
// del pool sin scopear. Con el rol de app y sin conexión de tenant, RLS no devuelve nada
// (fail-closed) en vez de filtrar entre empresas: los servicios deben usar QC(ctx), no s.Q.
func (s *Store) QC(ctx context.Context) *db.Queries {
	if tc := tenantFrom(ctx); tc != nil {
		return tc.q
	}
	return s.Q
}

// WithTx runs fn inside a transaction with a tx-bound Queries. Si el request trae conexión de
// tenant, la tx se abre sobre ELLA (hereda app.company_id → RLS aplica); si no, sobre el pool.
func (s *Store) WithTx(ctx context.Context, fn func(q *db.Queries) error) error {
	if tc := tenantFrom(ctx); tc != nil {
		tx, err := tc.conn.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
		if err := fn(db.New(tx)); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	if err := fn(s.Q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// WithTenant runs fn in a transaction bound to a single company by fijando el GUC de sesión
// app.company_id (SET LOCAL vía set_config, is_local=true → solo dura la tx). RLS en Postgres
// usa ese GUC para aislar CADA query de fn al tenant; una query sin este contexto ve/escribe
// nada (fail-closed). TODO acceso a datos de un request autenticado debe pasar por aquí.
// set_config parametrizado ($1) — nunca interpolar el companyID a mano.
func (s *Store) WithTenant(ctx context.Context, companyID int64, fn func(q *db.Queries) error) error {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	if _, err := tx.Exec(ctx, "select set_config('app.company_id', $1, true)", strconv.FormatInt(companyID, 10)); err != nil {
		return err
	}
	if err := fn(s.Q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
