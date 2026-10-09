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
)

// La tableta reintenta crear con el MISMO id mientras la primera petición sigue en vuelo (la red
// tardó, el usuario tocó otra vez). Antes, la transacción que llegaba segunda veía cero filas en el
// `on conflict do nothing` —la vecina ya había hecho commit— y lo tomaba por «id de otra empresa»:
// 409 «esa cuenta no se puede crear» sobre una cuenta que sí se creó.
func TestParallelCreateWithTheSameIDIsARetryNotAConflict(t *testing.T) {
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café del reintento", pesos("30"), false)
	const tablets = 3
	ctxs := make([]context.Context, tablets)
	for i := range ctxs {
		ctxs[i] = k.tenant(t, k.company)
	}
	for round := range 5 {
		id := uuid.New()
		first := addOf(cafe, "1")
		views := make([]*app.DraftView, tablets)
		created := make([]bool, tablets)
		errs := make([]error, tablets)
		var wg sync.WaitGroup
		for i := range tablets {
			wg.Add(1)
			go func() {
				defer wg.Done()
				views[i], created[i], errs[i] = k.drafts.Create(ctxs[i], app.CreateDraftCmd{ID: id, FolioName: "Persa",
					Lines: []app.DraftLineCmd{first}, Actor: k.user})
			}()
		}
		wg.Wait()
		nuevos := 0
		for i := range tablets {
			if errs[i] != nil {
				t.Fatalf("vuelta %d: el reintento en paralelo respondió error sobre una cuenta que sí se creó: %v", round, errs[i])
			}
			if views[i].ID != id || len(views[i].Lines) != 1 || !lineQty(t, views[i], cafe).Equal(pesos("1")) {
				t.Fatalf("vuelta %d: id=%v renglones=%d qty=%s: el reintento duplicó o cambió la cuenta",
					round, views[i].ID, len(views[i].Lines), lineQty(t, views[i], cafe))
			}
			if created[i] {
				nuevos++
			}
		}
		if nuevos != 1 {
			t.Fatalf("vuelta %d: %d peticiones dijeron que crearon la cuenta; es una sola", round, nuevos)
		}
		var vivas, consumidos int
		if err := k.st.Pool.QueryRow(context.Background(), `select
			(select count(*) from order_drafts where status = 'capturando'),
			(select count(*) from folio_consumido)`).Scan(&vivas, &consumidos); err != nil {
			t.Fatal(err)
		}
		if vivas != 1 || consumidos != 1 {
			t.Fatalf("vuelta %d: cuentas vivas=%d nombres gastados=%d: el reintento gastó un nombre de la bolsa sin usarlo",
				round, vivas, consumidos)
		}
		if _, err := k.st.Pool.Exec(context.Background(),
			`update order_drafts set status = 'descartada', discarded_at = now(), discard_reason = 'manual' where status = 'capturando'`); err != nil {
			t.Fatal(err)
		}
		if _, err := k.st.Pool.Exec(context.Background(), `delete from folio_consumido`); err != nil {
			t.Fatal(err)
		}
	}
}

// Lo mismo por la subida de pestañas: dos subidas idénticas a la vez (la tableta reintentó) daban
// 200 y 409 de la petición entera, y la tableta que recibió el 409 no borraba su copia.
func TestParallelImportOfTheSameTabIsARetryNotAConflict(t *testing.T) {
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café de la subida doble", pesos("30"), false)
	ctxs := []context.Context{k.tenant(t, k.company), k.tenant(t, k.company)}
	for round := range 5 {
		tab := app.ImportAccount{ID: uuid.New(), Lines: []app.DraftLineCmd{addOf(cafe, "2")}}
		results := make([][]app.ImportResult, 2)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i], errs[i] = k.drafts.Import(ctxs[i], []app.ImportAccount{tab}, k.user)
			}()
		}
		wg.Wait()
		outcomes := map[string]int{}
		for i := range 2 {
			if errs[i] != nil {
				t.Fatalf("vuelta %d: una subida idéntica en paralelo falló entera: %v", round, errs[i])
			}
			outcomes[results[i][0].Outcome]++
		}
		if outcomes["created"] != 1 || outcomes["exists"] != 1 {
			t.Fatalf("vuelta %d: resultados %v, quería un created y un exists", round, outcomes)
		}
	}
}

// El arreglo de arriba no puede abrir un oráculo: un id que ya es de OTRA empresa sigue sin revelarse.
// Responder «ya existe» (o devolver la cuenta) le diría a una tableta qué ids usa otro negocio.
func TestCreateWithAnotherCompanysDraftIDRevealsNothing(t *testing.T) {
	k := newDraftsKit(t)
	other := makeCompany(t, k.st, "otra-mismo-id")
	otherUser := makeUserIn(t, k.st, other, "cajero_otra_mismo_id", "cajero")
	var cafeB int64
	if err := k.st.Pool.QueryRow(context.Background(), `
		with c as (insert into categories (company_id, name) values ($1, 'b') returning id)
		insert into products (company_id, name, category_id, price) select $1, 'Café B mismo id', id, 10 from c returning id`,
		other).Scan(&cafeB); err != nil {
		t.Fatal(err)
	}
	theirs, _, err := k.drafts.Create(k.tenant(t, other), app.CreateDraftCmd{ID: uuid.New(), Actor: otherUser,
		Lines: []app.DraftLineCmd{addOf(cafeB, "1")}})
	if err != nil {
		t.Fatal(err)
	}
	cafe := makeProduct(t, k.st, "Café A mismo id", pesos("30"), false)
	v, _, err := k.drafts.Create(k.ctx, app.CreateDraftCmd{ID: theirs.ID, Lines: []app.DraftLineCmd{addOf(cafe, "1")}, Actor: k.user})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("crear con el id de una cuenta de otra empresa = (%v, %v), quería el mismo 409 de siempre", v, err)
	}
	var consumidos int
	if err := k.st.Pool.QueryRow(context.Background(),
		`select count(*) from folio_consumido where company_id = $1`, k.company).Scan(&consumidos); err != nil {
		t.Fatal(err)
	}
	if consumidos != 0 {
		t.Fatalf("el intento rechazado gastó %d nombres de la bolsa", consumidos)
	}
}
