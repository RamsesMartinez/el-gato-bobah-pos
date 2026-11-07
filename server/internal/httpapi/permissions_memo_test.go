package httpapi

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Los permisos se leen de la base UNA vez por petición (el gate y el handler preguntan lo mismo), y
// fuera de una petición con caché se leen cada vez.
func TestMemoizedPermissions(t *testing.T) {
	calls := 0
	resolve := MemoizedPermissions(func(context.Context, AuthUser) ([]domain.Permission, error) {
		calls++
		return []domain.Permission{domain.PermSalesView}, nil
	})
	u := AuthUser{ID: 1}

	ctx := context.WithValue(context.Background(), permCacheKey{}, &permCache{})
	for range 3 {
		if p, err := resolve(ctx, u); err != nil || len(p) != 1 {
			t.Fatalf("= %v, %v", p, err)
		}
	}
	if calls != 1 {
		t.Fatalf("con caché leyó %d veces, quiere 1", calls)
	}

	calls = 0
	for range 2 {
		_, _ = resolve(context.Background(), u)
	}
	if calls != 2 {
		t.Fatalf("sin caché leyó %d veces, quiere 2", calls)
	}
}
