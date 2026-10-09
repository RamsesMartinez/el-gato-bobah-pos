//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// DOS CAPTURAS A LA VEZ NO ARMAN UN PAQUETE DENTRO DE OTRO, NI UN INSUMO CIRCULAR.
//
// Las reglas se revisaban antes de abrir la transacción: P1 := [P2] y P2 := [P3] al mismo tiempo
// pasaban las dos —cada una veía a la otra todavía sin componentes— y quedaba un paquete dentro de
// otro, que la venta no descuenta. Lo encontró la auditoría de seguridad de la spec 028.
func TestConcurrentCapturesCannotNestPackagesOrCloseCycles(t *testing.T) {
	t.Parallel()
	owner := newTestStore(t)
	ctx := context.Background()
	admin := makeUser(t, owner, "admin_carrera", "admin")
	g := unitID(t, owner, "g")
	st := appRoleStore(t)

	tenant := func() (context.Context, func()) {
		c, release, err := st.AcquireTenant(ctx, defaultCompanyID)
		if err != nil {
			t.Fatal(err)
		}
		return c, release
	}
	both := func(a, b func(context.Context) error) (errA, errB error) {
		ca, ra := tenant()
		cb, rb := tenant()
		defer ra()
		defer rb()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); errA = a(ca) }()
		go func() { defer wg.Done(); errB = b(cb) }()
		wg.Wait()
		return errA, errB
	}
	svc := app.NewAdminService(st)

	for i := 0; i < 15; i++ {
		p1 := makeProduct(t, owner, fmt.Sprintf("Carrera P1 %d", i), decimal.RequireFromString("10"), false)
		p2 := makeProduct(t, owner, fmt.Sprintf("Carrera P2 %d", i), decimal.RequireFromString("10"), false)
		p3 := makeProduct(t, owner, fmt.Sprintf("Carrera P3 %d", i), decimal.RequireFromString("10"), false)
		errA, errB := both(
			func(c context.Context) error {
				return svc.SaveComposition(c, app.CompositionOfProduct, p1,
					app.CompositionRequest{Components: []app.CompositionComponent{{ProductID: p2, Quantity: 1}}}, admin)
			},
			func(c context.Context) error {
				return svc.SaveComposition(c, app.CompositionOfProduct, p2,
					app.CompositionRequest{Components: []app.CompositionComponent{{ProductID: p3, Quantity: 1}}}, admin)
			})
		if errA == nil && errB == nil {
			t.Fatalf("intento %d: las dos capturas pasaron y quedó un paquete dentro de otro", i)
		}
		for _, err := range []error{errA, errB} {
			if err != nil && !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("la que pierde se rechaza como validación, no con %v", err)
			}
		}

		a := makeIngredient(t, owner, fmt.Sprintf("Carrera A %d", i))
		b := makeIngredient(t, owner, fmt.Sprintf("Carrera B %d", i))
		one := decimal.RequireFromString("1")
		prep := func(target, comp int64) func(context.Context) error {
			return func(c context.Context) error {
				return svc.SaveComposition(c, app.CompositionOfIngredient, target, app.CompositionRequest{
					Items: []app.CompositionInputItem{{IngredientID: comp, Quantity: one, UnitID: g}}, Yield: &one,
				}, admin)
			}
		}
		if errA, errB := both(prep(a, b), prep(b, a)); errA == nil && errB == nil {
			t.Fatalf("intento %d: A lleva B y B lleva A, las dos pasaron", i)
		}
	}
}
