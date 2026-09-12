package domain

import (
	"strings"
	"testing"
)

// LA LISTA BLANCA ES LA ÚNICA PUERTA.
//
// Sin ella, cuántos valores distintos hay en la base lo decide el cliente: un bucle en el front, o
// una versión vieja que manda nombres que ya no existen, llenan la tabla de basura y el agregado
// deja de agregar. Además es lo que hace legible el mapa — «cobrar» es una acción, «clic en el
// botón azul» no.
func TestLaListaBlancaDeUso(t *testing.T) {
	casos := []struct {
		nombre   string
		pantalla string
		accion   string
		valido   bool
	}{
		{"pantalla conocida, sin acción", "pos", "", true},
		{"pantalla y acción conocidas", "caja", "cerrar-turno", true},
		{"pantalla inventada", "pantalla-que-no-existe", "", false},
		{"acción inventada en pantalla buena", "pos", "hacer-magia", false},
		{"pantalla vacía", "", "", false},
		{"pantalla larguísima", strings.Repeat("a", 100), "", false},
		// El servidor no puede confiar en que el cliente recorte: lo que llega, llega.
		{"acción larguísima", "pos", strings.Repeat("b", 100), false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := EventoDeUsoValido(c.pantalla, c.accion); got != c.valido {
				t.Fatalf("EventoDeUsoValido(%q, %q) = %v, quiere %v", c.pantalla, c.accion, got, c.valido)
			}
		})
	}
}

// EL LOTE SE PRE-AGREGA ANTES DE ESCRIBIR, Y ESO NO ES UNA OPTIMIZACIÓN COSMÉTICA.
//
// Cada `update` deja en Postgres la versión vieja de la fila muerta. Medido sobre el esquema real:
// 135 filas recibiendo 2,000 incrementos de a uno pasan de 64 kB a 232 kB antes de que autovacuum
// llegue —y con decenas de miles de filas vivas, autovacuum tarda días en disparar—. Agrupar el
// lote convierte hasta 50 escrituras físicas en una.
func TestElLoteDeUsoSePreAgrega(t *testing.T) {
	lote := []EventoDeUso{
		{Pantalla: "pos"},
		{Pantalla: "pos"},
		{Pantalla: "pos", Accion: "cobrar"},
		{Pantalla: "caja", Accion: "cerrar-turno"},
		{Pantalla: "pos"},
	}
	agregado := PreAgregarUso(lote)

	if len(agregado) != 3 {
		t.Fatalf("quedaron %d combinaciones y hay 3 distintas: %+v", len(agregado), agregado)
	}
	for _, a := range agregado {
		quiere := 1
		if a.Pantalla == "pos" && a.Accion == "" {
			quiere = 3
		}
		if a.Veces != quiere {
			t.Errorf("(%s, %q) = %d veces, quiere %d", a.Pantalla, a.Accion, a.Veces, quiere)
		}
	}
}

// Un lote enorme se recorta ANTES de tocar nada.
//
// El tope no protege la base —eso lo hace el resto— sino el camino: la llave del limitador y la
// línea de log viven de lo que llega, y un lote de miles solo puede venir de un bucle.
func TestElLoteDeUsoSeRecorta(t *testing.T) {
	lote := make([]EventoDeUso, MaxEventosPorLote+50)
	for i := range lote {
		lote[i] = EventoDeUso{Pantalla: "pos"}
	}
	if n := len(RecortarLoteDeUso(lote)); n != MaxEventosPorLote {
		t.Fatalf("el lote quedó en %d y el tope es %d", n, MaxEventosPorLote)
	}
	corto := []EventoDeUso{{Pantalla: "pos"}}
	if n := len(RecortarLoteDeUso(corto)); n != 1 {
		t.Fatalf("un lote corto se recortó a %d", n)
	}
}

// EL CORTE POR ROL SE APAGA CUANDO IDENTIFICA A UNA PERSONA (FR-009).
//
// Decir «el rol gerente hizo estas 40 acciones» en una empresa con UN gerente es decir su nombre.
// Es la mitad que hace real la promesa de no guardar la identidad: sin esto, la promesa se rompe en
// la pantalla aunque la columna no exista.
//
// Se decide con un número —cuántos usuarios activos tiene ese rol— y no con la base, para que sea
// puro y para que la decisión ocurra al ESCRIBIR, que es donde no se puede deshacer.
func TestElCorteDeRolSeApagaCuandoIdentifica(t *testing.T) {
	casos := []struct {
		activos   int
		permitido bool
	}{
		{0, false}, // nadie: no hay corte que dar
		{1, false}, // el caso que importa
		{2, true},
		{9, true},
	}
	for _, c := range casos {
		if got := CorteDeRolPermitido(c.activos); got != c.permitido {
			t.Errorf("CorteDeRolPermitido(%d) = %v, quiere %v", c.activos, got, c.permitido)
		}
	}
}

// Los CUATRO roles del sistema se pueden contar, no tres.
//
// El spec nombraba tres —cajero, gerente, administrador— y `mesero` existe desde la migración 0001.
// Una lista de tres lo manda al camino de error en vez de contarlo, y el mapa se quedaría ciego
// justo con el rol del que menos se sabe.
func TestSeCuentanLosCuatroRoles(t *testing.T) {
	for _, r := range []Role{RoleAdmin, RoleGerente, RoleCajero, RoleMesero} {
		if !RolMedible(r) {
			t.Errorf("el rol %q no se puede medir, y existe en el sistema", r)
		}
	}
	if RolMedible(Role("inventado")) {
		t.Error("un rol que no existe no debería medirse")
	}
}
