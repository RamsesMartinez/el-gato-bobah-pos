//go:build integration

package integration

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA MIGRACIÓN DE LA CONSOLA DE PLATAFORMA (spec 016).
//
// Estos casos NO pueden vivir en un unitario y esa es la razón de que estén aquí: la API de
// desarrollo se conecta como DUEÑO de la base, y para el dueño ni RLS ni los grants existen. Un
// test de aislamiento corrido como owner pasa siempre, aunque el aislamiento esté roto.
//
// Lo que se prueba es que la separación esté impuesta por **Postgres**, no por un `if` en Go.

// platformRolePassword: igual que `appRolePassword`, la contraseña de prueba del rol de plataforma.
// En producción la fija el bootstrap desde PLATFORM_DB_PASSWORD.
const platformRolePassword = "test_platform_pw"

// platformRoleStore abre una conexión CON EL ROL DE PLATAFORMA.
//
// Sin esto no hay prueba posible: el pool del harness es del owner, que salta todo.
func platformRoleStore(t *testing.T) *store.Store {
	t.Helper()
	u, _ := url.Parse(testURL(t))
	u.User = url.UserPassword("gatobobah_platform", platformRolePassword)
	st, err := store.New(context.Background(), u.String())
	if err != nil {
		t.Fatalf("abrir conexión con el rol de plataforma: %v", err)
	}
	t.Cleanup(st.Close)
	return st
}

// prepararRolDePlataforma le pone contraseña y acceso al rol, como el harness ya hace con el de la
// app. La migración crea el rol; la contraseña es cosa del despliegue.
func prepararRolDePlataforma(t *testing.T, st *store.Store) {
	t.Helper()
	ctx := context.Background()
	u, _ := url.Parse(testURL(t))
	dbName := u.Path[1:]
	for _, stmt := range []string{
		"alter role gatobobah_platform with login password '" + platformRolePassword + "'",
		"grant connect on database " + dbName + " to gatobobah_platform",
	} {
		if _, err := st.Pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparar el rol de plataforma (%q): %v", stmt, err)
		}
	}
}

// EL OPERADOR DE PLATAFORMA NO PERTENECE A NINGUNA EMPRESA (FR-002).
//
// Es la decisión que hace que FR-001 se cumpla por construcción: el login del negocio consulta
// `users`, y si el operador no está ahí no puede encontrarlo ni con el mismo nombre. Una columna
// `company_id` en esta tabla sería la señal de que alguien la metió al modelo de tenant.
func TestElOperadorDePlataformaNoTieneEmpresa(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	var existe bool
	if err := st.Pool.QueryRow(ctx,
		`select exists (select 1 from information_schema.tables where table_name = 'platform_operators')`,
	).Scan(&existe); err != nil {
		t.Fatalf("consultar la tabla: %v", err)
	}
	if !existe {
		t.Fatal("no existe platform_operators: la consola no tiene dónde vivir")
	}

	var conEmpresa bool
	if err := st.Pool.QueryRow(ctx,
		`select exists (select 1 from information_schema.columns
		                where table_name = 'platform_operators' and column_name = 'company_id')`,
	).Scan(&conEmpresa); err != nil {
		t.Fatalf("consultar las columnas: %v", err)
	}
	if conEmpresa {
		t.Fatal("platform_operators tiene company_id: el operador no pertenece a ninguna empresa, y meterlo al modelo de tenant es justo lo que esta feature evita")
	}

	// Y nace vacía: el primer operador se crea a propósito, no lo siembra una migración con una
	// contraseña conocida.
	var cuantos int
	if err := st.Pool.QueryRow(ctx, `select count(*) from platform_operators`).Scan(&cuantos); err != nil {
		t.Fatalf("contar operadores: %v", err)
	}
	if cuantos != 0 {
		t.Fatalf("la migración sembró %d operadores: una credencial que nace con el esquema es una credencial pública", cuantos)
	}
}

// EL ROL DE PLATAFORMA NO ES SUPERUSUARIO NI SALTA RLS.
//
// Si lo fuera, las otras dos barreras no existirían: un superusuario lee todo sin que ningún grant
// ni ninguna política lo detengan.
func TestElRolDePlataformaNoEsSuperusuario(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	var existe, super, saltaRLS bool
	if err := st.Pool.QueryRow(ctx,
		`select true, rolsuper, rolbypassrls from pg_roles where rolname = 'gatobobah_platform'`,
	).Scan(&existe, &super, &saltaRLS); err != nil {
		t.Fatalf("el rol gatobobah_platform no existe: %v", err)
	}
	if super {
		t.Fatal("gatobobah_platform es superusuario: las tres barreras dejan de existir de golpe")
	}
	if saltaRLS {
		t.Fatal("gatobobah_platform tiene bypassrls: leería cualquier fila de cualquier empresa en cuanto exista un grant")
	}
}

// LA SEPARACIÓN CORRE EN LAS DOS DIRECCIONES.
//
// Que la consola no lea el negocio es la mitad; la otra es que el negocio no lea la consola. Una
// credencial de operador leída desde el rol de la aplicación sería la forma más tonta de perderlo
// todo.
func TestElRolDeLaAppNoPuedeLeerLosOperadores(t *testing.T) {
	owner := newTestStore(t)
	app := appRoleStore(t)

	_ = owner // el harness ya migró con el owner; lo que importa es lo que ve el rol de la app.

	var n int
	err := app.Pool.QueryRow(context.Background(), `select count(*) from platform_operators`).Scan(&n)
	if err == nil {
		t.Fatalf("el rol de la aplicación leyó platform_operators (%d filas): el negocio no debe alcanzar las credenciales de la plataforma", n)
	}
	if !esPermisoDenegado(err) {
		t.Fatalf("se esperaba permiso denegado (42501) y llegó otro error: %v", err)
	}
}

// esPermisoDenegado: el 42501 de Postgres, que es la respuesta que prueba que la barrera está en la
// base y no en el código. Un error distinto —una tabla que no existe, una conexión caída— NO
// cuenta: pasaría el test sin que el permiso esté puesto.
func esPermisoDenegado(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "42501") || strings.Contains(msg, "permission denied")
}

// LA CONSOLA VE TODAS LAS EMPRESAS, Y EL GRANT SOLO NO ALCANZA (FR-007).
//
// El caso que este test existe para atrapar: darle `select on companies` al rol de plataforma y
// dar por hecho que ya ve el catálogo de clientes. No lo ve. `companies` lleva la política
// `company_self` y RLS le aplica a TODO rol que no sea superusuario, así que sin una política
// propia la consola vería UNA empresa de dos —la del GUC de la conexión— y la pantalla se vería
// perfectamente bien mintiendo.
//
// La otra mitad importa igual: abrir esa política NO puede aflojar la del negocio. Las políticas
// permisivas se suman, así que se verifica en la misma corrida que el rol de la aplicación sigue
// viendo solo la suya.
func TestLaConsolaVeTodasLasEmpresasYElNegocioNo(t *testing.T) {
	owner := newTestStore(t)
	ctx := context.Background()

	// Con una sola empresa este test no prueba nada: todo "ve todas" sería cierto por accidente.
	makeCompany(t, owner, "segunda-empresa")

	var total int
	if err := owner.Pool.QueryRow(ctx, `select count(*) from companies`).Scan(&total); err != nil {
		t.Fatalf("contar empresas como dueño: %v", err)
	}
	if total < 2 {
		t.Fatalf("el escenario necesita al menos 2 empresas y hay %d: con una sola, un aislamiento roto pasa verde", total)
	}

	prepararRolDePlataforma(t, owner)
	plataforma := platformRoleStore(t)

	var vistasPorLaConsola int
	if err := plataforma.Pool.QueryRow(ctx, `select count(*) from companies`).Scan(&vistasPorLaConsola); err != nil {
		t.Fatalf("la consola no pudo leer companies: %v", err)
	}
	if vistasPorLaConsola != total {
		t.Fatalf("la consola ve %d empresas de %d: falta la política de RLS para gatobobah_platform, y la lista de clientes saldría incompleta sin avisar", vistasPorLaConsola, total)
	}

	// Y el negocio sigue encerrado en la suya.
	app := appRoleStore(t)
	var vistasPorElNegocio int
	if err := app.Pool.QueryRow(ctx, `select count(*) from companies`).Scan(&vistasPorElNegocio); err != nil {
		t.Fatalf("el rol de la aplicación no pudo leer companies: %v", err)
	}
	if vistasPorElNegocio != 1 {
		t.Fatalf("el rol de la aplicación ve %d empresas: abrir la política de la plataforma aflojó el aislamiento del negocio", vistasPorElNegocio)
	}
}

// REVERTIR CORTA EL ACCESO, NO BORRA EL ROL — Y VOLVER A APLICAR DEJA LA CONSOLA VIVA.
//
// Dos cosas que se prueban juntas porque el defecto vive entre ellas:
//
//  1. El `Down` NO hace `drop role`. En Postgres los roles son del SERVIDOR: si el rol tiene
//     permisos en otra base del mismo clúster, `drop role` truena y `drop owned by` no los alcanza.
//     Un `Down` así pasa en CI (una base) y revienta en la máquina de quien programa (cinco), que
//     es la peor forma de fallar: parece entorno roto, no migración rota.
//
//  2. Por eso mismo, `up → down → up` tiene que dejar el rol como estaba. El `Down` le quita el
//     login; si el `Up` solo crea el rol "si no existe", la segunda vuelta lo deja SIN login y la
//     API arranca contra un rol que no se puede conectar — con un error que no menciona la
//     migración por ningún lado.
func TestRevertirLaConsolaNoBorraElRolYVolverAAplicarlaLaRevive(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	prepararRolDePlataforma(t, st)

	const antesDeLaConsola = 67
	migrarAbajoHasta(t, st.Pool, antesDeLaConsola)

	var sigueExistiendo, puedeEntrar bool
	if err := st.Pool.QueryRow(ctx,
		`select true, rolcanlogin from pg_roles where rolname = 'gatobobah_platform'`,
	).Scan(&sigueExistiendo, &puedeEntrar); err != nil {
		t.Fatalf("el Down borró el rol del clúster: %v — revertir tiene que cortar el acceso, no depender de cuántas bases tenga este Postgres", err)
	}
	if puedeEntrar {
		t.Fatal("el rol de plataforma sigue pudiendo conectarse después de revertir: el Down no cortó el acceso, que es lo único que de verdad importa al revertir")
	}

	var tablaViva bool
	if err := st.Pool.QueryRow(ctx,
		`select exists (select 1 from information_schema.tables where table_name = 'platform_operators')`,
	).Scan(&tablaViva); err != nil {
		t.Fatalf("consultar la tabla tras revertir: %v", err)
	}
	if tablaViva {
		t.Fatal("platform_operators sobrevivió al Down: revertir dejó el esquema a medias")
	}

	// Y de vuelta arriba. Sin volver a tocar el rol a mano: si hiciera falta un `alter role` para
	// que la consola funcione otra vez, el Up estaría incompleto.
	migrarArriba(t, st.Pool)

	if err := st.Pool.QueryRow(ctx,
		`select rolcanlogin from pg_roles where rolname = 'gatobobah_platform'`,
	).Scan(&puedeEntrar); err != nil {
		t.Fatalf("leer el rol tras reaplicar: %v", err)
	}
	if !puedeEntrar {
		t.Fatal("tras reaplicar la migración el rol quedó sin login: el `create role ... if not exists` no repara lo que el Down le quitó, y la API arrancaría contra un rol que no se puede conectar")
	}

	plataforma := platformRoleStore(t)
	var empresas int
	if err := plataforma.Pool.QueryRow(ctx, `select count(*) from companies`).Scan(&empresas); err != nil {
		t.Fatalf("la consola no volvió a leer companies tras reaplicar: %v — el Up no repuso los grants o la política", err)
	}
}

// EL ARRANQUE COMPRUEBA QUE LA CONSOLA SIRVE CON SU ROL, NO CON UNO PRESTADO.
//
// Escenario concreto que cierra: alguien copia `PLATFORM_DATABASE_URL` de `DATABASE_URL` o de
// `APP_DATABASE_URL` al configurar el despliegue. La consola funcionaría —mejor que nunca, de
// hecho— leyendo pedidos, usuarios y pagos de todas las empresas, y nada fallaría. La única señal
// sería una auditoría a mano.
//
// Por eso la comprobación es FUNCIONAL y no solo de banderas: pregunta si el rol puede leer
// `orders`, que es justo lo que ningún rol de plataforma debe poder.
func TestElArranqueRechazaUnaConsolaConRolPrestado(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	prepararRolDePlataforma(t, st)

	if err := store.AssertPlatformGrants(ctx, platformRoleStore(t)); err != nil {
		t.Fatalf("el rol correcto no pasó el chequeo de arranque: %v", err)
	}

	// El owner: superusuario, salta grants y RLS. Es el copy-paste de DATABASE_URL.
	if err := store.AssertPlatformGrants(ctx, st); err == nil {
		t.Fatal("el chequeo aceptó al dueño de la base: la consola serviría con bypass total y nadie lo notaría")
	}

	// El rol del negocio: no es superusuario, así que una comprobación que solo mirara banderas lo
	// dejaría pasar — y puede leer `orders`, que es exactamente lo que no debe poder.
	if err := store.AssertPlatformGrants(ctx, appRoleStore(t)); err == nil {
		t.Fatal("el chequeo aceptó al rol del negocio: puede leer las tablas de operación, que es lo único que la consola no debe alcanzar")
	}
}
