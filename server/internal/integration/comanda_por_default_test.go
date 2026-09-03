//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// LA EMPRESA NUEVA NACE IMPRIMIENDO LA COMANDA; LA QUE YA EXISTÍA NO CAMBIA.
//
// Cambiar el DEFAULT de la columna no toca ninguna fila: las empresas en operación conservan lo que
// tengan configurado. Un despliegue que le enciende una impresora a un negocio sin que nadie la
// pida es un defecto, no una mejora — y con dos empresas en la misma base, comprobarlo con una sola
// no probaría nada.
func TestLaEmpresaNuevaNaceImprimiendoLaComanda(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// La empresa por default ya tiene su fila, sembrada al levantar el store.
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	antes, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Una empresa nueva: se siembra su fila igual que lo hace provisionar.
	otra := makeCompany(t, st, "otra-comanda-default")
	if err := st.WithTenant(ctx, otra, func(q *db.Queries) error {
		return q.SeedBusinessSettings(ctx, "Otra Comanda")
	}); err != nil {
		t.Fatalf("sembrar los ajustes de la empresa nueva: %v", err)
	}

	var nace bool
	if err := st.Pool.QueryRow(ctx,
		`select print_kitchen_ticket from business_settings where company_id = $1`, otra).Scan(&nace); err != nil {
		t.Fatalf("leer el ajuste de la empresa nueva: %v", err)
	}
	if !nace {
		t.Error("la empresa nueva nació con la comanda apagada: el operador tiene que descubrir el ajuste para que cocina reciba papel")
	}

	// Y la que ya existía sigue igual.
	despues, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if despues.PrintKitchenTicket != antes.PrintKitchenTicket {
		t.Errorf("el ajuste de una empresa en operación cambió solo, de %v a %v",
			antes.PrintKitchenTicket, despues.PrintKitchenTicket)
	}
}
