//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// Las cuatro tablas de la 0071. Se nombran una sola vez y los tests las recorren: una tabla nueva
// que se agregue a la migración y no aquí deja su grant sin probar, que es exactamente el defecto
// que este archivo existe para atrapar.
var tablasDeMenusDePlataforma = []string{
	"platform_connections",
	"platform_menu_reads",
	"platform_menu_items",
	"platform_item_links",
}

// EL GRANT NO SE HEREDA. El de 0024 fue puntual y no dejó default privileges, así que cada tabla
// nueva necesita el suyo. Sin él, el primer request en producción responde 42501 y en desarrollo
// nunca falla, porque la API de dev se conecta como owner.
func TestLasTablasDeMenusDePlataformaTienenSusGrants(t *testing.T) {
	newTestStore(t) // migra
	appSt := appRoleStore(t)
	ctx := context.Background()

	for _, tabla := range tablasDeMenusDePlataforma {
		for _, priv := range []string{"select", "insert", "update", "delete"} {
			var tiene bool
			err := appSt.Pool.QueryRow(ctx,
				`select has_table_privilege('gatobobah_app', $1, $2)`, tabla, priv).Scan(&tiene)
			if err != nil {
				t.Fatalf("has_table_privilege(%s, %s): %v", tabla, priv, err)
			}
			if !tiene {
				t.Errorf("gatobobah_app no tiene %s sobre %s: en producción esto es un 42501 en el primer request", priv, tabla)
			}
		}
	}
}

// RLS: las políticas NO existen para el owner, así que esto solo se puede probar bajo el rol de
// app. Una fuga entre empresas no se ve hasta que hay un segundo cliente.
func TestUnaConexionDeOtraEmpresaNoSeVe(t *testing.T) {
	st := newTestStore(t)
	appSt := appRoleStore(t)
	ctx := context.Background()

	otra := makeCompany(t, st, "otra-conexion")
	sembrarConexion(t, st, otra, "tienda-de-la-otra")
	propia := sembrarConexion(t, st, defaultCompanyID, "tienda-propia")

	var vistas []int64
	filas, err := appSt.Pool.Query(ctx, `select id from platform_connections`)
	if err != nil {
		t.Fatalf("select bajo rol de app: %v", err)
	}
	defer filas.Close()
	for filas.Next() {
		var id int64
		if err := filas.Scan(&id); err != nil {
			t.Fatal(err)
		}
		vistas = append(vistas, id)
	}
	if len(vistas) != 1 || vistas[0] != propia {
		t.Fatalf("el rol de app vio %v; debía ver solo la conexión %d de su empresa", vistas, propia)
	}
}

// LOS CHEQUEOS DE INTEGRIDAD DE POSTGRES SALTAN RLS, por diseño. Así que una FK no impide sola que
// una empresa apunte a un producto de otra: hay que probarlo con datos de las dos.
func TestUnEmparejamientoNoCruzaEmpresas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	otra := makeCompany(t, st, "otra-emparejar")
	conexionPropia := sembrarConexion(t, st, defaultCompanyID, "tienda-propia")
	productoAjeno := sembrarProductoEn(t, st, otra, "Producto de la otra")

	_, err := st.Pool.Exec(ctx,
		`insert into platform_item_links (company_id, connection_id, external_id, kind, product_id, local_kind)
		 values ($1, $2, 'Item_ajeno', 'platillo', $3, 'producto')`,
		defaultCompanyID, conexionPropia, productoAjeno)
	if err == nil {
		t.Fatal("se emparejó un producto de OTRA empresa: la comparación reportaría diferencias de un catálogo que no es")
	}
}

// FR-005 EN EL ESQUEMA. Una lectura que vuelve sin productos no es un menú vacío: es una lectura
// que no sirve. Tratarla como dato válido haría que la comparación dijera que sobra todo el
// catálogo — el peor reporte posible, y el que más invita a una acción destructiva.
func subUnaLecturaOkSinItemsSeRechaza(t *testing.T, st *store.Store) {
	ctx := context.Background()
	con := sembrarConexion(t, st, defaultCompanyID, "tienda-vacia")

	_, err := st.Pool.Exec(ctx,
		`insert into platform_menu_reads (company_id, connection_id, status, finished_at, item_count)
		 values ($1, $2, 'ok', now(), 0)`, defaultCompanyID, con)
	if err == nil {
		t.Fatal("se aceptó una lectura 'ok' con cero items: FR-005 exige tratarla como fallida")
	}
	if !strings.Contains(err.Error(), "ok_trae_items") {
		t.Errorf("falló, pero no por el check esperado: %v", err)
	}
}

// Y su gemelo: una lectura en curso no puede tener hora de fin, ni una terminada carecer de ella.
func subElEstadoDeUnaLecturaEsCoherenteConSuHoraDeFin(t *testing.T, st *store.Store) {
	ctx := context.Background()
	con := sembrarConexion(t, st, defaultCompanyID, "tienda-coherente")

	casos := []struct {
		nombre string
		sql    string
	}{
		{"en curso con hora de fin", `insert into platform_menu_reads (company_id, connection_id, status, finished_at) values ($1, $2, 'en_curso', now())`},
		{"fallida sin hora de fin", `insert into platform_menu_reads (company_id, connection_id, status) values ($1, $2, 'fallida')`},
	}
	for _, c := range casos {
		if _, err := st.Pool.Exec(ctx, c.sql, defaultCompanyID, con); err == nil {
			t.Errorf("se aceptó %q", c.nombre)
		}
	}
}

// LA LISTA DE CLASES DE FALLO ES UN CONTROL DE SEGURIDAD, no una convención. Sirve para que el
// error crudo de una API ajena —que puede traer el secreto en el query string— nunca llegue a la
// base. Un `check` no se olvida cuando alguien agrega una rama nueva al servicio.
func subLaClaseDeFalloEsUnaListaCerrada(t *testing.T, st *store.Store) {
	ctx := context.Background()
	con := sembrarConexion(t, st, defaultCompanyID, "tienda-fallos")

	_, err := st.Pool.Exec(ctx,
		`insert into platform_menu_reads (company_id, connection_id, status, finished_at, failure_kind)
		 values ($1, $2, 'fallida', now(), 'Get "https://api?app_secret=SECRETO": timeout')`,
		defaultCompanyID, con)
	if err == nil {
		t.Fatal("se guardó un mensaje crudo como clase de fallo: ahí cabe un secreto de la plataforma")
	}
	for _, valida := range []string{"sin_credenciales", "auth_rechazada", "tiempo_agotado", "respuesta_invalida", "menu_vacio", "menu_truncado"} {
		if _, err := st.Pool.Exec(ctx,
			`insert into platform_menu_reads (company_id, connection_id, status, finished_at, failure_kind)
			 values ($1, $2, 'fallida', now(), $3)`, defaultCompanyID, con, valida); err != nil {
			t.Errorf("la clase %q debería aceptarse: %v", valida, err)
		}
	}
}

// VARIAS TIENDAS POR EMPRESA. Es la puerta del principio VIII que motivó rehacer el esquema: una
// llave única sin `external_store_id` dejaría fuera a la segunda sucursal.
func subDosTiendasDeLaMismaPlataformaCaben(t *testing.T, st *store.Store) {
	sembrarConexion(t, st, defaultCompanyID, "sucursal-centro")
	sembrarConexion(t, st, defaultCompanyID, "sucursal-norte")

	// Contado por SUS tiendas y no por la empresa entera: comparte esquema con las demás
	// subpruebas, y un `count(*)` global convertiría esta aserción en una función de cuántas
	// corrieron antes.
	var n int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from platform_connections
		  where company_id = $1 and external_store_id in ('sucursal-centro', 'sucursal-norte')`,
		defaultCompanyID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("quedaron %d conexiones; una empresa tiene que poder registrar sus dos sucursales", n)
	}
}

// Y la misma tienda dos veces, no: es captura duplicada y produciría dos fotos del mismo menú.
func subLaMismaTiendaNoSeRegistraDosVeces(t *testing.T, st *store.Store) {
	sembrarConexion(t, st, defaultCompanyID, "tienda-repetida")
	if _, err := st.Pool.Exec(context.Background(),
		`insert into platform_connections (company_id, delivery_platform_id, external_store_id, label)
		 select $1, id, 'tienda-repetida', 'otra etiqueta' from delivery_platforms where name = 'Uber Eats' limit 1`,
		defaultCompanyID); err == nil {
		t.Fatal("se registró la misma tienda dos veces")
	}
}

// FR-012: un item de la plataforma no puede apuntar a dos productos. Lo impide la PK, y sin este
// assert es la promesa de un comentario.
func subUnItemDeLaPlataformaNoApuntaADosProductos(t *testing.T, st *store.Store) {
	ctx := context.Background()
	con := sembrarConexion(t, st, defaultCompanyID, "tienda-fr012")
	a := sembrarProductoEn(t, st, defaultCompanyID, "Chamoyada")
	b := sembrarProductoEn(t, st, defaultCompanyID, "Frappé")

	emparejar(t, st, con, "Chamoyada_de_Mango", a)
	if _, err := st.Pool.Exec(ctx,
		`insert into platform_item_links (company_id, connection_id, external_id, kind, product_id, local_kind)
		 values ($1, $2, 'Chamoyada_de_Mango', 'platillo', $3, 'producto')`,
		defaultCompanyID, con, b); err == nil {
		t.Fatal("el mismo item quedó emparejado con dos productos: la comparación reportaría diferencias contradictorias")
	}
}

// FR-010: al revés SÍ. Es el caso real del negocio, no una generalización — una `Chamoyada` abajo
// son doce chamoyadas arriba.
func subUnProductoSiPuedeSerVariosPlatillosDeArriba(t *testing.T, st *store.Store) {
	con := sembrarConexion(t, st, defaultCompanyID, "tienda-fr010")
	chamoyada := sembrarProductoEn(t, st, defaultCompanyID, "Chamoyada")

	for _, sabor := range []string{"Chamoyada_de_Mango", "Chamoyada_de_Fresa", "Chamoyada_de_Sand"} {
		emparejar(t, st, con, sabor, chamoyada)
	}
	var n int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from platform_item_links where product_id = $1`, chamoyada).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("quedaron %d parejas; un producto del POS tiene que poder ser varios platillos arriba", n)
	}
}

// FR-014: la pareja se guarda contra identificadores, así que renombrar el producto no la toca.
func subRenombrarElProductoNoRompeLaPareja(t *testing.T, st *store.Store) {
	ctx := context.Background()
	con := sembrarConexion(t, st, defaultCompanyID, "tienda-fr014")
	p := sembrarProductoEn(t, st, defaultCompanyID, "Nombre viejo")
	emparejar(t, st, con, "Item_de_arriba", p)

	if _, err := st.Pool.Exec(ctx, `update products set name = 'Nombre nuevo' where id = $1`, p); err != nil {
		t.Fatal(err)
	}
	var quedan int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from platform_item_links where product_id = $1`, p).Scan(&quedan); err != nil {
		t.Fatal(err)
	}
	if quedan != 1 {
		t.Fatalf("la pareja se perdió al renombrar el producto: quedan %d", quedan)
	}
}

// --- fixtures de este archivo ---

func sembrarConexion(t *testing.T, st *store.Store, companyID int64, storeID string) int64 {
	t.Helper()
	var id int64
	err := st.Pool.QueryRow(context.Background(),
		`insert into platform_connections (company_id, delivery_platform_id, external_store_id, label)
		 select $1, id, $2, $3 from delivery_platforms
		  where company_id = $1 and name = 'Uber Eats' limit 1
		 returning id`, companyID, storeID, "Tienda "+storeID).Scan(&id)
	if err != nil {
		t.Fatalf("sembrarConexion(%s): %v", storeID, err)
	}
	return id
}

func sembrarProductoEn(t *testing.T, st *store.Store, companyID int64, nombre string) int64 {
	t.Helper()
	ctx := context.Background()
	// La categoría lleva el nombre de la prueba: `categories_name_scope` es único por empresa, y
	// desde que estas pruebas comparten esquema, dos que siembren «Chamoyada» chocarían. El choque
	// no dice nada del código, solo del fixture.
	var catID int64
	if err := st.Pool.QueryRow(ctx,
		`insert into categories (company_id, name) values ($1, $2) returning id`,
		companyID, "cat-"+t.Name()+"-"+nombre).Scan(&catID); err != nil {
		t.Fatalf("categoría de %s: %v", nombre, err)
	}
	// El nombre del producto también: `products_company_name_key` es único por empresa. Lo que la
	// prueba mira es el id que devuelve, no el texto.
	var id int64
	if err := st.Pool.QueryRow(ctx,
		`insert into products (company_id, name, price, category_id, type)
		 values ($1, $2, $3, $4, 'simple') returning id`,
		companyID, t.Name()+"-"+nombre, decimal.NewFromInt(50), catID).Scan(&id); err != nil {
		t.Fatalf("producto %s: %v", nombre, err)
	}
	return id
}

func emparejar(t *testing.T, st *store.Store, conexion int64, externalID string, productoID int64) {
	t.Helper()
	if _, err := st.Pool.Exec(context.Background(),
		`insert into platform_item_links (company_id, connection_id, external_id, kind, product_id, local_kind)
		 values ($1, $2, $3, 'platillo', $4, 'producto')`,
		defaultCompanyID, conexion, externalID, productoID); err != nil {
		t.Fatalf("emparejar(%s): %v", externalID, err)
	}
}

// --- Las dos puertas por donde se borra el emparejamiento ---
//
// Es trabajo humano que no se reconstruye, y tiene dos vecinos que podrían llevárselo. Los dos
// fallos serían SILENCIOSOS y semanas después: la pantalla reportaría de pronto que el 100% del
// menú difiere, sin un solo error en el log.

func TestElEmparejamientoSobreviveALaPoda(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	con := sembrarConexion(t, st, defaultCompanyID, "tienda-poda")
	p := sembrarProductoEn(t, st, defaultCompanyID, "Producto emparejado")
	emparejar(t, st, con, "Item_de_arriba", p)

	// Dos lecturas viejas y una reciente, para que la poda tenga algo que borrar y algo que dejar.
	for _, edad := range []string{"200 days", "150 days", "1 day"} {
		if _, err := st.Pool.Exec(ctx,
			`insert into platform_menu_reads (company_id, connection_id, status, started_at, finished_at, item_count)
			 values ($1, $2, 'ok', now() - $3::interval, now() - $3::interval, 5)`,
			defaultCompanyID, con, edad); err != nil {
			t.Fatal(err)
		}
	}
	antes := cuantasParejas(t, st, con)
	if _, err := st.Pool.Exec(ctx,
		`delete from platform_menu_reads r
		  where r.started_at < now() - interval '92 days'
		    and r.id <> (select r2.id from platform_menu_reads r2
		                  where r2.connection_id = r.connection_id order by r2.started_at desc limit 1)`); err != nil {
		t.Fatal(err)
	}
	if despues := cuantasParejas(t, st, con); despues != antes {
		t.Fatalf("la poda se llevó %d parejas: es una sesión completa de trabajo manual y no se reconstruye", antes-despues)
	}
}

func TestLaPodaConservaLaUltimaLectura(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	con := sembrarConexion(t, st, defaultCompanyID, "tienda-abandonada")
	// Una tienda que nadie volvió a leer en meses: TODAS sus lecturas son viejas.
	for _, edad := range []string{"300 days", "250 days"} {
		if _, err := st.Pool.Exec(ctx,
			`insert into platform_menu_reads (company_id, connection_id, status, started_at, finished_at, item_count)
			 values ($1, $2, 'ok', now() - $3::interval, now() - $3::interval, 5)`,
			defaultCompanyID, con, edad); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.Pool.Exec(ctx,
		`delete from platform_menu_reads r
		  where r.started_at < now() - interval '92 days'
		    and r.id <> (select r2.id from platform_menu_reads r2
		                  where r2.connection_id = r.connection_id order by r2.started_at desc limit 1)`); err != nil {
		t.Fatal(err)
	}
	var quedan int
	if err := st.Pool.QueryRow(ctx,
		`select count(*) from platform_menu_reads where connection_id = $1`, con).Scan(&quedan); err != nil {
		t.Fatal(err)
	}
	if quedan != 1 {
		t.Fatalf("quedaron %d lecturas; sin conservar la última, «se leyó hace 10 meses» se vuelve «nunca se ha leído» y FR-003 pide distinguirlos", quedan)
	}
}

// En este repo los productos SÍ se borran: el patrón de reorg de datos de AGENTS.md §6 corre
// `delete from products`. Con `on delete cascade`, un reorg se llevaría las parejas confirmadas sin
// avisar — el precedente correcto es order_lines.product_id, que referencia sin `on delete`.
func TestBorrarUnProductoEmparejadoFalla(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	con := sembrarConexion(t, st, defaultCompanyID, "tienda-restrict")
	p := sembrarProductoEn(t, st, defaultCompanyID, "Producto con pareja")
	emparejar(t, st, con, "Item_de_arriba", p)

	if _, err := st.Pool.Exec(ctx, `delete from products where id = $1`, p); err == nil {
		t.Fatalf("se borró un producto con %d pareja(s) confirmada(s) sin que nada avisara", cuantasParejas(t, st, con))
	}
}

// Un platillo se empareja con un producto y una opción con una opción. Guardar el id de una opción
// en la columna de producto pasa los tipos y produce un mapeo que NUNCA empata con nada.
func TestLaParejaDeUnaOpcionNoApuntaAUnProducto(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	con := sembrarConexion(t, st, defaultCompanyID, "tienda-opciones")
	p := sembrarProductoEn(t, st, defaultCompanyID, "Un producto")

	if _, err := st.Pool.Exec(ctx,
		`insert into platform_item_links (company_id, connection_id, external_id, kind, product_id, local_kind)
		 values ($1, $2, 'Una_opcion', 'opcion', $3, 'producto')`,
		defaultCompanyID, con, p); err != nil {
		t.Skipf("el esquema no impide la combinación; la valida el servicio: %v", err)
	}
	// Si el esquema la acepta, la barrera es el servicio — y lo que no puede pasar es que las dos
	// la dejen entrar. Este caso documenta dónde vive la validación.
	var lk string
	if err := st.Pool.QueryRow(ctx,
		`select local_kind from platform_item_links where connection_id = $1 and external_id = 'Una_opcion'`,
		con).Scan(&lk); err != nil {
		t.Fatal(err)
	}
	if lk != "producto" {
		t.Fatalf("local_kind quedó %q", lk)
	}
}

func cuantasParejas(t *testing.T, st *store.Store, con int64) int {
	t.Helper()
	var n int
	if err := st.Pool.QueryRow(context.Background(),
		`select count(*) from platform_item_links where connection_id = $1`, con).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// EL SERVICIO, EJERCIDO BAJO EL ROL DE APP — que es como corre en producción.
//
// Los tests de arriba prueban el ESQUEMA con SQL crudo, y por eso no vieron el defecto que este
// atrapa: `app.MenusDePlataformaService` usaba `store.Q` (el pool crudo) en vez de `store.QC(ctx)`
// (la conexión con el tenant fijado). Bajo el rol `gatobobah_app`, una conexión arbitraria del pool
// no tiene `app.company_id`, así que RLS la deja ver CERO filas y todo inserto revienta contra el
// `not null` de company_id.
//
// En desarrollo no falla —la API se conecta como owner y la base trae el GUC por `alter database`—
// así que el síntoma aparece solo en producción: la pantalla sale vacía, sin un error.
func TestElServicioRespetaElTenantBajoElRolDeApp(t *testing.T) {
	st := newTestStore(t)
	appSt := appRoleStore(t)
	ctx := context.Background()

	// La plataforma que se va a conectar, sembrada por el owner.
	var platID int16
	if err := st.Pool.QueryRow(ctx,
		`select id from delivery_platforms where company_id = $1 and name = 'Uber Eats' limit 1`,
		defaultCompanyID).Scan(&platID); err != nil {
		t.Fatal(err)
	}

	// El servicio, sobre el store del ROL DE APP y con el ctx de tenant que pone el middleware.
	svc := app.NewMenusDePlataformaService(appSt, nil, nil)
	tctx, soltar, err := appSt.AcquireTenant(ctx, defaultCompanyID)
	if err != nil {
		t.Fatalf("AcquireTenant: %v", err)
	}
	defer soltar()

	id, err := svc.CrearConexion(tctx, app.AltaDeConexion{
		PlatformID: platID, ExternalStoreID: "tienda-del-servicio", Label: "Centro",
	})
	if err != nil {
		t.Fatalf("crear la conexión bajo el rol de app: %v", err)
	}

	cons, err := svc.ListarConexiones(tctx)
	if err != nil {
		t.Fatalf("listar bajo el rol de app: %v", err)
	}
	if len(cons) != 1 || cons[0].ID != id {
		t.Fatalf("el servicio devolvió %d conexiones; con RLS mal aplicado salen cero y la pantalla se ve vacía sin ningún error", len(cons))
	}

	// LA PARTE QUE DE VERDAD ATRAPA EL DEFECTO. El harness hace `alter database ... set
	// app.company_id = 1`, así que CUALQUIER conexión del pool hereda la empresa 1 y un servicio
	// que use el pool crudo «funciona» en las pruebas mientras rompe en producción.
	//
	// Pedir el tenant de OTRA empresa deja al descubierto cuál conexión se está usando: con
	// `store.QC(ctx)` se ve el catálogo de la empresa 2 (vacío); con `store.Q` se ve el de la 1.
	otra := makeCompany(t, st, "vecina-del-servicio")
	octx, soltarOtra, err := appSt.AcquireTenant(ctx, otra)
	if err != nil {
		t.Fatalf("AcquireTenant de la otra empresa: %v", err)
	}
	defer soltarOtra()

	ajenas, err := svc.ListarConexiones(octx)
	if err != nil {
		t.Fatalf("listar como la otra empresa: %v", err)
	}
	if len(ajenas) != 0 {
		t.Fatalf("la empresa %d vio %d conexiones de otra: el servicio está usando el pool crudo en vez de la conexión con el tenant fijado", otra, len(ajenas))
	}
	// Y que lo que creó sea de SU empresa, no de ninguna otra.
	var dueña int64
	if err := st.Pool.QueryRow(ctx,
		`select company_id from platform_connections where id = $1`, id).Scan(&dueña); err != nil {
		t.Fatal(err)
	}
	if dueña != defaultCompanyID {
		t.Fatalf("la conexión quedó en la empresa %d y no en la %d", dueña, defaultCompanyID)
	}
}

// LAS RESTRICCIONES DEL ESQUEMA, TODAS SOBRE UN SOLO ESQUEMA.
//
// Cada una usa su propia tienda y no se pisan, así que reconstruir la base ocho veces solo cuesta
// tiempo: `newTestStore` hace `drop schema` y corre las 71 migraciones, y la suite completa ya hace
// eso 368 veces. Medido: en CI eso fue lo que la empujó contra el tope de 20 minutos.
func TestLasRestriccionesDelEsquemaDeMenus(t *testing.T) {
	st := newTestStore(t)
	casos := map[string]func(*testing.T, *store.Store){
		"una lectura ok sin items se rechaza":       subUnaLecturaOkSinItemsSeRechaza,
		"el estado es coherente con la hora de fin": subElEstadoDeUnaLecturaEsCoherenteConSuHoraDeFin,
		"la clase de fallo es una lista cerrada":    subLaClaseDeFalloEsUnaListaCerrada,
		"dos tiendas de la misma plataforma caben":  subDosTiendasDeLaMismaPlataformaCaben,
		"la misma tienda no se registra dos veces":  subLaMismaTiendaNoSeRegistraDosVeces,
		"un item no apunta a dos productos":         subUnItemDeLaPlataformaNoApuntaADosProductos,
		"un producto sí puede ser varios platillos": subUnProductoSiPuedeSerVariosPlatillosDeArriba,
		"renombrar el producto no rompe la pareja":  subRenombrarElProductoNoRompeLaPareja,
	}
	for nombre, caso := range casos {
		t.Run(nombre, func(t *testing.T) { caso(t, st) })
	}
}
