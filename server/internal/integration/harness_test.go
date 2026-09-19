//go:build integration

// Package integration runs end-to-end tests against a REAL Postgres — the flows that unit
// tests can't reach because the store is a concrete *db.Queries (no mockeable interface):
// rotación/reuso de refresh, la tx de creación de orden + depleción, y el reembolso.
//
// Correr: TEST_DATABASE_URL="postgres://…/gatobobah_test?sslmode=disable" go test -tags=integration ./internal/integration/...
// Sin la env se omiten (Skip). Cada test estrena una base propia, clonada de una plantilla ya
// migrada (ver sembrarPlantilla).
package integration

import (
	"context"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Reloj fijo: fechas de negocio deterministas para asertar sobre reportes por día.
var fixedNow = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

func clock() time.Time { return fixedNow }

// defaultCompanyID: la empresa 'gatobobah' que siembra la migración 0022 (id=1 en BD limpia).
const defaultCompanyID = 1

// appRolePassword: password de prueba para gatobobah_app, fijado tras migrar (en prod lo hace
// el bootstrap desde APP_DB_PASSWORD).
const appRolePassword = "test_app_pw"

// nombreDePlantilla y prefijoDeBase comparten raíz a propósito: si una prueba muere sin limpiar,
// un `psql -l` dice de dónde salió la base huérfana.
const (
	nombreDePlantilla = "egb_plantilla"
	prefijoDeBase     = "egb_prueba_"
)

var (
	// El pool de administración vive lo que vive el binario de pruebas: crear y soltar bases exige
	// estar conectado a OTRA base, y abrir esa conexión por prueba costaría más que el clon.
	adminUna  sync.Once
	adminPool *store.Store

	plantillaUna sync.Once

	basesMu        sync.Mutex
	basesPorPrueba = map[string]string{}
	contadorDeBase atomic.Int64
)

// baseDeDatosURL es la base MADRE del entorno: de ella cuelgan la plantilla y las bases de cada
// prueba. Nadie prueba contra ella.
func baseDeDatosURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("TEST_DATABASE_URL")
	if u == "" {
		t.Skip("TEST_DATABASE_URL no definido; omitiendo tests de integración")
	}
	return u
}

// testURL devuelve la base de la prueba EN CURSO, no la madre. Los tests que abren su propia
// conexión —otro rol, otra goroutine— la usan para caer en la MISMA base que sembró el harness;
// devolverles la madre los pondría a probar un esquema que nadie preparó.
func testURL(t *testing.T) string {
	t.Helper()
	madre := baseDeDatosURL(t)
	basesMu.Lock()
	defer basesMu.Unlock()
	if u, ok := basesPorPrueba[pruebaRaiz(t.Name())]; ok {
		return u
	}
	return madre
}

// pruebaRaiz recorta el nombre de una subprueba ("TestX/caso") a su raíz ("TestX"). Una subprueba
// hereda la base que abrió su padre, que es justo el patrón con el que varias comparten esquema.
func pruebaRaiz(nombre string) string {
	if i := strings.IndexByte(nombre, '/'); i >= 0 {
		return nombre[:i]
	}
	return nombre
}

// conBase reescribe la base de datos de una URL conservando credenciales y parámetros.
func conBase(rawURL, nombre string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.Path = "/" + nombre
	return u.String()
}

func admin(t *testing.T) *store.Store {
	t.Helper()
	madre := baseDeDatosURL(t)
	adminUna.Do(func() {
		st, err := store.New(context.Background(), madre)
		if err != nil {
			return
		}
		adminPool = st
	})
	if adminPool == nil {
		t.Fatalf("no se pudo abrir el pool de administración contra %s", madre)
	}
	return adminPool
}

// sembrarPlantilla migra UNA vez una base que luego se clona por prueba.
//
// Correr las migraciones cuesta ~770 ms y `create database … template` ~30: con las ~364 llamadas
// a newTestStore que tiene la suite, la diferencia son minutos de CI, no milisegundos. El
// aislamiento NO se afloja — cada prueba sigue estrenando una base virgen; lo que cambia es de
// dónde sale el esquema.
//
// Se rehace en cada corrida del binario, no se reusa la de la corrida anterior: una plantilla
// vieja serviría un esquema que ya no es el del repositorio, y eso no falla — pasa en verde.
func sembrarPlantilla(t *testing.T) {
	t.Helper()
	adm := admin(t)
	ctx := context.Background()
	var err error
	plantillaUna.Do(func() {
		// `with (force)` corta conexiones de una corrida anterior que murió a media prueba.
		if _, e := adm.Pool.Exec(ctx, "drop database if exists "+nombreDePlantilla+" with (force)"); e != nil {
			err = e
			return
		}
		if _, e := adm.Pool.Exec(ctx, "create database "+nombreDePlantilla); e != nil {
			err = e
			return
		}
		tpl, e := store.New(ctx, conBase(baseDeDatosURL(t), nombreDePlantilla))
		if e != nil {
			err = e
			return
		}
		// El pool se cierra ANTES de que nadie clone: `create database … template` se niega si
		// queda una sola sesión abierta contra la plantilla.
		defer tpl.Close()
		if e := store.Migrate(ctx, tpl.Pool); e != nil {
			err = e
			return
		}
		// El rol es de CLUSTER, no de la base: se le fija el password una sola vez y lo heredan
		// todos los clones. La migración 0024 lo crea sin password, como en producción.
		if _, e := adm.Pool.Exec(ctx, "alter role gatobobah_app with login password '"+appRolePassword+"'"); e != nil {
			err = e
			return
		}
	})
	if err != nil {
		t.Fatalf("sembrar la plantilla: %v", err)
	}
}

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	sembrarPlantilla(t)
	adm := admin(t)

	nombre := prefijoDeBase + itoa(int(contadorDeBase.Add(1)))
	if _, err := adm.Pool.Exec(ctx, "drop database if exists "+nombre+" with (force)"); err != nil {
		t.Fatalf("soltar la base previa: %v", err)
	}
	if _, err := adm.Pool.Exec(ctx, "create database "+nombre+" template "+nombreDePlantilla); err != nil {
		t.Fatalf("clonar la plantilla: %v", err)
	}
	// Se registra ANTES del cierre del pool: los cleanups corren en orden inverso, así que soltar
	// la base queda de último, cuando ya nadie está conectado a ella.
	t.Cleanup(func() {
		basesMu.Lock()
		delete(basesPorPrueba, pruebaRaiz(t.Name()))
		basesMu.Unlock()
		_, _ = adm.Pool.Exec(context.Background(), "drop database if exists "+nombre+" with (force)")
	})

	// Ni el `grant connect` ni el ajuste del GUC viajan con el clon —Postgres no copia los
	// privilegios ni los settings de la base plantilla—, así que van por clon.
	for _, stmt := range []string{
		"grant connect on database " + nombre + " to gatobobah_app",
		// GUC de tenant por defecto a nivel BD: las conexiones del OWNER (que salta RLS)
		// auto-sellan company_id=1 en sus inserts sin fijar el GUC en cada test. Aplica a
		// conexiones NUEVAS → va antes de abrir el pool.
		"alter database " + nombre + " set app.company_id = '" + itoa(defaultCompanyID) + "'",
	} {
		if _, err := adm.Pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparar la base de la prueba (%q): %v", stmt, err)
		}
	}

	dbURL := conBase(baseDeDatosURL(t), nombre)
	basesMu.Lock()
	basesPorPrueba[pruebaRaiz(t.Name())] = dbURL
	basesMu.Unlock()

	st, err := store.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

// appRoleStore devuelve un store conectado como gatobobah_app (no-superusuario) → RLS SÍ aplica.
// Úsalo para verificar el aislamiento real de tenant a través del store/servicios.
func appRoleStore(t *testing.T) *store.Store {
	t.Helper()
	u, _ := url.Parse(testURL(t))
	u.User = url.UserPassword("gatobobah_app", appRolePassword)
	st, err := store.New(context.Background(), u.String())
	if err != nil {
		t.Fatalf("app-role store.New: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// --- fixtures mínimos vía SQL crudo (owner: salta RLS). company_id explícito para poder sembrar
// en cualquier empresa (los inserts del owner sin company_id caen en la empresa 1 por el GUC). ---

func makeCompany(t *testing.T, st *store.Store, slug string) int64 {
	t.Helper()
	var id int64
	if err := st.Pool.QueryRow(context.Background(),
		`insert into companies (slug, name) values ($1, $2) returning id`, slug, "Test "+slug).Scan(&id); err != nil {
		t.Fatalf("makeCompany(%s): %v", slug, err)
	}
	// Espeja a provisionCompany: una empresa sin métodos de pago no puede cobrar, así que un test
	// que la creara pelada estaría probando un mundo que el sistema no produce.
	if err := st.Q.SeedBasePaymentMethods(context.Background(), id); err != nil {
		t.Fatalf("sembrar métodos de %s: %v", slug, err)
	}
	// Por WithTenant: el seed toma company_id del GUC, no de un parámetro.
	if err := st.WithTenant(context.Background(), id, func(q *db.Queries) error {
		return q.SeedDeliveryPlatforms(context.Background())
	}); err != nil {
		t.Fatalf("sembrar plataformas de %s: %v", slug, err)
	}
	return id
}

func makeUser(t *testing.T, st *store.Store, username, role string) int64 {
	return makeUserIn(t, st, defaultCompanyID, username, role)
}

func makeUserIn(t *testing.T, st *store.Store, companyID int64, username, role string) int64 {
	t.Helper()
	var id int64
	err := st.Pool.QueryRow(context.Background(),
		`insert into users (company_id, name, username, role) values ($1, $2, $3, $4::user_role) returning id`,
		companyID, "Test "+username, username, role).Scan(&id)
	if err != nil {
		t.Fatalf("makeUser(%s): %v", username, err)
	}
	return id
}

func makeProduct(t *testing.T, st *store.Store, name string, price decimal.Decimal, trackStock bool) int64 {
	t.Helper()
	ctx := context.Background()
	var catID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into categories (name) values ($1) returning id`, "cat-"+name).Scan(&catID); err != nil {
		t.Fatalf("makeCategory(%s): %v", name, err)
	}
	var id int64
	if err := st.Pool.QueryRow(ctx,
		`insert into products (name, category_id, price, track_stock) values ($1, $2, $3, $4) returning id`,
		name, catID, price, trackStock).Scan(&id); err != nil {
		t.Fatalf("makeProduct(%s): %v", name, err)
	}
	return id
}

func countOrderMovements(t *testing.T, st *store.Store, orderID int64) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from stock_movements where order_id = $1`, orderID).Scan(&n); err != nil {
		t.Fatalf("countOrderMovements: %v", err)
	}
	return n
}

// abrirCajaPrincipal deja la caja principal con turno abierto. Desde que cobrar exige caja abierta
// (domain.ErrNoOpenRegister) es precondición de cualquier test que cree una venta, así que vive
// aquí y no copiada en cada archivo.
func abrirCajaPrincipal(t *testing.T, st *store.Store, por int64) int64 {
	t.Helper()
	ctx := context.Background()
	var regID int64
	if err := st.Pool.QueryRow(ctx,
		`select id from cash_registers where is_primary and is_active limit 1`).Scan(&regID); err != nil {
		t.Fatalf("caja principal: %v", err)
	}
	var sessID int64
	// La fecha del turno sale del RELOJ DE LOS TESTS, no de current_date: la venta se archiva con
	// ese mismo reloj, y un turno abierto en la fecha real dejaría al arqueo hablando de un día y a
	// las ventas de otro.
	if err := st.Pool.QueryRow(ctx,
		`insert into register_sessions (business_date, opening_cash, opened_by, register_id)
		 values ($3, 0, $1, $2) returning id`, por, regID, fixedNow).Scan(&sessID); err != nil {
		t.Fatalf("abrir caja principal: %v", err)
	}
	return sessID
}

// platformID resuelve una plataforma de reparto DENTRO de una empresa. El filtro por empresa no es
// decorativo: delivery_platforms es per-tenant y cada empresa tiene su propia "Uber Eats", así que
// buscar solo por nombre devolvería la del tenant equivocado sin dar error.
func platformID(t *testing.T, st *store.Store, companyID int64, name string) int16 {
	t.Helper()
	var id int16
	if err := st.Pool.QueryRow(context.Background(),
		`select id from delivery_platforms where company_id = $1 and name = $2`,
		companyID, name).Scan(&id); err != nil {
		t.Fatalf("platformID(%d, %s): %v", companyID, name, err)
	}
	return id
}

// optionID devuelve una opción de modificador cualquiera de la empresa, creando el grupo si hace
// falta. Sirve para probar los precios de plataforma de los extras sin montar un menú completo.
func optionID(t *testing.T, st *store.Store, companyID int64) int64 {
	t.Helper()
	ctx := context.Background()
	var groupID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into modifier_groups (company_id, name) values ($1, 'Extras de prueba') returning id`,
		companyID).Scan(&groupID); err != nil {
		t.Fatalf("grupo de modificadores: %v", err)
	}
	var id int64
	if err := st.Pool.QueryRow(ctx,
		`insert into modifier_options (company_id, group_id, name, price_delta) values ($1, $2, 'Extra', 20) returning id`,
		companyID, groupID).Scan(&id); err != nil {
		t.Fatalf("opción de modificador: %v", err)
	}
	return id
}

// entregarPendientes marca como entregados todos los pedidos sin terminar de la empresa.
//
// Existe porque cerrar la caja YA NO admite pedientes (domain.ErrOpenOrders): un pedido abierto es
// comida que va a salir y dinero sin decidir, y dejarlo colgado de un arqueo ya firmado hace que
// ese corte no pueda volver a cuadrar. Los tests que cierran un turno tienen que terminar sus
// ventas primero, igual que el operador real.
func entregarPendientes(t *testing.T, st *store.Store) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(),
		`update orders set status = 'entregada', completed_at = now()
		  where status in ('abierta', 'lista')`); err != nil {
		t.Fatalf("entregar pendientes: %v", err)
	}
}

// crearYCobrar confirma el pedido y luego lo cobra, que es el flujo real desde que confirmar es
// obligatorio (feature 005).
//
// Antes esto era UNA llamada con `Payments`, y ese camino ya no existe: era el atajo por el que se
// cobraba sin que cocina se enterara, y por ser el corto era el que se usaba. Los tests lo usaban
// para armar escenarios, así que el helper conserva la forma de la llamada y hace los dos pasos.
//
// Devuelve error para que los tests que comprueban un rechazo sigan escribiéndose igual.
func crearYCobrar(t *testing.T, ctx context.Context, svc *app.OrdersService, cmd app.CreateOrderCmd) (*app.OrderView, error) {
	t.Helper()
	pagos := cmd.Payments
	cmd.Payments = nil

	ord, err := svc.Create(ctx, cmd)
	if err != nil || len(pagos) == 0 {
		return ord, err
	}
	for _, p := range pagos {
		if _, err := svc.Charge(ctx, app.ChargeCmd{
			OrderID: ord.ID, MethodID: p.MethodID, Amount: p.Amount, Tip: p.Tip,
			Reference: p.Reference, ActorID: cmd.OpenedBy,
		}); err != nil {
			return nil, err
		}
	}
	// Se relee: tras el cobro cambian el estado y lo pagado, y los tests miran eso.
	return svc.Detail(ctx, ord.ID)
}
