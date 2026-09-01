//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/shopspring/decimal"
)

// LA FECHA DE NEGOCIO NO PUEDE CAER A UTC.
//
// `businessDate` caía a UTC cuando no podía leer la zona del negocio, y el comentario lo justificaba
// bien —abrir caja no se detiene por un ajuste mal escrito— pero eligió mal el valor: el producto
// tiene un default y caer a UTC corre la fecha SEIS HORAS sin avisar. Un turno abierto después de
// las 18:00 locales queda fechado al día siguiente, y todo su dinero cae en el arqueo equivocado.
//
// No es teórico: los pedidos 61 y 62 de la cuenta de pruebas, del 2026-08-29 a las 20:50 hora local,
// están fechados el 30 por exactamente esto.
//
// Se prueba a una hora que en UTC YA ES del día siguiente: a cualquier otra, los dos caminos dan la
// misma fecha y el test pasaría con el defecto puesto.
func TestSinZonaLaFechaDeNegocioUsaElDefaultDelProductoYNoUTC(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// 2026-08-29 20:50 en México = 2026-08-30 02:50 UTC. La fecha correcta es el 29.
	enLaNoche := time.Date(2026, 8, 30, 2, 50, 0, 0, time.UTC)
	svc := app.NewBackofficeService(st, func() time.Time { return enLaNoche })
	cajero := makeUser(t, st, "cajero_sin_zona", "cajero")

	// Sin fila de ajustes no se puede leer la zona: es el camino del fallback, y es el que dejó los
	// dos pedidos mal fechados en producción.
	if _, err := st.Pool.Exec(ctx, `delete from business_settings`); err != nil {
		t.Fatalf("quitar los ajustes: %v", err)
	}

	sesion, err := svc.OpenSession(ctx, registerID(t, st, "Caja principal"), decimal.RequireFromString("500"), cajero)
	if err != nil {
		t.Fatalf("abrir caja: %v", err)
	}

	var fecha time.Time
	if err := st.Pool.QueryRow(ctx,
		`select business_date from register_sessions where id = $1`, sesion.ID).Scan(&fecha); err != nil {
		t.Fatalf("leer la fecha del turno: %v", err)
	}

	quiere := domain.BusinessDate(enLaNoche, domain.LoadBusinessLocation(domain.DefaultTimezone))
	if !fecha.Equal(quiere) {
		t.Errorf("el turno quedó fechado %s y quiere %s: la fecha cayó a UTC y todo el dinero de ese turno entra al arqueo del día siguiente",
			fecha.Format("2006-01-02"), quiere.Format("2006-01-02"))
	}
}
