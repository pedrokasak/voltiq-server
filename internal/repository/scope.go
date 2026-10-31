package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/voltiq/server/internal/reqctx"
)

// tenantMarker marks where a tenant filter goes in a query's WHERE clause.
const tenantMarker = "/*tenant*/"

// scoped replaces tenantMarker with "AND tenant_id = $n" when the request
// carries a tenant, so lookups by primary key can never cross tenants even
// when the database role bypasses RLS. System contexts (no tenant) keep the
// query unfiltered.
func scoped(ctx context.Context, query string, args ...any) (string, []any) {
	return scopedColumn(ctx, "tenant_id", query, args...)
}

// scopedColumn is scoped with an explicit (possibly qualified) column name.
func scopedColumn(ctx context.Context, column, query string, args ...any) (string, []any) {
	tenantID := reqctx.TenantID(ctx)
	if tenantID == "" {
		return strings.ReplaceAll(query, tenantMarker, ""), args
	}
	args = append(args, string(tenantID))
	clause := fmt.Sprintf("AND %s = $%d", column, len(args))
	return strings.ReplaceAll(query, tenantMarker, clause), args
}

// ErrNotFound is returned when a write targets a row that does not exist,
// is soft-deleted, or belongs to another tenant.
var ErrNotFound = errors.New("record not found")

// execer is satisfied by *pgxpool.Pool and pgx.Tx.
type execer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}
