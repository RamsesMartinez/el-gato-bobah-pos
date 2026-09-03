//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// LA MIGRACIÓN LE PONE EL ESQUEMA A LAS EMPRESAS QUE YA EXISTEN, NO SOLO A LAS NUEVAS.
//
// Con DOS empresas a propósito: `add column ... not null default` sella el valor en todas las filas
// existentes, pero eso solo se ve si hay más de una. Con una sola, cualquier backfill "por cada
// empresa" es un no-op y la migración pasa verde para dejar a la segunda sin ajuste.
func TestLaMigracionDejaTodoNegocioNombrandoConRazas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	otra := makeCompany(t, st, "bolsa-otra")

	// La empresa por defecto ya tiene su fila desde el seed; a la nueva se le siembra igual que en
	// provisionCompany.
	if err := st.WithTenant(ctx, otra, func(q *db.Queries) error {
		return q.SeedBusinessSettings(ctx, "Otra")
	}); err != nil {
		t.Fatalf("sembrar ajustes: %v", err)
	}

	for _, empresa := range []int64{defaultCompanyID, otra} {
		var esquema string
		if err := st.Pool.QueryRow(ctx,
			`select folio_scheme::text from business_settings where company_id = $1`, empresa).Scan(&esquema); err != nil {
			t.Fatalf("empresa %d: %v", empresa, err)
		}
		if esquema != string(domain.EsquemaRazas) {
			t.Errorf("la empresa %d nombra con %q, quiere razas: el default nuevo tiene que alcanzar a las que ya existían",
				empresa, esquema)
		}
	}
}

// LA BOLSA SE AGOTA ANTES DE REPETIR, CONTRA POSTGRES.
//
// El unitario prueba la regla; esto prueba que el servicio la persiste. Son cosas distintas: la
// bolsa vive en una tabla y quien la lea mal —sin el esquema, sin vaciarla, o vaciándola de más—
// deja la regla intacta y el comportamiento roto.
//
// Se piden MÁS pedidos que nombres tiene la lista, para cruzar el punto en que la bolsa se vacía:
// es donde el defecto vive.
func TestLosNombresNoSeRepitenHastaAgotarLaBolsa(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewOrdersService(st, clock)

	cajero := makeUser(t, st, "cajero_bolsa", "cajero")
	prod := makeProduct(t, st, "Café bolsa", decimal.RequireFromString("10"), false)
	abrirCajaPrincipal(t, st, cajero)

	lista := domain.NombresDelEsquema(domain.EsquemaRazas)
	total := len(lista) + 5

	vistos := map[string]int{}
	for i := 1; i <= total; i++ {
		ord, err := svc.Create(ctx, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: cajero,
			Lines: []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
		})
		if err != nil {
			t.Fatalf("pedido %d: %v", i, err)
		}
		if ord.FolioName == "" {
			t.Fatalf("el pedido %d salió sin nombre: es con lo que cocina lo canta", i)
		}
		// Todos son del MISMO día, así que ninguno puede repetirse: pasando la vuelta el sufijo
		// numerado es lo correcto, pero el nombre completo sigue siendo único.
		if antes, dup := vistos[ord.FolioName]; dup {
			t.Fatalf("%q salió en el pedido %d y otra vez en el %d", ord.FolioName, antes, i)
		}
		vistos[ord.FolioName] = i

		// Hasta agotar la lista, ningún nombre lleva número: eso solo aparece cuando el día ya pasó
		// del largo de la lista.
		if i <= len(lista) && !contieneNombre(lista, ord.FolioName) {
			t.Fatalf("el pedido %d se llamó %q, que no está en la lista de razas (¿se numeró antes de tiempo?)",
				i, ord.FolioName)
		}
	}

	// Y la bolsa quedó con la vuelta NUEVA a medias, no con las dos vueltas encimadas.
	consumidos, err := st.Q.FolioNamesConsumidos(ctx, db.FolioSchemeRazas)
	if err != nil {
		t.Fatalf("FolioNamesConsumidos: %v", err)
	}
	if len(consumidos) != total-len(lista) {
		t.Errorf("la bolsa lleva %d consumidos, quiere %d: al vaciarla no se borró la vuelta anterior",
			len(consumidos), total-len(lista))
	}
}

// LA BOLSA ES DE CADA EMPRESA, Y SE PRUEBA BAJO EL ROL DE LA APP.
//
// Dos cosas que el owner no puede ver: que el `grant` de la tabla nueva exista —el de 0024 fue
// puntual y no hay default privileges, así que sin el suyo el primer pedido en producción devuelve
// 42501— y que RLS aísle. Un negocio que gasta la bolsa no puede dejar al de al lado sin nombres.
func TestLaBolsaDeUnaEmpresaNoTocaLaDeLaOtra(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	otra := makeCompany(t, st, "bolsa-vecina")
	appSt := appRoleStore(t)

	// La empresa 1 gasta tres nombres.
	if err := appSt.WithTenant(ctx, defaultCompanyID, func(q *db.Queries) error {
		for _, n := range []string{"Persa", "Bombay", "Korat"} {
			if err := q.MarcarFolioConsumido(ctx, db.MarcarFolioConsumidoParams{
				Scheme: db.FolioSchemeRazas, Name: n,
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("marcar consumidos (¿falta el grant de folio_consumido?): %v", err)
	}

	verBolsa := func(empresa int64) []string {
		t.Helper()
		var out []string
		if err := appSt.WithTenant(ctx, empresa, func(q *db.Queries) error {
			var err error
			out, err = q.FolioNamesConsumidos(ctx, db.FolioSchemeRazas)
			return err
		}); err != nil {
			t.Fatalf("leer la bolsa de %d: %v", empresa, err)
		}
		return out
	}

	if n := len(verBolsa(defaultCompanyID)); n != 3 {
		t.Errorf("la empresa que gastó ve %d consumidos, quiere 3", n)
	}
	if n := len(verBolsa(otra)); n != 0 {
		t.Errorf("la empresa vecina ve %d consumidos de la otra: fuga entre empresas", n)
	}

	// Y vaciar la bolsa de una NO vacía la de la otra.
	if err := appSt.WithTenant(ctx, otra, func(q *db.Queries) error {
		if err := q.MarcarFolioConsumido(ctx, db.MarcarFolioConsumidoParams{
			Scheme: db.FolioSchemeRazas, Name: "Siamés",
		}); err != nil {
			return err
		}
		return q.VaciarBolsaDeFolios(ctx, db.FolioSchemeRazas)
	}); err != nil {
		t.Fatalf("vaciar la bolsa de la vecina: %v", err)
	}
	if n := len(verBolsa(defaultCompanyID)); n != 3 {
		t.Errorf("tras vaciar la bolsa de la vecina, la primera ve %d consumidos: el delete cruzó empresas", n)
	}
}

// CAMBIAR DE ESQUEMA NO PIERDE LA VUELTA DEL OTRO.
//
// Las dos bolsas son independientes a propósito: un negocio que prueba animales una semana y vuelve
// a razas retoma donde iba. Con una sola bolsa compartida, volver le repetiría nombres que ya cantó.
func TestLasDosBolsasSonIndependientes(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	sembrarConsumidos(t, st, db.FolioSchemeRazas, "Persa", "Bombay")
	sembrarConsumidos(t, st, db.FolioSchemeAnimales, "Tigre")

	if err := st.Q.VaciarBolsaDeFolios(ctx, db.FolioSchemeAnimales); err != nil {
		t.Fatalf("vaciar animales: %v", err)
	}
	razas, err := st.Q.FolioNamesConsumidos(ctx, db.FolioSchemeRazas)
	if err != nil {
		t.Fatalf("FolioNamesConsumidos: %v", err)
	}
	if len(razas) != 2 {
		t.Errorf("la bolsa de razas quedó con %d consumidos, quiere 2: vaciar un esquema se llevó el otro", len(razas))
	}
}

func sembrarConsumidos(t *testing.T, st *store.Store, scheme db.FolioScheme, nombres ...string) {
	t.Helper()
	for _, n := range nombres {
		if err := st.Q.MarcarFolioConsumido(context.Background(), db.MarcarFolioConsumidoParams{
			Scheme: scheme, Name: n,
		}); err != nil {
			t.Fatalf("marcar %q: %v", n, err)
		}
	}
}

func contieneNombre(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
