//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// Regresión: tras 0023 (business_settings pasó a PK company_id, sin columna id), las queries de
// ajustes seguían con `where id = true` → GET/PUT reventaban con 42703 (500) en prod. Este test
// corre sobre el esquema ya migrado (0023 siembra una fila para la empresa 1) y falla si la query
// vuelve a referenciar una columna inexistente.
func TestBusinessSettingsGetAndUpdate(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st)
	admin := makeUser(t, st, "admin_settings", "admin")

	// GET no debe reventar (antes: column "id" does not exist).
	if _, err := settings.Get(ctx); err != nil {
		t.Fatalf("Get business settings: %v", err)
	}

	// PUT actualiza la fila del tenant y hace roundtrip.
	if _, err := settings.SetDeliveryFee(ctx, decimal.RequireFromString("35"), admin); err != nil {
		t.Fatalf("SetDeliveryFee: %v", err)
	}
	got, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get tras update: %v", err)
	}
	if !got.DeliveryFee.Equal(decimal.RequireFromString("35")) {
		t.Fatalf("deliveryFee = %s, want 35", got.DeliveryFee)
	}
}
