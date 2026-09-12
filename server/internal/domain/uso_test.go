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
	agregado := PreAgregarUso(lote, RoleCajero)

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
func TestElCorteDeRolSeApagaCuandoIdentifica(t *testing.T) {
	// Con una sola persona en el rol, el corte no se da. Es el mismo borde de siempre, ahora
	// expresado sobre la plantilla entera: ver TestElBaldeSinCorteNoPuedeSerUnaSolaPersona para el
	// caso que esta versión de la regla agregó.
	plantilla := map[Role]int{RoleCajero: 3, RoleMesero: 3}
	casos := []struct {
		activos int
		quiere  DecisionDeCorte
	}{
		{0, NoGuardar}, // nadie: no hay corte que dar, y el balde no tapa a nadie
		{1, NoGuardar}, // el caso que importa
		{2, GuardarConRol},
		{9, GuardarConRol},
	}
	for _, c := range casos {
		plantilla[RoleGerente] = c.activos
		if got := CortarPorRol(plantilla, RoleGerente); got != c.quiere {
			t.Errorf("con %d gerentes activos = %v, quiere %v", c.activos, got, c.quiere)
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

// TODA PANTALLA MEDIBLE DICE QUIÉN PUEDE ABRIRLA.
//
// La mitad barata de la desincronización: una pantalla nueva en la lista blanca sin su renglón de
// roles se descartaría SIEMPRE —`PantallaPermitidaParaRol` devuelve false para lo que no conoce— y
// el mapa mostraría un cero permanente que se lee como «nadie la usa».
func TestTodaPantallaMedibleTieneRoles(t *testing.T) {
	for _, pantalla := range PantallasMedibles() {
		if !PantallaPermitidaParaRol(pantalla, RoleAdmin) {
			t.Errorf("la pantalla %q no dice qué roles pueden abrirla: sus eventos se descartarían todos, en silencio", pantalla)
		}
	}
}

// UN ROL NO PUEDE REPORTAR USO DE UNA PANTALLA QUE NO PUEDE ABRIR.
//
// La lista blanca acota el CONJUNTO de valores, no su coherencia. Sin esto, un mesero manda treinta
// aperturas por minuto de la pantalla de usuarios —que un GET suyo recibiría con 403— y el mapa
// dice que es la más usada del sistema. No es una fuga: es una medición que se puede llenar de
// mentiras desde adentro, y entonces deja de servir para lo único que existe.
func TestUnRolNoReportaPantallasQueNoPuedeAbrir(t *testing.T) {
	mentira := []EventoDeUso{{Pantalla: "usuarios"}, {Pantalla: "negocio"}}
	if n := len(PreAgregarUso(mentira, RoleMesero)); n != 0 {
		t.Fatalf("el mesero reportó %d pantallas de administración: el mapa se llena de mentiras desde adentro", n)
	}
	// Y lo que sí puede abrir, se cuenta.
	if n := len(PreAgregarUso([]EventoDeUso{{Pantalla: "pos"}}, RoleMesero)); n != 1 {
		t.Fatal("el mesero no pudo reportar el POS, que es donde trabaja")
	}
	// El admin sí ve las de administración.
	if n := len(PreAgregarUso(mentira, RoleAdmin)); n != 2 {
		t.Fatalf("el admin reportó %d de 2 pantallas suyas", n)
	}
}

// EL BALDE «SIN CORTE» PUEDE SER UNA SOLA PERSONA, y ése es el caso que la regla vieja no veía.
//
// La supresión se decidía rol por rol y de forma independiente: cada rol bajo el umbral caía en el
// mismo `role is null`. Cuando **exactamente un rol** queda por debajo, ese balde ES esa persona —y
// la consola lo pinta con la etiqueta «sin corte», que promete justo lo contrario—.
//
// El escenario es la plantilla típica del cliente de hoy, no una hipótesis.
func TestElBaldeSinCorteNoPuedeSerUnaSolaPersona(t *testing.T) {
	casos := []struct {
		nombre  string
		activos map[Role]int
		rol     Role
		quiere  DecisionDeCorte
	}{
		{
			// 1 admin · 2 gerentes · 3 cajeros · 2 meseros: el único suprimido es el admin, así que
			// `sin corte` es el dueño y su rejilla completa queda etiquetada con su puesto.
			nombre:  "un solo rol bajo el umbral: no se guarda nada",
			activos: map[Role]int{RoleAdmin: 1, RoleGerente: 2, RoleCajero: 3, RoleMesero: 2},
			rol:     RoleAdmin,
			quiere:  NoGuardar,
		},
		{
			nombre:  "y los roles que sí se cortan siguen contándose",
			activos: map[Role]int{RoleAdmin: 1, RoleGerente: 2, RoleCajero: 3, RoleMesero: 2},
			rol:     RoleCajero,
			quiere:  GuardarConRol,
		},
		{
			// Dos roles de una persona cada uno: el balde tiene dos, y ya no señala a nadie.
			nombre:  "dos roles bajo el umbral: el balde tapa a los dos",
			activos: map[Role]int{RoleAdmin: 1, RoleMesero: 1, RoleCajero: 3},
			rol:     RoleAdmin,
			quiere:  GuardarSinCorte,
		},
		{
			nombre:  "un local de una sola persona no se mide",
			activos: map[Role]int{RoleAdmin: 1},
			rol:     RoleAdmin,
			quiere:  NoGuardar,
		},
		{
			// Un rol sin nadie activo no aporta al balde: no hay a quién tapar con él.
			nombre:  "los roles vacíos no cuentan como personas en el balde",
			activos: map[Role]int{RoleAdmin: 1, RoleGerente: 0, RoleMesero: 0, RoleCajero: 4},
			rol:     RoleAdmin,
			quiere:  NoGuardar,
		},
		{
			nombre:  "un rol que no existe en la plantilla no se guarda",
			activos: map[Role]int{RoleCajero: 5},
			rol:     Role("cocina"),
			quiere:  NoGuardar,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := CortarPorRol(c.activos, c.rol); got != c.quiere {
				t.Fatalf("CortarPorRol(%v, %q) = %v, quiere %v", c.activos, c.rol, got, c.quiere)
			}
		})
	}
}
