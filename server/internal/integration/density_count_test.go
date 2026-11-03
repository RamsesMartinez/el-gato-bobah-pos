//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LO QUE PESA VACÍO UN EMPAQUE Y EL CONTEO PESANDO CON EMPAQUE (spec 058, US4).

func tareField(s string) app.Field[decimal.Decimal] {
	if s == "" {
		return app.Field[decimal.Decimal]{Set: true}
	}
	return app.Field[decimal.Decimal]{Set: true, Value: decp(s)}
}

func TestThePresentationKeepsWhatItWeighsEmpty(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	salsa := ingredientIn(t, st, defaultCompanyID, "Salsa tara", "ml")
	ml := unitID(t, st, "ml")
	svc, ctx := asTenant(t, defaultCompanyID)

	p, err := svc.CreatePresentation(ctx, salsa, app.PresentationInput{Name: "Botella", Qty: dd("1000"), UnitID: ml, Tare: tareField("120")})
	if err != nil {
		t.Fatal(err)
	}
	if p.TareG == nil || !p.TareG.Equal(dd("120")) {
		t.Fatalf("la botella pesa vacía %v; quería 120", p.TareG)
	}
	var events int
	if err := st.Pool.QueryRow(context.Background(), `select count(*) from audit_events
		where action = 'presentation.tare_set' and entity_id = $1 and after->>'tareG' = '120'`, p.ID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("eventos de tara = %d; quería 1 con 120 g", events)
	}

	t.Run("editar sin mandarla no la toca; null la quita", func(t *testing.T) {
		p2, err := svc.UpdatePresentation(ctx, p.ID, app.PresentationInput{Name: "Botella 1 L", Qty: dd("1000"), UnitID: ml})
		if err != nil {
			t.Fatal(err)
		}
		if p2.TareG == nil || !p2.TareG.Equal(dd("120")) {
			t.Fatalf("una tableta vieja que edita el nombre borró la tara: %v", p2.TareG)
		}
		p3, err := svc.UpdatePresentation(ctx, p.ID, app.PresentationInput{Name: "Botella 1 L", Qty: dd("1000"), UnitID: ml, Tare: tareField("")})
		if err != nil {
			t.Fatal(err)
		}
		if p3.TareG != nil {
			t.Fatalf("null debió quitarla; quedó %s", p3.TareG)
		}
	})

	t.Run("cero, de más o en un artículo en piezas se rechaza", func(t *testing.T) {
		for _, v := range []string{"0", "100000.01", "-5"} {
			if _, err := svc.UpdatePresentation(ctx, p.ID, app.PresentationInput{Name: "Botella 1 L", Qty: dd("1000"), UnitID: ml, Tare: tareField(v)}); !errors.Is(err, domain.ErrValidation) {
				t.Errorf("tara %s = %v; quería 400", v, err)
			}
		}
		vasos := ingredientIn(t, st, defaultCompanyID, "Vasos tara", "pieza")
		if _, err := svc.CreatePresentation(ctx, vasos, app.PresentationInput{Name: "Paquete", Qty: dd("50"), UnitID: unitID(t, st, "pieza"),
			Tare: tareField("10")}); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("tara en piezas = %v; quería 400", err)
		}
	})
}

type grossKit struct {
	*countKit
	salsa, bottle int64
	g, kg         int16
}

func newGrossKit(t *testing.T) *grossKit {
	t.Helper()
	k := &grossKit{countKit: newCountKit(t)}
	k.ctx = app.WithActorUser(k.ctx, k.user)
	k.salsa = ingredientIn(t, k.st, defaultCompanyID, "Salsa conteo bruto", "ml")
	if _, err := k.svc.SetDensity(k.ctx, k.salsa, app.DensityInput{Method: "manual", Density: decp("1.1")}); err != nil {
		t.Fatal(err)
	}
	if err := k.st.Pool.QueryRow(context.Background(), `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g)
		values ($1, $2, 'Botella 1 L', 1000, 120) returning id`, defaultCompanyID, k.salsa).Scan(&k.bottle); err != nil {
		t.Fatal(err)
	}
	k.g, k.kg = unitID(t, k.st, "g"), unitID(t, k.st, "kg")
	return k
}

func (k *grossKit) gross(weight string, unit int16, containers int) *app.GrossCapture {
	return &app.GrossCapture{Weight: dd(weight), WeightUnitID: unit, PresentationID: k.bottle, Containers: containers,
		SeenTareG: decp("120"), SeenDensity: decp("1.1")}
}

// weighedOf es el desglose de la pesada que el conteo abierto devuelve para un artículo (#204).
func weighedOf(t *testing.T, k *grossKit) *app.CountWeighed {
	t.Helper()
	v, err := k.svc.CurrentCount(k.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range v.Items {
		if it.IngredientID == k.salsa {
			return it.Weighed
		}
	}
	t.Fatal("el artículo no está en el conteo")
	return nil
}

func TestCountingByWeighingWithThePresentation(t *testing.T) {
	t.Parallel()
	k := newGrossKit(t)
	c, err := k.svc.OpenCount(k.ctx, k.user)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("el conteo dice qué se puede pesar y con qué números", func(t *testing.T) {
		v, err := k.svc.CurrentCount(k.ctx)
		if err != nil {
			t.Fatal(err)
		}
		var item *app.CountItem
		for i := range v.Items {
			if v.Items[i].IngredientID == k.salsa {
				item = &v.Items[i]
			}
		}
		if item == nil || !item.GrossWeighable || item.DensityGPerMl == nil || len(item.Presentations) != 1 || item.Presentations[0].TareG == nil {
			t.Fatalf("el artículo no llega listo para pesar con empaque: %+v", item)
		}
	})

	t.Run("3 cerradas más 850 g con 2 empaques son 3,554.5455 ml, y queda lo pesado", func(t *testing.T) {
		got, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.salsa, app.CountCapture{Qty: dd("3000"), Gross: k.gross("850", k.g, 2)}, k.user)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(dd("3554.5455")) {
			t.Fatalf("contado %s; quería 3554.5455", got)
		}
		var gross, tare, density string
		var containers int
		if err := k.st.Pool.QueryRow(context.Background(), `select gross_g::text, gross_containers, tare_g_used::text, density_used::text
			from stock_count_lines where count_id = $1 and ingredient_id = $2`, c.ID, k.salsa).Scan(&gross, &containers, &tare, &density); err != nil {
			t.Fatal(err)
		}
		if dd(gross).Cmp(dd("850")) != 0 || containers != 2 || dd(tare).Cmp(dd("120")) != 0 || dd(density).Cmp(dd("1.1")) != 0 {
			t.Fatalf("lo pesado quedó %s g, %d empaques, %s g vacío, %s g/ml", gross, containers, tare, density)
		}
	})

	t.Run("al volver a abrir, el conteo trae cómo se pesó (#204)", func(t *testing.T) {
		w := weighedOf(t, k)
		if w == nil {
			t.Fatal("lo contado pesando vuelve sin desglose: la pantalla solo puede mostrar el total")
		}
		if !w.GrossG.Equal(dd("850")) || w.Containers != 2 || !w.TareG.Equal(dd("120")) || !w.ClosedQty.Equal(dd("3000")) {
			t.Fatalf("desglose %+v; quería 850 g, 2 empaques de 120 g y 3,000 ml cerrados", *w)
		}
	})

	t.Run("si después cambia lo que pesa vacío, el desglose sigue siendo el de la captura", func(t *testing.T) {
		if _, err := k.st.Pool.Exec(context.Background(), `update ingredient_presentations set tare_g = 125 where id = $1`, k.bottle); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = k.st.Pool.Exec(context.Background(), `update ingredient_presentations set tare_g = 120 where id = $1`, k.bottle)
		})
		if w := weighedOf(t, k); w == nil || !w.TareG.Equal(dd("120")) || !w.ClosedQty.Equal(dd("3000")) {
			t.Fatalf("desglose %+v; quería el de la captura (120 g vacío, 3,000 ml cerrados)", w)
		}
	})

	t.Run("en kg da lo mismo", func(t *testing.T) {
		got, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.salsa, app.CountCapture{Gross: k.gross("0.85", k.kg, 2)}, k.user)
		if err != nil || !got.Equal(dd("554.5455")) {
			t.Fatalf("= %s, %v; quería 554.5455", got, err)
		}
	})

	t.Run("recapturar sin pesar borra lo pesado", func(t *testing.T) {
		if err := k.svc.CaptureCount(k.ctx, c.ID, k.salsa, dd("2000"), k.user); err != nil {
			t.Fatal(err)
		}
		var gross *string
		if err := k.st.Pool.QueryRow(context.Background(), `select gross_g::text from stock_count_lines where count_id = $1 and ingredient_id = $2`,
			c.ID, k.salsa).Scan(&gross); err != nil {
			t.Fatal(err)
		}
		if gross != nil {
			t.Fatalf("quedó el bruto %s de una captura anterior", *gross)
		}
		if w := weighedOf(t, k); w != nil {
			t.Fatalf("recapturado sin pesar, el conteo sigue trayendo una pesada: %+v", *w)
		}
	})

	t.Run("lo que no tiene sentido se rechaza sin guardar", func(t *testing.T) {
		vasos := ingredientIn(t, k.st, defaultCompanyID, "Vasos bruto", "pieza")
		sinDensidad := ingredientIn(t, k.st, defaultCompanyID, "Jarabe sin densidad bruto", "ml")
		if _, err := k.st.Pool.Exec(context.Background(), `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g)
			values ($1, $2, 'Botella', 1000, 120)`, defaultCompanyID, sinDensidad); err != nil {
			t.Fatal(err)
		}
		cases := map[string]struct {
			ing int64
			in  app.CountCapture
			err error
		}{
			"pesa menos que los empaques vacíos":                 {k.salsa, app.CountCapture{Gross: k.gross("200", k.g, 2)}, domain.ErrGrossBelowTare},
			"pesar y pesar piezas a la vez":                      {k.salsa, app.CountCapture{Gross: k.gross("850", k.g, 2), Weight: decp("1"), WeightUnitID: &k.g}, domain.ErrValidation},
			"en ml como unidad de la báscula":                    {k.salsa, app.CountCapture{Gross: k.gross("850", unitID(t, k.st, "ml"), 2)}, domain.ErrValidation},
			"un empaque de otro artículo":                        {vasos, app.CountCapture{Gross: k.gross("850", k.g, 2)}, domain.ErrValidation},
			"cero empaques":                                      {k.salsa, app.CountCapture{Gross: k.gross("850", k.g, 0)}, domain.ErrValidation},
			"en ml sin densidad (no es el error de las compras)": {sinDensidad, app.CountCapture{Gross: k.gross("850", k.g, 2)}, domain.ErrValidation},
		}
		for name, tc := range cases {
			_, err := k.svc.CaptureCountChecked(k.ctx, c.ID, tc.ing, tc.in, k.user)
			if !errors.Is(err, tc.err) || errors.Is(err, domain.ErrUnitKindMismatch) {
				t.Errorf("%s = %v; quería %v", name, err, tc.err)
			}
		}
	})

	t.Run("si la tara o la densidad ya no son las que vio la pantalla, no se guarda", func(t *testing.T) {
		if _, err := k.st.Pool.Exec(context.Background(), `update ingredient_presentations set tare_g = 130 where id = $1`, k.bottle); err != nil {
			t.Fatal(err)
		}
		_, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.salsa, app.CountCapture{Gross: k.gross("850", k.g, 2)}, k.user)
		if !errors.Is(err, domain.ErrCountInputsChanged) {
			t.Fatalf("= %v; quería 409 COUNT_INPUTS_CHANGED", err)
		}
		in := k.gross("850", k.g, 2)
		in.SeenTareG = decp("130")
		in.SeenDensity = decp("1.2")
		if _, err := k.svc.CaptureCountChecked(k.ctx, c.ID, k.salsa, app.CountCapture{Gross: in}, k.user); !errors.Is(err, domain.ErrCountInputsChanged) {
			t.Fatalf("con otra densidad = %v; quería 409", err)
		}
	})

	t.Run("lo de otra empresa no se puede pesar", func(t *testing.T) {
		other := makeCompany(t, k.st, "bruto-ajeno")
		inTheThreeCases(t, int64(defaultCompanyID), other, func(t *testing.T, st *store.Store, ctx context.Context) {
			s := app.NewBackofficeService(st, nil)
			if _, err := s.CaptureCountChecked(ctx, c.ID, k.salsa, app.CountCapture{Gross: k.gross("850", k.g, 2)}, k.user); err == nil {
				t.Fatal("se capturó lo pesado en el conteo de otra empresa")
			}
		})
	})
}

// Unir insumos (spec 056) con densidad y tara: el destino que no las tiene recibe las del origen; con
// densidades distintas no se adivina cuál es la buena.
func TestMergingIntoAnArticleWithoutDensityTakesTheSourceDensityAndTare(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	owner := context.Background()
	if _, err := k.svc.SetDensity(k.ctx, k.b, app.DensityInput{Method: "manual", Density: decp("1.03")}); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(owner, `insert into ingredient_presentations (company_id, ingredient_id, name, qty_in_base, tare_g, brand)
		values ($1, $2, 'Bolsa', 1000, null, 'Leche unir marca B'), ($1, $3, 'Bolsa', 1000, 15, null)`, defaultCompanyID, k.a, k.b); err != nil {
		t.Fatal(err)
	}
	got := k.merge(t, k.a, k.req(k.b))
	if got.DensityFrom == nil || got.DensityConflict {
		t.Fatalf("la unión no dijo de dónde salió la densidad: %+v / %v", got.DensityFrom, got.DensityConflict)
	}
	var density, tare *string
	if err := k.st.Pool.QueryRow(owner, `select i.density_g_per_ml::text, p.tare_g::text from ingredients i
		join ingredient_presentations p on p.ingredient_id = i.id and p.active where i.id = $1`, k.a).Scan(&density, &tare); err != nil {
		t.Fatal(err)
	}
	if density == nil || tare == nil {
		t.Fatalf("el destino quedó sin densidad (%v) o su bolsa sin lo que pesa vacía (%v)", density, tare)
	}
}

func TestMergingSourcesWithDifferentDensitiesCopiesNone(t *testing.T) {
	t.Parallel()
	k := newMergeKit(t)
	c := ingredientIn(t, k.st, defaultCompanyID, "Leche unir marca C", "g")
	for id, d := range map[int64]string{k.b: "1.03", c: "1.05"} {
		if _, err := k.svc.SetDensity(k.ctx, id, app.DensityInput{Method: "manual", Density: decp(d)}); err != nil {
			t.Fatal(err)
		}
	}
	got := k.merge(t, k.a, k.req(k.b, c))
	if !got.DensityConflict || got.DensityFrom != nil {
		t.Fatalf("con 1.03 y 1.05 no debía copiar ninguna: from=%v conflict=%v", got.DensityFrom, got.DensityConflict)
	}
	var density *string
	if err := k.st.Pool.QueryRow(context.Background(), `select density_g_per_ml::text from ingredients where id = $1`, k.a).Scan(&density); err != nil {
		t.Fatal(err)
	}
	if density != nil {
		t.Fatalf("el destino adivinó la densidad %s", *density)
	}
}
