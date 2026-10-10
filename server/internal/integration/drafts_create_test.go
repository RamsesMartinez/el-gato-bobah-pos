//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA CUENTA NACE CON EL PRIMER PRODUCTO, CON SU NOMBRE (US2, FR-001, D-2, D-6, FR-020).
func TestDraftIsBornWithItsFirstProduct(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	taro := makeProduct(t, k.st, "Taro de la cuenta", pesos("55"), true)
	ctx := k.ctx

	id := uuid.New()
	first := addOf(taro, "1")
	v, created, err := k.drafts.Create(ctx, app.CreateDraftCmd{ID: id, Lines: []app.DraftLineCmd{first}, Actor: k.user})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !created || v.ID != id || v.Status != domain.DraftCapturing {
		t.Fatalf("created=%v id=%v status=%s", created, v.ID, v.Status)
	}
	if v.FolioName == nil || !contieneNombre(domain.NombresDelEsquema(domain.EsquemaPorDefecto), *v.FolioName) {
		t.Fatalf("folioName = %v: tiene que salir de la bolsa del negocio", v.FolioName)
	}
	if v.OpenedBy == "" {
		t.Fatal("openedBy vacío: quién capturó se pierde si no se guarda desde que nace (FR-020)")
	}
	var openedBy int64
	var scheme string
	if err := k.st.Pool.QueryRow(context.Background(), `select opened_by, folio_scheme::text from order_drafts where id = $1`, id).
		Scan(&openedBy, &scheme); err != nil {
		t.Fatal(err)
	}
	if openedBy != k.user || scheme != "razas" {
		t.Fatalf("opened_by=%d scheme=%s", openedBy, scheme)
	}
	if len(v.Lines) != 1 || !v.Total.Equal(pesos("55")) {
		t.Fatalf("lines=%d total=%s", len(v.Lines), v.Total)
	}

	t.Run("el reintento con el mismo id devuelve la misma cuenta sin duplicar renglones", func(t *testing.T) {
		again, created, err := k.drafts.Create(ctx, app.CreateDraftCmd{ID: id, Lines: []app.DraftLineCmd{first}, Actor: k.user})
		if err != nil {
			t.Fatalf("reintento: %v", err)
		}
		if created || again.ID != id || *again.FolioName != *v.FolioName || len(again.Lines) != 1 || !lineQty(t, again, taro).Equal(pesos("1")) {
			t.Fatalf("created=%v nombre=%v renglones=%d qty=%s: el reintento duplicó o cambió la cuenta",
				created, again.FolioName, len(again.Lines), lineQty(t, again, taro))
		}
	})

	t.Run("sin turno abierto nace igual y no toca almacén, turno ni fecha", func(t *testing.T) {
		var orders, moves int
		if err := k.st.Pool.QueryRow(context.Background(),
			`select (select count(*) from orders), (select count(*) from stock_movements)`).Scan(&orders, &moves); err != nil {
			t.Fatal(err)
		}
		if orders != 0 || moves != 0 {
			t.Fatalf("pedidos=%d movimientos=%d: una cuenta en captura no es pedido ni descuenta inventario (D-6)", orders, moves)
		}
	})
}

func TestDraftNameProposal(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café del nombre", pesos("30"), false)

	propose := func(t *testing.T, name string) *app.DraftView {
		t.Helper()
		v, _, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), FolioName: name,
			Lines: []app.DraftLineCmd{addOf(cafe, "1")}, Actor: k.user})
		if err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
		return v
	}
	a := propose(t, "Persa")
	if *a.FolioName != "Persa" {
		t.Fatalf("propuso Persa libre y salió %s: el nombre que la pantalla ya le dijo al cliente se respeta", *a.FolioName)
	}
	b := propose(t, "Persa")
	if *b.FolioName == "Persa" {
		t.Fatal("dos cuentas vivas con «Persa»: se entregan cruzadas")
	}
}

// Dos tabletas abren cuenta a la vez y proponen el mismo nombre (US2 AS2).
func TestTwoDraftsNeverShareAName(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café de la carrera", pesos("30"), false)

	// Una conexión por tableta, para todas las vueltas: el pool del rol es chico.
	ctxs := []context.Context{k.tenant(t, k.company), k.tenant(t, k.company)}
	for round := 0; round < 5; round++ {
		names := make([]string, 2)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				v, _, err := k.drafts.Create(ctxs[i], app.CreateDraftCmd{ID: uuid.New(), FolioName: "Siamés",
					Lines: []app.DraftLineCmd{addOf(cafe, "1")}, Actor: k.user})
				errs[i] = err
				if err == nil {
					names[i] = *v.FolioName
				}
			}()
		}
		wg.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatalf("vuelta %d: una de las dos tabletas no pudo abrir cuenta: %v", round, err)
			}
		}
		if names[0] == names[1] {
			t.Fatalf("vuelta %d: las dos cuentas salieron como %q", round, names[0])
		}
		// Se sueltan para que la siguiente vuelta vuelva a competir por «Siamés».
		if _, err := k.st.Pool.Exec(context.Background(),
			`update order_drafts set status = 'descartada', discarded_at = now(), discard_reason = 'manual' where status = 'capturando'`); err != nil {
			t.Fatal(err)
		}
		if _, err := k.st.Pool.Exec(context.Background(), `delete from folio_consumido`); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDraftRejectsAnotherCompanysProduct(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	other := makeCompany(t, k.st, "otra-cuenta")
	var ajeno int64
	if err := k.st.Pool.QueryRow(context.Background(), `
		with c as (insert into categories (company_id, name) values ($1, 'ajena') returning id)
		insert into products (company_id, name, category_id, price) select $1, 'Producto ajeno', id, 10 from c returning id`,
		other).Scan(&ajeno); err != nil {
		t.Fatal(err)
	}
	_, _, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), Lines: []app.DraftLineCmd{addOf(ajeno, "1")}, Actor: k.user})
	if !errors.Is(err, domain.ErrProductNotSell) {
		t.Fatalf("producto de otra empresa = %v, quería que no está en este menú", err)
	}
}

// Validación en la frontera: nada absurdo llega a la tabla.
func TestDraftCreateRejectsBadInput(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café de la frontera", pesos("30"), false)
	cases := map[string]app.CreateDraftCmd{
		"sin renglones":       {ID: uuid.New(), Actor: k.user},
		"sin id":              {Lines: []app.DraftLineCmd{addOf(cafe, "1")}, Actor: k.user},
		"cantidad cero":       {ID: uuid.New(), Lines: []app.DraftLineCmd{addOf(cafe, "0")}, Actor: k.user},
		"agregado sin opId":   {ID: uuid.New(), Lines: []app.DraftLineCmd{{ProductID: cafe, Qty: pesos("1")}}, Actor: k.user},
		"cabecera en «Nuevo»": {ID: uuid.New(), OrderID: new(int64(1)), Header: &app.DraftHeaderPatch{}, Lines: []app.DraftLineCmd{addOf(cafe, "1")}, Actor: k.user},
	}
	for name, cmd := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := k.drafts.Create(k.ctx, cmd); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("Create = %v, quería 400", err)
			}
		})
	}
}

// LOS TRES CASOS DONDE RLS YA FALLÓ, sobre crear, leer y los nombres vivos.
func TestDraftsInTheThreeCases(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	other := makeCompany(t, k.st, "otra-tres")
	otherUser := makeUserIn(t, k.st, other, "cajero_otra_tres", "cajero")
	cafe := makeProduct(t, k.st, "Café de los tres", pesos("30"), false)
	var cafeB int64
	if err := k.st.Pool.QueryRow(context.Background(), `
		with c as (insert into categories (company_id, name) values ($1, 'b') returning id)
		insert into products (company_id, name, category_id, price) select $1, 'Café B', id, 10 from c returning id`,
		other).Scan(&cafeB); err != nil {
		t.Fatal(err)
	}
	mine, _, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: uuid.New(), FolioName: "Bombay",
		Lines: []app.DraftLineCmd{addOf(cafe, "1")}, Actor: k.user})
	if err != nil {
		t.Fatal(err)
	}

	inTheThreeCases(t, k.company, other, func(t *testing.T, st *store.Store, ctx context.Context) {
		svc := app.NewDraftsService(st, app.NewOrdersService(st, clock))
		if _, err := svc.Get(ctx, mine.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("leer la cuenta de otra empresa = %v, quería no encontrada", err)
		}
		if _, err := svc.AddLine(ctx, mine.ID, addOf(cafe, "1")); err == nil {
			t.Fatal("agregó a la cuenta de otra empresa")
		}
		if _, ok := store.CompanyFrom(ctx); !ok {
			// Sin empresa no se crea nada: no hay de quién sería.
			if _, _, err := svc.Create(ctx, app.CreateDraftCmd{ID: uuid.New(), Lines: []app.DraftLineCmd{addOf(cafeB, "1")}, Actor: otherUser}); err == nil {
				t.Fatal("se creó una cuenta sin empresa")
			}
			return
		}
		// La otra empresa SÍ puede llamar «Bombay» a su cuenta: los nombres vivos son por empresa, y
		// si la consulta de nombres vivos viera los de la dueña se lo negaría.
		v, _, err := svc.Create(ctx, app.CreateDraftCmd{ID: uuid.New(), FolioName: "Bombay",
			Lines: []app.DraftLineCmd{addOf(cafeB, "1")}, Actor: otherUser})
		if err != nil {
			t.Fatalf("crear en la otra empresa: %v", err)
		}
		if *v.FolioName != "Bombay" {
			t.Fatalf("la otra empresa recibió %s: la consulta de nombres vivos vio los de la dueña", *v.FolioName)
		}
	})
	if got := draftStatus(t, k.st, mine.ID); got != domain.DraftCapturing {
		t.Fatalf("la cuenta de la dueña quedó %s", got)
	}
}

// 12 HORAS SIN TOCAR: SE DESCARTA SOLA Y SU NOMBRE VUELVE A LA BOLSA (D-8, FR-013).
//
// El barrido es perezoso: corre cuando alguien crea una cuenta o mira la fila. Lo que se prueba es
// que el reloj sea el del ÚLTIMO cambio y no el de creación, y que el nombre se suelte.
func TestIdleDraftExpires(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café dormido", pesos("30"), false)
	old := k.newDraft(t, addOf(cafe, "1"))
	recent := k.newDraft(t, addOf(cafe, "1"))
	ctx := context.Background()
	// La vieja: creada hace dos días, tocada hace 13 h. La reciente: creada hace dos días también,
	// pero tocada hace 11 h — vive, aunque haya nacido antes del límite.
	if _, err := k.st.Pool.Exec(ctx, `update order_drafts set created_at = now() - interval '2 days',
		updated_at = now() - interval '13 hours' where id = $1`, old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := k.st.Pool.Exec(ctx, `update order_drafts set created_at = now() - interval '2 days',
		updated_at = now() - interval '11 hours' where id = $1`, recent.ID); err != nil {
		t.Fatal(err)
	}
	// El nombre se marcó al nacer la cuenta: se mueve su marca al mismo instante, como habría sido.
	if _, err := k.st.Pool.Exec(ctx, `update folio_consumido f set taken_at = d.created_at
		from order_drafts d where d.folio_name = f.name`); err != nil {
		t.Fatal(err)
	}

	k.newDraft(t, addOf(cafe, "1")) // cualquier creación barre primero

	var status, reason string
	if err := k.st.Pool.QueryRow(ctx, `select status, coalesce(discard_reason, '') from order_drafts where id = $1`, old.ID).
		Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if status != domain.DraftDiscarded || reason != domain.DiscardExpired {
		t.Fatalf("la de 13 h quedó %s/%s, quería descartada por vencida", status, reason)
	}
	if got := draftStatus(t, k.st, recent.ID); got != domain.DraftCapturing {
		t.Fatalf("la de 11 h quedó %s: las 12 horas cuentan desde el último cambio", got)
	}
	var still int
	if err := k.st.Pool.QueryRow(ctx, `select count(*) from folio_consumido where name = $1`, *old.FolioName).Scan(&still); err != nil {
		t.Fatal(err)
	}
	if still != 0 {
		t.Fatalf("el nombre %s de la cuenta vencida sigue fuera de la bolsa", *old.FolioName)
	}
}
