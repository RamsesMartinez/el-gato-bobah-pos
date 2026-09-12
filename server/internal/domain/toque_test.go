package domain

import "testing"

func TestToqueValido(t *testing.T) {
	casos := []struct {
		nombre string
		toque  Toque
		valido bool
	}{
		{"normal", Toque{Pantalla: "pos", Celda: 37, Orientacion: OrientacionHorizontal}, true},
		{"la primera celda", Toque{Pantalla: "pos", Celda: 0, Orientacion: OrientacionVertical}, true},
		{"la última", Toque{Pantalla: "pos", Celda: CeldasDeLaRejilla - 1, Orientacion: OrientacionHorizontal}, true},
		{"una celda que no existe", Toque{Pantalla: "pos", Celda: CeldasDeLaRejilla, Orientacion: OrientacionHorizontal}, false},
		{"celda negativa", Toque{Pantalla: "pos", Celda: -1, Orientacion: OrientacionHorizontal}, false},
		// Una pantalla que se mide en la 017 pero NO está instrumentada para toque: medir doce para
		// mirar dos es volumen y ruido.
		{"pantalla no instrumentada", Toque{Pantalla: "caja", Celda: 0, Orientacion: OrientacionHorizontal}, false},
		{"pantalla inventada", Toque{Pantalla: "no-existe", Celda: 0, Orientacion: OrientacionHorizontal}, false},
		// El caso del balde invisible: si esto pasara, la fila entra a la base y la consola nunca
		// la muestra.
		{"orientación inventada", Toque{Pantalla: "pos", Celda: 0, Orientacion: "landscape"}, false},
		{"sin orientación", Toque{Pantalla: "pos", Celda: 0}, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := ToqueValido(c.toque); got != c.valido {
				t.Fatalf("ToqueValido(%+v) = %v, quiere %v", c.toque, got, c.valido)
			}
		})
	}
}

// LA LISTA INSTRUMENTADA ES SUBCONJUNTO DE LA QUE SE MIDE (spec 017).
//
// El toque entra por el MISMO endpoint que las aperturas y se valida también contra
// `pantallasMedibles`. Una pantalla instrumentada que no esté allá tendría todos sus toques
// descartados en silencio: la rejilla saldría vacía sin un solo error, y eso se lee como «nadie
// toca ahí» en vez de como «está mal configurada».
func TestLasPantallasConToqueSonSubconjunto(t *testing.T) {
	medibles := map[string]bool{}
	for _, p := range PantallasMedibles() {
		medibles[p] = true
	}
	for _, p := range PantallasConToque() {
		if !medibles[p] {
			t.Errorf("la pantalla %q está instrumentada para toque y no está en la lista de la 017: sus toques se descartan todos, en silencio", p)
		}
	}
}

func TestLaRejillaCambiaDeFormaConLaOrientacion(t *testing.T) {
	col, fil := RejillaDe(OrientacionHorizontal)
	if col != 12 || fil != 7 {
		t.Fatalf("horizontal = %d×%d, quiere 12×7", col, fil)
	}
	col, fil = RejillaDe(OrientacionVertical)
	if col != 7 || fil != 12 {
		t.Fatalf("vertical = %d×%d, quiere 7×12", col, fil)
	}
	// Las dos tienen el mismo número de celdas, que es lo que permite un solo `check` de rango en
	// la base — y también la razón por la que ese check NO puede atrapar una orientación inventada.
	if CeldasDeLaRejilla != 12*7 {
		t.Fatal("las dos rejillas deben tener el mismo número de celdas")
	}
}

// PreAgregarToques es lo que hace que mil toques sean un renglón.
func TestPreAgregarToques(t *testing.T) {
	lote := []Toque{
		{Pantalla: "pos", Celda: 37, Orientacion: OrientacionHorizontal},
		{Pantalla: "pos", Celda: 37, Orientacion: OrientacionHorizontal},
		{Pantalla: "pos", Celda: 37, Orientacion: OrientacionHorizontal},
		// La MISMA celda en la otra forma de pantalla: es otro lugar y no se suma con la anterior.
		{Pantalla: "pos", Celda: 37, Orientacion: OrientacionVertical},
		// Lo que no se cuenta: pantalla no instrumentada, celda fuera y orientación inventada.
		{Pantalla: "caja", Celda: 1, Orientacion: OrientacionHorizontal},
		{Pantalla: "pos", Celda: CeldasDeLaRejilla, Orientacion: OrientacionHorizontal},
		{Pantalla: "pos", Celda: 1, Orientacion: "landscape"},
	}

	agregado := PreAgregarToques(lote, RoleCajero)
	if len(agregado) != 2 {
		t.Fatalf("quedaron %d combinaciones y son 2 (la celda 37 en cada orientación): %+v", len(agregado), agregado)
	}
	// El orden es el de aparición, no el de un map: dos lotes iguales tienen que producir los
	// `update` en el mismo orden o dos transacciones concurrentes se interbloquean.
	if agregado[0].Orientacion != OrientacionHorizontal || agregado[0].Veces != 3 {
		t.Fatalf("la primera combinación es %+v, quiere horizontal ×3", agregado[0])
	}
	if agregado[1].Orientacion != OrientacionVertical || agregado[1].Veces != 1 {
		t.Fatalf("la segunda es %+v, quiere vertical ×1", agregado[1])
	}
}

// Un rol que no puede abrir la pantalla no puede tocarla (el espejo de `RequireRole`).
//
// Sin esto, cualquiera con un token propio reporta trescientos toques en una pantalla que un GET
// suyo recibiría con 403, y la rejilla con la que se decide un rediseño se llena de mentiras desde
// adentro. No es una fuga: es una medición que deja de servir para lo único que existe.
//
// Se prueba con un rol DESCONOCIDO sobre una pantalla instrumentada porque hoy las cuatro roles
// pueden abrir `pos`, que es la única de la lista: con un par (rol, pantalla) prohibido de verdad
// este test no tendría con qué fallar, y un test que no puede fallar no protege nada.
func TestElToqueDeUnRolQueNoPuedeAbrirLaPantallaNoCuenta(t *testing.T) {
	agregado := PreAgregarToques([]Toque{
		{Pantalla: "pos", Celda: 0, Orientacion: OrientacionHorizontal},
	}, Role("cocina"))
	if len(agregado) != 0 {
		t.Fatalf("se contó el toque de un rol que no puede abrir esa pantalla: %+v", agregado)
	}
}

func TestRecortarLoteDeToques(t *testing.T) {
	lote := make([]Toque, MaxToquesPorLote+40)
	if n := len(RecortarLoteDeToques(lote)); n != MaxToquesPorLote {
		t.Fatalf("el lote quedó en %d y el tope es %d", n, MaxToquesPorLote)
	}
	corto := make([]Toque, 3)
	if n := len(RecortarLoteDeToques(corto)); n != 3 {
		t.Fatalf("un lote corto se recortó a %d", n)
	}
}
