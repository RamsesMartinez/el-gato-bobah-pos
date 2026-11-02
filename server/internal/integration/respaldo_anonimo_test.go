//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// LO QUE BORRA scripts/respaldo-anonimo.sh SE COMPRUEBA CONTRA EL CATÁLOGO, NO CONTRA UNA LISTA.
//
// El anonimizador se quedó atrás dos veces sin que nada fallara: los pedidos de plataforma llegaron
// después que él y su texto crudo pasó intacto a la base de pruebas, y luego los borradores de
// pedido (0082) trajeron otra vez el nombre del cliente. En los dos casos la verificación final del
// script decía «0 datos personales», porque no sabía de esas tablas. La prueba anterior buscaba con
// grep columnas `raw_*` dentro de un `create table`: no veía un `alter table ... add column` ni una
// columna con otro nombre.
//
// Aquí se recorre el catálogo de la base ya migrada. Toda columna que no sea un número, una fecha,
// un booleano o un enum —texto, json, binario, IP, uuid, arreglos, y cualquier tipo que se agregue
// mañana— tiene que estar en uno de dos lugares:
//   - el paso 5 del script la trunca (su tabla entera) o la reescribe;
//   - noPersonalData, con el porqué de que no lleve datos de una persona.
// Una columna nueva que no está en ninguno truena aquí, y quien la agregó decide de cuál es. La
// lista es de los tipos que NO pueden guardar un dato de una persona, no de los que sí: un tipo que
// nadie previó cae del lado que exige decidir.
//
// Además, lo que el paso 5 toca lo cuenta la verificación final (FUGAS), y las dos sentencias se
// ejecutan contra el esquema actual: un renombre que deje al script apuntando a una columna que ya
// no existe falla aquí y no a media restauración.
//
// Lo que NO cubre:
//   - una columna mal clasificada en noPersonalData: lo decide quien la escribe y lo revisa el PR;
//   - un dato personal metido en una columna de catálogo —un producto llamado como un cliente—;
//   - una asignación que no borra (`set x = x`, o un `where` que no toca nada): la prueba acredita
//     la columna por nombrarla, y solo el borrador y la bitácora sembrados se ven borrados de verdad.

// noPersonalData son las columnas de texto que pasan intactas a la base de pruebas. El valor dice
// por qué no llevan datos de una persona.
var noPersonalData = map[string]string{
	// Catálogo, configuración y nombres internos del negocio: son lo que las pruebas vienen a probar.
	"branches.card_count_mode":                      "configuración",
	"branches.code":                                 "código interno de la sucursal",
	"branches.name":                                 "nombre de la sucursal",
	"branches.timezone":                             "configuración",
	"business_settings.business_name":               "nombre del negocio, no de una persona",
	"business_settings.corte_de_vista":              "configuración",
	"business_settings.costing_method":              "configuración",
	"business_settings.stock_item_label":            "configuración",
	"business_settings.timezone":                    "configuración",
	"card_terminals.name":                           "nombre de la terminal",
	"cash_concepts.name":                            "catálogo de conceptos de caja",
	"cash_denominations.currency":                   "código de moneda",
	"cash_registers.name":                           "nombre de la caja",
	"categories.color":                              "catálogo",
	"categories.image_url":                          "catálogo",
	"categories.name":                               "catálogo",
	"channels.code":                                 "catálogo",
	"channels.name":                                 "catálogo",
	"combo_slots.name":                              "catálogo",
	"companies.name":                                "nombre de la empresa",
	"companies.slug":                                "identificador de la empresa",
	"count_places.name":                             "lugar del almacén",
	"delivery_platforms.name":                       "nombre de la plataforma",
	"expense_categories.name":                       "catálogo de gastos",
	"folio_consumido.name":                          "nombre de folio de la lista fija del binario",
	"fudo_import_map.entity":                        "mapa de la carga de FUDO",
	"fudo_import_map.fudo_name":                     "nombre de producto o insumo en FUDO",
	"fudo_import_map.new_table":                     "mapa de la carga de FUDO",
	"ingredient_categories.name":                    "catálogo",
	"ingredient_presentations.name":                 "catálogo",
	"ingredient_purchase_formats.name":              "catálogo",
	"ingredient_replacements.kind":                  "tipo de reemplazo (cambio de unidad o unión)",
	"ingredients.composition_status":                "estado",
	"ingredients.name":                              "catálogo",
	"modifier_groups.name":                          "catálogo",
	"modifier_option_platform_prices.source":        "estado",
	"modifier_options.composition_status":           "estado",
	"modifier_options.name":                         "catálogo",
	"payment_methods.name":                          "catálogo",
	"product_modifier_groups.title":                 "catálogo",
	"product_platform_prices.source":                "estado",
	"products.composition_status":                   "estado",
	"products.description":                          "descripción del producto",
	"products.image_url":                            "catálogo",
	"products.name":                                 "catálogo",
	"products.sku":                                  "catálogo",
	"products.system_kind":                          "estado",
	"supplier_items.norm_name":                      "nombre del artículo según el proveedor",
	"supplier_items.raw_code":                       "código del artículo según el proveedor",
	"supplier_items.raw_name":                       "nombre del artículo según el proveedor",
	"supplier_items.status":                         "estado",
	"units.code":                                    "catálogo",
	"units.name":                                    "catálogo",
	"waste_reasons.description":                     "catálogo de motivos de merma",
	"waste_reasons.name":                            "catálogo de motivos de merma",
	"usage_daily.action":                            "medición de uso, sin persona",
	"usage_daily.screen":                            "medición de uso, sin persona",
	"usage_touches_daily.orientation":               "medición de uso, sin persona",
	"usage_touches_daily.screen":                    "medición de uso, sin persona",
	"daily_summary_sends.status":                    "estado",
	"platform_menu_reads.failure_kind":              "estado",
	"platform_menu_items.external_id":               "menú publicado de la tienda",
	"platform_menu_items.name":                      "menú publicado de la tienda",
	"platform_item_exclusions.external_id":          "menú publicado de la tienda",
	"platform_item_links.external_id":               "menú publicado de la tienda",
	"platform_item_links.local_kind":                "estado",
	"platform_price_changes.external_id":            "menú publicado de la tienda",
	"platform_price_changes.name":                   "menú publicado de la tienda",
	"platform_connections.external_store_id":        "identificador de la tienda en la plataforma",
	"platform_connections.label":                    "nombre de la tienda",
	"platform_credentials.client_id":                "identificador de la app, no secreto",
	"platform_credentials.client_secret_encrypted":  "cifrado con la llave de producción: aquí es ilegible",
	"platform_webhook_keys.key_primary_encrypted":   "cifrado con la llave de producción: aquí es ilegible",
	"platform_webhook_keys.key_secondary_encrypted": "cifrado con la llave de producción: aquí es ilegible",

	// Pedidos y cobros: estados, folios y copias del catálogo que los reportes miden.
	"orders.currency":                       "código de moneda",
	"orders.folio_name":                     "nombre de folio de la lista fija del binario",
	"orders.platform_order_ref":             "folio de la plataforma, sin datos del cliente",
	"order_drafts.discard_reason":           "valor de una lista cerrada (check)",
	"order_drafts.folio_name":               "nombre de folio de la lista fija del binario",
	"order_drafts.platform_order_ref":       "folio de la plataforma, sin datos del cliente",
	"order_drafts.status":                   "estado",
	"order_draft_lines.modifiers":           "opciones del catálogo elegidas",
	"order_lines.product_name":              "copia del nombre del producto",
	"order_line_modifiers.group_title":      "copia del catálogo",
	"order_line_modifiers.option_name":      "copia del catálogo",
	"order_payment_voids.covered":           "qué piezas cubría el pago devuelto",
	"order_payments.card_terminal_name":     "copia del nombre de la terminal",
	"platform_settlements.document_ref":     "folio del documento de pago de la plataforma",
	"platform_settlements.payout_reference": "referencia del depósito de la plataforma al negocio",
	"register_cash_movements.kind":          "estado",
	"register_sessions.card_count_mode":     "configuración",
	"register_sessions.currency":            "código de moneda",
	"register_sessions.opening_reason":      "valor de una lista cerrada (check)",
	"session_terminal_counts.terminal_name": "copia del nombre de la terminal",
	"stock_counts.status":                   "estado",
	"waste_records.status":                  "estado",
	"expenses.currency":                     "código de moneda",
	"expenses.doc_folio":                    "folio del ticket del proveedor",
	"expenses.doc_kind":                     "estado",
	"attendance_corrections.field":          "valor de una lista cerrada (check)",
	"attendance_entries.source":             "estado",
	"attendance_entries.status":             "estado",
	"attendance_entries.in_device_key":      "clave aleatoria del navegador; sin IP ni descripción no identifica",
	"attendance_entries.out_device_key":     "clave aleatoria del navegador; sin IP ni descripción no identifica",
	"audit_events.action":                   "clave estable de la acción",
	"audit_events.actor_kind":               "estado",
	"audit_events.actor_label":              "nombre de la plataforma que actuó",
	"audit_events.device_key":               "clave aleatoria del navegador; sin IP ni descripción no identifica",
	"audit_events.entity_type":              "estado",
	"platform_webhook_events.event_id":      "id del aviso; el cuerpo crudo se trunca con la tabla",

	// Identificadores aleatorios: llaves de idempotencia que manda la tableta e ids de borradores.
	// No autentican nada.
	"order_draft_adds.draft_id":           "id aleatorio",
	"order_draft_adds.line_id":            "id aleatorio",
	"order_draft_adds.op_id":              "llave de idempotencia",
	"order_draft_lines.draft_id":          "id aleatorio",
	"order_draft_lines.id":                "id aleatorio",
	"order_drafts.id":                     "id aleatorio",
	"order_line_batches.client_uuid":      "llave de idempotencia",
	"order_line_move_batches.client_uuid": "llave de idempotencia",
	"order_line_moves.client_uuid":        "llave de idempotencia",
	"order_payment_voids.client_uuid":     "llave de idempotencia",
	"order_payments.client_uuid":          "llave de idempotencia",
	"orders.client_uuid":                  "llave de idempotencia",
}

// anonymousBackupScript devuelve el paso 5 (el SQL que borra) y la consulta de FUGAS del script.
func anonymousBackupScript(t *testing.T) (step4, leaks string) {
	t.Helper()
	src := anonymousBackupSource(t)
	m := regexp.MustCompile(`(?s)<<'SQL'\n(.*?)\nSQL\n`).FindStringSubmatch(src)
	if m == nil {
		t.Fatal("no encontré el bloque <<'SQL' del paso 5 en respaldo-anonimo.sh")
	}
	step4 = m[1]
	f := regexp.MustCompile(`(?s)\nFUGAS=\$\(psql_ "\$TEST_RESTORED_DATABASE_URL" -tAc "(.*?)"\)\n`).FindStringSubmatch(src)
	if f == nil {
		t.Fatal("no encontré la consulta FUGAS= en respaldo-anonimo.sh")
	}
	// Dentro de comillas dobles de bash, \$ es un $ literal.
	leaks = strings.ReplaceAll(f[1], `\$`, `$`)
	return step4, leaks
}

func anonymousBackupSource(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "respaldo-anonimo.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestTheAnonymousBackupNeverLeavesARawDatabase: la base restaurada trae todo lo crudo de producción
// hasta que el paso que borra termina. Antes, un fallo en ese paso (pasó con el respaldo real, por
// las restricciones NOT VALID) hacía rollback y dejaba la base entera sin anonimizar. Y el paso corre
// después de migrar, contra el mismo esquema que valida la prueba de abajo: al revés, una columna
// nueva que la prueba exige borrar todavía no existe en el respaldo y el paso truena.
func TestTheAnonymousBackupNeverLeavesARawDatabase(t *testing.T) {
	t.Parallel()
	src := anonymousBackupSource(t)
	trap := regexp.MustCompile(`(?s)\nlimpiar\(\) \{\n(.*?)\n\}\ntrap limpiar EXIT\n`).FindStringSubmatch(src)
	if trap == nil || !strings.Contains(trap[1], `drop database if exists \"$NOMBRE_BASE\"`) {
		t.Fatal("respaldo-anonimo.sh no borra la base destino al salir con error: quedaría con los datos crudos")
	}
	if !strings.Contains(trap[1], `"$BASE_ANONIMA" != 1`) {
		t.Fatal("limpiar no distingue la base ya anonimizada: borraría también la buena")
	}
	migrate := strings.Index(src, "go run ./cmd/migrate")
	step := strings.Index(src, "<<'SQL'")
	leaks := strings.Index(src, `if [[ "$FUGAS" -ne 0 ]]`)
	done := strings.Index(src, "\nBASE_ANONIMA=1\n")
	switch {
	case migrate < 0 || step < 0 || leaks < 0 || done < 0:
		t.Fatal("respaldo-anonimo.sh cambió de forma: no encontré las migraciones, el paso que borra, FUGAS o BASE_ANONIMA=1")
	case migrate > step:
		t.Error("el paso que borra corre antes de migrar: valida un esquema distinto del que la prueba revisa")
	case done < leaks:
		t.Error("BASE_ANONIMA=1 se pone antes de revisar FUGAS: una base con fugas sobreviviría")
	}
}

func withoutSQLComments(sql string) string {
	return regexp.MustCompile(`--[^\n]*`).ReplaceAllString(sql, "")
}

// coveredByStep4 dice qué tablas trunca el paso que borra y qué columnas reescribe (tabla.columna).
func coveredByStep4(step4 string) (truncated map[string]bool, rewritten map[string]bool) {
	sql := withoutSQLComments(step4)
	truncated, rewritten = map[string]bool{}, map[string]bool{}
	for _, m := range regexp.MustCompile(`(?is)\btruncate\s+([^;]+);`).FindAllStringSubmatch(sql, -1) {
		for _, tbl := range strings.Split(m[1], ",") {
			truncated[strings.TrimSpace(tbl)] = true
		}
	}
	// update <tabla> set <a> = …, <b> = … [where …];  Las literales se vacían antes de buscar
	// asignaciones, para que un '=' dentro de un texto no cuente como columna.
	literals := regexp.MustCompile(`'[^']*'`)
	for _, m := range regexp.MustCompile(`(?is)\bupdate\s+(\w+)\s+set\s+(.*?)(?:\bwhere\b[^;]*)?;`).FindAllStringSubmatch(sql, -1) {
		for _, a := range regexp.MustCompile(`(?:^|,)\s*(\w+)\s*=`).FindAllStringSubmatch(literals.ReplaceAllString(m[2], "''"), -1) {
			rewritten[m[1]+"."+a[1]] = true
		}
	}
	return truncated, rewritten
}

func TestEveryColumnIsCoveredByTheAnonymousBackup(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	step4, leaks := anonymousBackupScript(t)
	truncated, rewritten := coveredByStep4(step4)
	// Sin CASCADE: arrastraría en silencio cualquier tabla que un día referencie a las truncadas.
	if regexp.MustCompile(`(?i)\btruncate\b[^;]*\bcascade\b`).MatchString(withoutSQLComments(step4)) {
		t.Error("un truncate del paso que borra usa CASCADE")
	}

	rows, err := st.Pool.Query(ctx, `
		select c.relname || '.' || a.attname,
		       not (ty.typcategory in ('N', 'B', 'D', 'E')
		            or ty.typname in ('date', 'time', 'timestamp', 'timestamptz', 'interval'))
		  from pg_attribute a
		  join pg_class c on c.oid = a.attrelid
		  join pg_namespace n on n.oid = c.relnamespace and n.nspname = 'public'
		  join pg_type ty on ty.oid = a.atttypid
		 -- 'm': una vista materializada viaja en el pg_dump con su contenido.
		 where c.relkind in ('r', 'p', 'm') and a.attnum > 0 and not a.attisdropped
		   and c.relname <> 'goose_db_version'
		 order by 1`)
	if err != nil {
		t.Fatal(err)
	}
	var columns []string
	exists, isText := map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var c string
		var text bool
		if err := rows.Scan(&c, &text); err != nil {
			t.Fatal(err)
		}
		exists[c] = true
		if text {
			isText[c] = true
			columns = append(columns, c)
		}
	}
	rows.Close()
	if len(columns) < 150 {
		t.Fatalf("solo %d columnas que clasificar: la consulta al catálogo dejó de encontrarlas", len(columns))
	}

	var unclassified []string
	for _, c := range columns {
		table := c[:strings.Index(c, ".")]
		if truncated[table] || rewritten[c] {
			continue
		}
		if _, ok := noPersonalData[c]; !ok {
			unclassified = append(unclassified, c)
		}
	}
	if len(unclassified) > 0 {
		t.Errorf("columnas que el anonimizador deja pasar sin que nadie haya dicho que no llevan "+
			"datos de una persona (%d). Bórralas en el paso 5 de scripts/respaldo-anonimo.sh y cuéntalas en "+
			"FUGAS, o agrégalas a noPersonalData con el porqué:\n  %s",
			len(unclassified), strings.Join(unclassified, "\n  "))
	}
	var stale []string
	for c := range noPersonalData {
		if !isText[c] {
			stale = append(stale, c)
		}
	}
	for c := range rewritten {
		if !exists[c] {
			stale = append(stale, c+" (en el paso que borra)")
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("columnas que ya no existen, o en noPersonalData que no hace falta clasificar (%d):\n  %s", len(stale), strings.Join(stale, "\n  "))
	}

	// Lo que el paso que borra toca lo cuenta FUGAS: una subconsulta por renglón, de la misma tabla y que
	// nombre la columna. Sin eso la verificación dice 0 aunque la sentencia no haya corrido.
	countedIn := map[string][]string{}
	for _, m := range regexp.MustCompile(`\(select count\(\*\) from (\w+)([^\n]*)\)`).FindAllStringSubmatch(leaks, -1) {
		countedIn[m[1]] = append(countedIn[m[1]], m[2])
	}
	var uncounted []string
	for tbl := range truncated {
		if _, ok := countedIn[tbl]; !ok {
			uncounted = append(uncounted, tbl+" (truncada)")
		}
	}
	for c := range rewritten {
		if !isText[c] {
			continue
		}
		tbl, col, _ := strings.Cut(c, ".")
		word := regexp.MustCompile(`\b` + col + `\b`)
		found := false
		for _, where := range countedIn[tbl] {
			found = found || word.MatchString(where)
		}
		if !found {
			uncounted = append(uncounted, c)
		}
	}
	sort.Strings(uncounted)
	if len(uncounted) > 0 {
		t.Errorf("el paso que borra los borra pero FUGAS no los cuenta, así que diría 0 aunque no hubieran corrido (%d):\n  %s",
			len(uncounted), strings.Join(uncounted, "\n  "))
	}
	if t.Failed() {
		return
	}

	// Las dos sentencias corren contra el esquema de hoy, sobre un borrador con el nombre del
	// cliente (el hueco de la 0082): FUGAS lo ve antes y no lo ve después.
	user := makeUser(t, st, "anonimo", "cajero")
	if _, err := st.Pool.Exec(ctx, `
		insert into order_drafts (id, opened_by, folio_name, folio_scheme, customer_name)
		values (gen_random_uuid(), $1, 'Siamés', 'razas', 'Juana Pérez Cliente')`, user); err != nil {
		t.Fatalf("sembrar el borrador: %v", err)
	}
	// Una fila vieja que no cumple una restricción NOT VALID: así quedaron en producción las de
	// 0092 y 0101 (un renglón de compra en $0). Postgres vuelve a revisar la restricción en CUALQUIER
	// update de esa fila, aunque la columna no tenga que ver, y el paso tronaba con el respaldo
	// real. Se reproduce con una restricción propia sobre la misma fila del borrador.
	if _, err := st.Pool.Exec(ctx, `alter table order_drafts
		add constraint legacy_row_breaks_it check (folio_name <> 'Siamés') not valid`); err != nil {
		t.Fatal(err)
	}
	// La bitácora es de solo agregar ni siquiera para el owner: el paso apaga su trigger para borrar
	// el antes, el después, la IP y el aparato, y tiene que dejarlo encendido.
	if _, err := st.Pool.Exec(ctx, `
		insert into audit_events (action, actor_kind, actor_user_id, entity_type, entity_id, ip, device, before, after)
		values ('order.cancelled', 'user', $1, 'order', 1, '10.1.2.3', '{"ua":"Tableta de Juana"}',
		        '{"status":"abierta"}', '{"reason":"Juana Pérez reclamó"}')`, user); err != nil {
		t.Fatalf("sembrar la bitácora: %v", err)
	}
	var before, after int
	if err := st.Pool.QueryRow(ctx, leaks).Scan(&before); err != nil {
		t.Fatalf("FUGAS no corre contra el esquema actual: %v", err)
	}
	if before == 0 {
		t.Fatal("FUGAS no ve el borrador con nombre de cliente ni el usuario sembrado: la verificación no mide nada")
	}
	if _, err := st.Pool.Exec(ctx, step4); err != nil {
		t.Fatalf("el paso que borra no corre contra el esquema actual: %v", err)
	}
	if err := st.Pool.QueryRow(ctx, leaks).Scan(&after); err != nil {
		t.Fatal(err)
	}
	var name *string
	if err := st.Pool.QueryRow(ctx, `select customer_name from order_drafts`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != nil || after != 0 {
		t.Fatalf("tras el anonimizado el borrador conserva el nombre %v y FUGAS cuenta %d", name, after)
	}
	var auditLeft, triggersOff int
	if err := st.Pool.QueryRow(ctx, `
		select (select count(*) from audit_events
		         where before is not null or after is not null or ip is not null or device is not null),
		       (select count(*) from pg_trigger
		         where tgrelid = 'audit_events'::regclass and not tgisinternal and tgenabled <> 'O')`).
		Scan(&auditLeft, &triggersOff); err != nil {
		t.Fatal(err)
	}
	if auditLeft != 0 || triggersOff != 0 {
		t.Fatalf("bitácora: %d eventos conservan antes/después/IP/aparato y %d triggers quedaron apagados",
			auditLeft, triggersOff)
	}
	// La restricción vuelve como estaba: sin validar, para no exigirle a la fila vieja lo que en
	// producción tampoco se le exige.
	var validated bool
	if err := st.Pool.QueryRow(ctx, `select convalidated from pg_constraint
		 where conname = 'legacy_row_breaks_it'`).Scan(&validated); err != nil {
		t.Fatalf("el paso que borra no repuso la restricción NOT VALID: %v", err)
	}
	if validated {
		t.Fatal("el paso que borra repuso la restricción validada; en producción está NOT VALID")
	}
}
