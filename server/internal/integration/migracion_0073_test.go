//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
)

// LA MIGRACIÓN DE LOS PEDIDOS DE PLATAFORMA (spec 021).
//
// Tres familias de cosa que este archivo vigila, y ninguna se ve desde un unitario:
//
//   - LOS ÍNDICES DE TENANT que faltaban en tablas VIEJAS. Sin `users (company_id, id)` y
//     `platform_connections (company_id, id)`, las FK compuestas de esta migración no se pueden
//     ni crear: el `alter table` truena en seco. Se comprobó contra Postgres real que ninguna de
//     las dos los tenía antes de la 0073.
//   - LOS GRANTS. El `grant` puntual de la 0024 enseñó que una tabla nueva sin su grant responde
//     `42501` en el primer request de producción y nunca en desarrollo, porque la API de dev se
//     conecta como owner.
//   - EL `check` DE `orders` QUE HUBO QUE RELAJAR. Un pedido de Uber PARA RECOGER no entraba, y es
//     el camino de prueba más barato que tenemos: no lleva repartidor.

func TestLosIndicesDeTenantExistenParaLasFKCompuestas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Sin estos dos, ningún `foreign key (x, company_id) references tabla (id, company_id)` se
	// puede declarar. Postgres exige un único que case exactamente con las columnas referenciadas.
	for _, idx := range []struct{ nombre, tabla string }{
		{"users_tenant_key", "users"},
		{"platform_connections_tenant_key", "platform_connections"},
	} {
		var existe bool
		if err := st.Pool.QueryRow(ctx,
			`select exists (select 1 from pg_indexes where indexname = $1 and tablename = $2)`,
			idx.nombre, idx.tabla,
		).Scan(&existe); err != nil {
			t.Fatalf("consultar %s: %v", idx.nombre, err)
		}
		if !existe {
			t.Fatalf("falta el índice %s sobre %s: sin él la FK compuesta no se puede crear", idx.nombre, idx.tabla)
		}
	}
}

func TestLasTablasDeLa0073TienenRLSYSusGrants(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	tablas := []string{
		"platform_webhook_keys",
		"platform_webhook_events",
		"platform_incoming_orders",
		"platform_incoming_order_lines",
	}

	for _, tabla := range tablas {
		var rls bool
		if err := st.Pool.QueryRow(ctx,
			`select relrowsecurity from pg_class where relname = $1`, tabla,
		).Scan(&rls); err != nil {
			t.Fatalf("consultar RLS de %s: %v", tabla, err)
		}
		if !rls {
			t.Fatalf("%s no tiene RLS activo: una empresa vería los pedidos de otra", tabla)
		}

		// Sin política, `enable row level security` bloquea TODO para el rol de la aplicación.
		var politicas int
		if err := st.Pool.QueryRow(ctx,
			`select count(*) from pg_policies where tablename = $1`, tabla,
		).Scan(&politicas); err != nil {
			t.Fatalf("consultar políticas de %s: %v", tabla, err)
		}
		if politicas == 0 {
			t.Fatalf("%s tiene RLS pero ninguna política: el rol de la aplicación no puede leer nada", tabla)
		}

		// Los cuatro verbos. `select` solo dejaría la feature a medias sin que nada falle al
		// arrancar: el fallo aparece en el primer pedido que entra, en producción.
		for _, verbo := range []string{"select", "insert", "update", "delete"} {
			var puede bool
			if err := st.Pool.QueryRow(ctx,
				`select has_table_privilege('gatobobah_app', $1, $2)`, tabla, verbo,
			).Scan(&puede); err != nil {
				t.Fatalf("consultar privilegio %s sobre %s: %v", verbo, tabla, err)
			}
			if !puede {
				t.Fatalf("a gatobobah_app le falta %s sobre %s: en producción sale 42501 y en desarrollo nunca", verbo, tabla)
			}
		}
	}
}

// UN PEDIDO DE PLATAFORMA PARA RECOGER EN TIENDA.
//
// La 0007 trae `check (service_type = 'domicilio' or delivery_platform_id is null)`. Mientras la
// captura era manual nadie lo notaba, porque quien capturaba elegía «domicilio». Con los pedidos
// entrando solos el tipo lo dice Uber, y el primer pedido para recoger revienta la inserción.
func TestUnPedidoDePlataformaPuedeSerParaRecoger(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	empresa := makeCompany(t, st, "recoger-en-tienda")
	usuario := makeUserIn(t, st, empresa, "cajera-recoge", "cajero")
	plataforma := platformID(t, st, empresa, "Uber Eats")

	n := 0
	insertar := func(tipo string) error {
		n++
		_, err := st.Pool.Exec(ctx, `
			insert into orders (client_uuid, business_date, daily_number, service_type,
			                    delivery_platform_id, opened_by, company_id)
			values (gen_random_uuid(), current_date, $1, $2, $3, $4, $5)`,
			900+n, tipo, plataforma, usuario, empresa)
		return err
	}

	if err := insertar("para_llevar"); err != nil {
		t.Fatalf("un pedido de plataforma PARA RECOGER no entró, y es el camino de prueba más barato "+
			"que tenemos —no lleva repartidor—: %v", err)
	}
	// Y lo que el `check` sigue impidiendo, a propósito: de mostrador con plataforma no tiene
	// sentido. Relajarlo entero habría dejado pasar eso también.
	if err := insertar("mostrador"); err == nil {
		t.Fatal("un pedido de plataforma DE MOSTRADOR entró: el check se relajó de más")
	} else if !strings.Contains(err.Error(), "check") && !strings.Contains(err.Error(), "restricción") {
		t.Fatalf("falló por otra razón distinta del check: %v", err)
	}
}

// LA GUARDA DE CANCELACIÓN SOBREVIVIÓ AL `drop constraint`.
//
// La 0073 borra `orders_check` para relajarlo, y ese nombre lo AUTOGENERÓ Postgres: la 0007 declaró
// dos checks sin nombre y quedaron `orders_check` y `orders_check1`. Si la numeración se recorre, el
// `drop` se lleva la guarda equivocada —la que exige que una orden cancelada tenga hora, responsable
// y motivo— sin que nada falle al migrar. El defecto saldría meses después, en una cancelación sin
// rastro de quién la hizo.
func TestLaGuardaDeCancelacionSobreviveALaRelajacionDelCheck(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	empresa := makeCompany(t, st, "guarda-cancelacion")
	usuario := makeUserIn(t, st, empresa, "cajera-cancela", "cajero")

	// Cancelada sin hora, sin responsable y sin motivo: tiene que rebotar.
	_, err := st.Pool.Exec(ctx, `
		insert into orders (client_uuid, business_date, daily_number, service_type,
		                    status, opened_by, company_id)
		values (gen_random_uuid(), current_date, 911, 'mostrador', 'cancelada', $1, $2)`,
		usuario, empresa)
	if err == nil {
		t.Fatal("se pudo cancelar una orden sin hora, sin responsable y sin motivo: " +
			"la 0073 se llevó la guarda equivocada al relajar el check de servicio")
	}
	if !strings.Contains(err.Error(), "check") && !strings.Contains(err.Error(), "restricción") {
		t.Fatalf("rebotó por otra razón: %v", err)
	}
}
