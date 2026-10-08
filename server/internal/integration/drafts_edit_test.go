//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"uuid"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

func TestAddingIsIdempotentByOp(t *testing.T) {
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café idempotente", pesos("30"), false)
	v := k.newDraft(t, addOf(cafe, "1"))

	op := addOf(cafe, "1")
	for range 3 {
		if _, err := k.drafts.AddLine(k.ctx, v.ID, op); err != nil {
			t.Fatalf("AddLine: %v", err)
		}
	}
	got, err := k.drafts.Get(k.ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if q := lineQty(t, got, cafe); !q.Equal(pesos("2")) {
		t.Fatalf("qty = %s tras reintentar 3 veces el mismo toque, quería 2 (el primero + uno)", q)
	}
}

// Dos tabletas tocan el mismo producto a la vez: un renglón con 2, no dos de 1 (D-5, caso 17).
func TestConcurrentAddsMergeIntoOneLine(t *testing.T) {
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café concurrente", pesos("30"), false)
	leche := makeProduct(t, k.st, "Leche concurrente", pesos("10"), false)
	v := k.newDraft(t, addOf(leche, "1"))

	// Una conexión por tableta, para todas las vueltas: el pool del rol es chico.
	ctxs := []context.Context{k.tenant(t, k.company), k.tenant(t, k.company)}
	for round := range 5 {
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = k.drafts.AddLine(ctxs[i], v.ID, addOf(cafe, "1"))
			}()
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatalf("vuelta %d: %v", round, err)
			}
		}
	}
	got, err := k.drafts.Get(k.ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	var cafeLines int
	for _, l := range got.Lines {
		if l.ProductID == cafe {
			cafeLines++
		}
	}
	if cafeLines != 1 || !lineQty(t, got, cafe).Equal(pesos("10")) {
		t.Fatalf("renglones de café = %d con %s: diez toques de dos tabletas son UN renglón de 10", cafeLines, lineQty(t, got, cafe))
	}
	var dupPositions int
	if err := k.st.Pool.QueryRow(context.Background(), `
		select count(*) from (select position from order_draft_lines where draft_id = $1 group by position having count(*) > 1) x`,
		v.ID).Scan(&dupPositions); err != nil {
		t.Fatal(err)
	}
	if dupPositions != 0 {
		t.Fatal("dos renglones con la misma posición: la cuenta se pinta en un orden distinto en cada tableta")
	}
}

func TestPlusOnALine(t *testing.T) {
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café del más", pesos("30"), false)
	v := k.newDraft(t, app.DraftLineCmd{OpID: uuid.New(), ProductID: cafe, Qty: pesos("1"), Notes: "tibio"})
	lineID := v.Lines[0].ID

	got, err := k.drafts.AddLine(k.ctx, v.ID, app.DraftLineCmd{OpID: uuid.New(), IntoLineID: &lineID, Qty: pesos("1")})
	if err != nil {
		t.Fatalf("«+» sobre el renglón: %v", err)
	}
	if len(got.Lines) != 1 || !got.Lines[0].Qty.Equal(pesos("2")) || got.Lines[0].Notes != "tibio" {
		t.Fatalf("el «+» de un renglón con nota tiene que sumarle a ESE renglón: %+v", got.Lines)
	}

	t.Run("sobre un renglón que otra tableta quitó", func(t *testing.T) {
		if _, err := k.drafts.RemoveLine(k.ctx, v.ID, lineID, got.Lines[0].Version); err != nil {
			t.Fatal(err)
		}
		_, err := k.drafts.AddLine(k.ctx, v.ID, app.DraftLineCmd{OpID: uuid.New(), IntoLineID: &lineID, Qty: pesos("1")})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("«+» sobre un renglón quitado = %v, quería 404 (la tableta recarga)", err)
		}
	})
}

func TestStaleChangeIsRejected(t *testing.T) {
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café viejo", pesos("30"), false)
	v := k.newDraft(t, addOf(cafe, "3"))
	line := v.Lines[0]

	// Tableta A baja a 2; tableta B, con la versión de antes, quiere bajar a 1.
	if _, err := k.drafts.ChangeLine(k.ctx, app.ChangeDraftLineCmd{DraftID: v.ID, LineID: line.ID,
		ExpectedVersion: line.Version, Qty: new(pesos("2"))}); err != nil {
		t.Fatalf("A: %v", err)
	}
	_, err := k.drafts.ChangeLine(k.ctx, app.ChangeDraftLineCmd{DraftID: v.ID, LineID: line.ID,
		ExpectedVersion: line.Version, Qty: new(pesos("1"))})
	if !errors.Is(err, domain.ErrDraftChanged) {
		t.Fatalf("cambiar con versión vieja = %v, quería DRAFT_CHANGED", err)
	}
	_, err = k.drafts.RemoveLine(k.ctx, v.ID, line.ID, line.Version)
	if !errors.Is(err, domain.ErrDraftChanged) {
		t.Fatalf("quitar con versión vieja = %v, quería DRAFT_CHANGED", err)
	}
	got, _ := k.drafts.Get(k.ctx, v.ID)
	if !lineQty(t, got, cafe).Equal(pesos("2")) {
		t.Fatalf("qty = %s: un rechazo no aplica nada", lineQty(t, got, cafe))
	}

	t.Run("un «+» de la otra tableta también vuelve vieja la versión", func(t *testing.T) {
		// El «−» manda la cantidad ABSOLUTA: aplicarlo sobre un «+» ajeno borraría lo que la otra agregó.
		cur, _ := k.drafts.Get(k.ctx, v.ID)
		seen := cur.Lines[0].Version
		into := cur.Lines[0].ID
		if _, err := k.drafts.AddLine(k.ctx, v.ID, app.DraftLineCmd{OpID: uuid.New(), IntoLineID: &into, Qty: pesos("1")}); err != nil {
			t.Fatal(err)
		}
		_, err := k.drafts.ChangeLine(k.ctx, app.ChangeDraftLineCmd{DraftID: v.ID, LineID: into, ExpectedVersion: seen, Qty: new(pesos("1"))})
		if !errors.Is(err, domain.ErrDraftChanged) {
			t.Fatalf("= %v, quería DRAFT_CHANGED", err)
		}
	})

	t.Run("cantidad cero en un cambio se rechaza (para quitar está DELETE)", func(t *testing.T) {
		cur, _ := k.drafts.Get(k.ctx, v.ID)
		_, err := k.drafts.ChangeLine(k.ctx, app.ChangeDraftLineCmd{DraftID: v.ID, LineID: cur.Lines[0].ID,
			ExpectedVersion: cur.Lines[0].Version, Qty: new(pesos("0"))})
		if !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("= %v, quería 400", err)
		}
	})
}

func TestRemovingTheLastLineKeepsTheAccount(t *testing.T) {
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café último", pesos("30"), false)
	v := k.newDraft(t, addOf(cafe, "1"))
	got, err := k.drafts.RemoveLine(k.ctx, v.ID, v.Lines[0].ID, v.Lines[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.DraftCapturing || len(got.Lines) != 0 || *got.FolioName != *v.FolioName {
		t.Fatalf("status=%s renglones=%d nombre=%v: quitar el último deja la cuenta vacía y viva, con su nombre",
			got.Status, len(got.Lines), got.FolioName)
	}
	if got.Lines == nil || got.Unavailable == nil {
		t.Fatal("lines/unavailable nil: saldrían como null en el JSON")
	}
}

func TestDraftHeader(t *testing.T) {
	k := newDraftsKit(t)
	jefa := makeUser(t, k.st, "gerente_descuento", "gerente")
	crepa := makeProduct(t, k.st, "Crepa de cabecera", pesos("100"), false)
	uber := platformID(t, k.st, defaultCompanyID, "Uber Eats")
	v := k.newDraft(t, addOf(crepa, "1"))

	patch := func(t *testing.T, p app.DraftHeaderPatch, actor int64) (*app.DraftView, error) {
		t.Helper()
		cur, err := k.drafts.Get(k.ctx, v.ID)
		if err != nil {
			t.Fatal(err)
		}
		if p.ExpectedVersion == 0 {
			p.ExpectedVersion = cur.HeaderVersion
		}
		res, err := k.drafts.PatchHeader(k.ctx, v.ID, p, actor)
		return res.View, err
	}
	setBy := func(t *testing.T) (discount, ref *int64) {
		t.Helper()
		if err := k.st.Pool.QueryRow(context.Background(), `select discount_set_by, platform_ref_set_by from order_drafts where id = $1`, v.ID).
			Scan(&discount, &ref); err != nil {
			t.Fatal(err)
		}
		return
	}

	t.Run("versión vieja", func(t *testing.T) {
		if _, err := patch(t, app.DraftHeaderPatch{ExpectedVersion: -1, CustomerName: app.Field[string]{Set: true, Value: new("Mesa 4")}}, k.user); !errors.Is(err, domain.ErrDraftChanged) {
			t.Fatalf("= %v, quería DRAFT_CHANGED", err)
		}
	})

	t.Run("el descuento lo firma quien lo puso, no quien abrió la cuenta", func(t *testing.T) {
		got, err := patch(t, app.DraftHeaderPatch{Discount: app.Field[app.DraftDiscount]{Set: true, Value: &app.DraftDiscount{Percent: new(pesos("10"))}}}, jefa)
		if err != nil {
			t.Fatal(err)
		}
		if !got.DiscountTotal.Equal(pesos("10")) || !got.Total.Equal(pesos("90")) {
			t.Fatalf("descuento=%s total=%s", got.DiscountTotal, got.Total)
		}
		if d, _ := setBy(t); d == nil || *d != jefa {
			t.Fatalf("discount_set_by = %v, quería %d", d, jefa)
		}
	})

	t.Run("quitar el descuento borra su firma", func(t *testing.T) {
		if _, err := patch(t, app.DraftHeaderPatch{Discount: app.Field[app.DraftDiscount]{Set: true}}, k.user); err != nil {
			t.Fatal(err)
		}
		if d, _ := setBy(t); d != nil {
			t.Fatalf("discount_set_by = %d sin descuento", *d)
		}
	})

	t.Run("descuento mayor que la venta", func(t *testing.T) {
		_, err := patch(t, app.DraftHeaderPatch{Discount: app.Field[app.DraftDiscount]{Set: true, Value: &app.DraftDiscount{Amount: new(pesos("101"))}}}, k.user)
		if !errors.Is(err, domain.ErrDescuentoMayorQueLaVenta) {
			t.Fatalf("= %v", err)
		}
	})

	t.Run("cambiar a plataforma reprecia; cambiar de plataforma tira el folio", func(t *testing.T) {
		got, err := patch(t, app.DraftHeaderPatch{PlatformID: app.Field[int16]{Set: true, Value: &uber},
			PlatformOrderRef: app.Field[string]{Set: true, Value: new("  UB-1  ")}}, jefa)
		if err != nil {
			t.Fatal(err)
		}
		if got.Total.Equal(pesos("100")) {
			t.Fatal("con Uber el total sigue a precio de mostrador: no se reprecio")
		}
		if got.PlatformOrderRef == nil || *got.PlatformOrderRef != "UB-1" {
			t.Fatalf("folio = %v, quería UB-1 recortado", got.PlatformOrderRef)
		}
		if _, ref := setBy(t); ref == nil || *ref != jefa {
			t.Fatalf("platform_ref_set_by = %v, quería %d", ref, jefa)
		}
		got, err = patch(t, app.DraftHeaderPatch{PlatformID: app.Field[int16]{Set: true}}, k.user)
		if err != nil {
			t.Fatal(err)
		}
		if got.PlatformOrderRef != nil || !got.Total.Equal(pesos("100")) {
			t.Fatalf("de vuelta a mostrador: folio=%v total=%s", got.PlatformOrderRef, got.Total)
		}
		if _, ref := setBy(t); ref != nil {
			t.Fatal("platform_ref_set_by quedó sin folio")
		}
	})

	t.Run("folio de plataforma sin plataforma", func(t *testing.T) {
		if _, err := patch(t, app.DraftHeaderPatch{PlatformOrderRef: app.Field[string]{Set: true, Value: new("X1")}}, k.user); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("= %v", err)
		}
	})

	t.Run("plataforma de otra empresa", func(t *testing.T) {
		other := makeCompany(t, k.st, "otra-plataforma")
		ajena := platformID(t, k.st, other, "Uber Eats")
		if _, err := patch(t, app.DraftHeaderPatch{PlatformID: app.Field[int16]{Set: true, Value: &ajena}}, k.user); !errors.Is(err, domain.ErrPlatformNotFound) {
			t.Fatalf("= %v", err)
		}
	})

	t.Run("servicio inválido", func(t *testing.T) {
		if _, err := patch(t, app.DraftHeaderPatch{ServiceType: new("dron")}, k.user); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("= %v", err)
		}
	})
}

func TestDraftLimits(t *testing.T) {
	k := newDraftsKit(t)
	p0 := makeProduct(t, k.st, "P0", pesos("1"), false)
	v := k.newDraft(t, addOf(p0, "1"))
	// Hasta el tope con renglones que no se fusionan (cada uno con su nota).
	cafe := makeProduct(t, k.st, "Café del tope", pesos("1"), false)
	for i := 1; i < domain.MaxDraftLines; i++ {
		if _, err := k.drafts.AddLine(k.ctx, v.ID, app.DraftLineCmd{OpID: uuid.New(), ProductID: cafe, Qty: pesos("1"), Notes: "n" + itoa(i)}); err != nil {
			t.Fatalf("renglón %d: %v", i, err)
		}
	}
	_, err := k.drafts.AddLine(k.ctx, v.ID, app.DraftLineCmd{OpID: uuid.New(), ProductID: cafe, Qty: pesos("1"), Notes: "uno de más"})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("renglón %d = %v, quería 400", domain.MaxDraftLines+1, err)
	}
	// Fusionar con uno existente no crea renglón: sí se permite en el tope.
	if _, err := k.drafts.AddLine(k.ctx, v.ID, addOf(p0, "1")); err != nil {
		t.Fatalf("fusionar en el tope: %v", err)
	}
}

func TestDeactivatedProductInALiveDraft(t *testing.T) {
	k := newDraftsKit(t)
	taro := makeProduct(t, k.st, "Taro que se agota", pesos("55"), false)
	cafe := makeProduct(t, k.st, "Café que sigue", pesos("30"), false)
	v := k.newDraft(t, addOf(taro, "1"), addOf(cafe, "1"))
	if _, err := k.st.Pool.Exec(context.Background(), `update products set is_active = false where id = $1`, taro); err != nil {
		t.Fatal(err)
	}
	got, err := k.drafts.Get(k.ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range got.Lines {
		if l.ProductID == taro && l.Available {
			t.Fatal("el producto desactivado sigue disponible en la cuenta")
		}
	}
	if !got.Total.Equal(pesos("30")) || len(got.Unavailable) != 1 || got.Unavailable[0] != "Taro que se agota" {
		t.Fatalf("total=%s unavailable=%v: lo que ya no se vende no suma y se nombra", got.Total, got.Unavailable)
	}
}

func TestTerminalDraftsReceiveNothing(t *testing.T) {
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café terminal", pesos("30"), false)
	for status, want := range map[string]error{"enviada": domain.ErrDraftAlreadySent, "descartada": domain.ErrDraftDiscarded} {
		t.Run(status, func(t *testing.T) {
			v := k.newDraft(t, addOf(cafe, "1"))
			if status == "enviada" {
				var orderID int64
				if err := k.st.Pool.QueryRow(context.Background(), `
					insert into orders (client_uuid, business_date, daily_number, service_type, opened_by, subtotal, total, status)
					values ($1, current_date, 999, 'mostrador', $2, 0, 0, 'abierta') returning id`, uuid.New(), k.user).Scan(&orderID); err != nil {
					t.Fatal(err)
				}
				if _, err := k.st.Pool.Exec(context.Background(),
					`update order_drafts set status = 'enviada', sent_at = now(), order_id = $2 where id = $1`, v.ID, orderID); err != nil {
					t.Fatal(err)
				}
			} else if _, err := k.st.Pool.Exec(context.Background(),
				`update order_drafts set status = 'descartada', discarded_at = now(), discard_reason = 'manual' where id = $1`, v.ID); err != nil {
				t.Fatal(err)
			}
			line := v.Lines[0]
			calls := map[string]func() error{
				"agregar": func() error { _, err := k.drafts.AddLine(k.ctx, v.ID, addOf(cafe, "1")); return err },
				"cambiar": func() error {
					_, err := k.drafts.ChangeLine(k.ctx, app.ChangeDraftLineCmd{DraftID: v.ID, LineID: line.ID, ExpectedVersion: line.Version, Qty: new(pesos("2"))})
					return err
				},
				"quitar": func() error { _, err := k.drafts.RemoveLine(k.ctx, v.ID, line.ID, line.Version); return err },
				"cabecera": func() error {
					_, err := k.drafts.PatchHeader(k.ctx, v.ID, app.DraftHeaderPatch{ExpectedVersion: v.HeaderVersion,
						CustomerName: app.Field[string]{Set: true, Value: new("x")}}, k.user)
					return err
				},
			}
			for name, call := range calls {
				if err := call(); !errors.Is(err, want) {
					t.Errorf("%s en una cuenta %s = %v, quería %v", name, status, err, want)
				}
			}
		})
	}
}

// LOS TRES CASOS SOBRE CADA ESCRITURA: ninguna alcanza la cuenta de otra empresa.
func TestDraftWritesInTheThreeCases(t *testing.T) {
	k := newDraftsKit(t)
	other := makeCompany(t, k.st, "otra-escritura")
	cafe := makeProduct(t, k.st, "Café ajeno a la escritura", pesos("30"), false)
	v := k.newDraft(t, addOf(cafe, "1"))
	line := v.Lines[0]

	inTheThreeCases(t, k.company, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewDraftsService(st, app.NewOrdersService(st, clock))
		writes := map[string]func() error{
			"agregar": func() error { _, err := svc.AddLine(ctx, v.ID, addOf(cafe, "1")); return err },
			"cambiar": func() error {
				_, err := svc.ChangeLine(ctx, app.ChangeDraftLineCmd{DraftID: v.ID, LineID: line.ID, ExpectedVersion: line.Version, Qty: new(decimal.NewFromInt(5))})
				return err
			},
			"quitar": func() error { _, err := svc.RemoveLine(ctx, v.ID, line.ID, line.Version); return err },
			"cabecera": func() error {
				_, err := svc.PatchHeader(ctx, v.ID, app.DraftHeaderPatch{ExpectedVersion: v.HeaderVersion,
					CustomerName: app.Field[string]{Set: true, Value: new("intruso")}}, k.user)
				return err
			},
		}
		for name, w := range writes {
			if err := w(); err == nil {
				t.Errorf("%s alcanzó la cuenta de otra empresa", name)
			}
		}
	})
	got, err := k.drafts.Get(k.ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 1 || !got.Lines[0].Qty.Equal(pesos("1")) || got.CustomerName != nil {
		t.Fatalf("la cuenta de la dueña cambió desde fuera: %+v", got)
	}
}
