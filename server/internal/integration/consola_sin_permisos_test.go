//go:build integration

package integration

import (
	"context"
	"testing"
)

// LO QUE LA CONSOLA NO PUEDE TOCAR, PREGUNTÁNDOSELO A POSTGRES (US3: FR-005, FR-006, FR-009).
//
// Esta es la barrera de verdad: no un `if` en un handler ni una ruta que nadie escribió, sino un
// permiso que no existe. Un endpoint nuevo que por descuido consultara `orders` desde la consola no
// devolvería datos de más — fallaría con 42501 en el primer request.
//
// Se prueba bajo el ROL, no como dueño: para el dueño ni los grants ni RLS existen y todo esto
// pasaría en verde con la tabla abierta de par en par.
func TestLaConsolaNoAlcanzaLaOperacionDeNingunaEmpresa(t *testing.T) {
	st := newTestStore(t)
	prepararRolDePlataforma(t, st)
	plataforma := platformRoleStore(t)
	ctx := context.Background()

	// Las cinco que duelen: el dinero, quién lo cobró, en qué turno, qué se gastó y quién trabaja
	// ahí. Si mañana nace una tabla de operación, hereda esta protección sola —los grants son
	// lista de lo permitido— y lo único que hay que hacer es agregarla a esta lista.
	for _, tabla := range []string{"orders", "order_payments", "register_sessions", "expenses", "users"} {
		t.Run(tabla, func(t *testing.T) {
			var n int
			err := plataforma.Pool.QueryRow(ctx, "select count(*) from "+tabla).Scan(&n)
			if err == nil {
				t.Fatalf("la consola leyó %s (%d filas): tiene permiso sobre la operación de los clientes", tabla, n)
			}
			if !esPermisoDenegado(err) {
				t.Fatalf("%s no falló por permiso denegado sino por otra cosa, así que este test no prueba nada: %v", tabla, err)
			}
		})
	}

	// Y lo que sí: el catálogo de clientes. Sin esto, los rechazos de arriba podrían venir de una
	// conexión rota y el test entero sería un espejismo.
	var empresas int
	if err := plataforma.Pool.QueryRow(ctx, "select count(*) from companies").Scan(&empresas); err != nil {
		t.Fatalf("la consola no pudo leer companies: %v — sin esto no hay consola", err)
	}
	if empresas < 1 {
		t.Fatal("la consola ve cero empresas en una base recién migrada: la política de RLS no está haciendo su parte")
	}
}

// Y TAMPOCO PUEDE ESCRIBIR (FR-009).
//
// No es redundante con el test de arriba: los grants son de `select`, así que la escritura está
// denegada POR OMISIÓN — y lo que se omite no se nota hasta que alguien lo agrega. Este test es lo
// que hace que las acciones de soporte (spec 018) tengan que ser una decisión y no un accidente:
// quien agregue un `grant insert` para salir del paso, lo ve en rojo.
func TestLaConsolaNoPuedeEscribirNadaDeUnCliente(t *testing.T) {
	st := newTestStore(t)
	prepararRolDePlataforma(t, st)
	plataforma := platformRoleStore(t)
	ctx := context.Background()

	casos := map[string]string{
		"insert": `insert into companies (slug, name) values ('inventada', 'Inventada')`,
		"update": `update companies set name = 'Otro Nombre'`,
		"delete": `delete from companies`,
	}
	for nombre, sql := range casos {
		t.Run(nombre, func(t *testing.T) {
			_, err := plataforma.Pool.Exec(ctx, sql)
			if err == nil {
				t.Fatalf("la consola pudo hacer %s sobre companies: puede modificar el negocio de un cliente", nombre)
			}
			if !esPermisoDenegado(err) {
				t.Fatalf("%s no falló por permiso denegado: %v", nombre, err)
			}
		})
	}

	// El dueño comprueba que nada cambió: un rechazo que llegara DESPUÉS de escribir sería peor que
	// no tener barrera, porque el test diría que todo está bien.
	var nombre string
	if err := st.Pool.QueryRow(ctx, `select name from companies where id = $1`, defaultCompanyID).Scan(&nombre); err != nil {
		t.Fatalf("releer la empresa: %v", err)
	}
	if nombre == "Otro Nombre" {
		t.Fatal("el update rechazado sí escribió")
	}
}
