//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// La tarjeta de un grupo tiene que decir QUÉ opciones tiene, no solo cuántas: con 149 grupos, un
// "12 opciones" obliga a expandirlos uno por uno para dar con el que se busca.
//
// Se topa a 4 porque en una tarjeta de 7" el quinto nombre ya no cabe en el renglón, y el resto se
// resume con "y N más" — de ahí que el conteo y la vista previa tengan que contar LO MISMO.
func TestLaTarjetaDelGrupoMuestraSusPrimerasOpciones(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewAdminService(st)

	gid, err := svc.CreateGroup(ctx, "Salsas vista previa", 0, 3)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	// Se crean en desorden alfabético a propósito: la vista previa sigue el ORDEN DEL GRUPO
	// (sort_key), que es el que el operador acomodó, no el alfabético.
	for _, n := range []string{"Rance", "BBQ", "Búfalo", "Chipotle", "Mango habanero", "Ajo"} {
		if _, err := svc.CreateOption(ctx, gid, n, decimal.Zero, 1); err != nil {
			t.Fatalf("CreateOption(%s): %v", n, err)
		}
	}

	page, err := svc.ListGroups(ctx, "", "vista previa", "name", "asc", 10, 0)
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	var g *app.GroupView
	for i := range page.Items {
		if page.Items[i].ID == gid {
			g = &page.Items[i]
		}
	}
	if g == nil {
		t.Fatal("el grupo no salió en la lista")
	}
	if g.OptionCount != 6 {
		t.Fatalf("optionCount = %d, quiere 6", g.OptionCount)
	}
	if g.OptionPreview == "" {
		t.Fatal("la tarjeta no trae vista previa: vuelve a obligar a expandir grupo por grupo")
	}
	// Cuatro nombres, ni uno más: el quinto no cabe y el conteo es el que dice cuántos faltan.
	if n := len(strings.Split(g.OptionPreview, " · ")); n != 4 {
		t.Errorf("la vista previa trae %d nombres (%q), quiere 4", n, g.OptionPreview)
	}
	// Y son las PRIMERAS del grupo, no unas cualesquiera.
	if !strings.HasPrefix(g.OptionPreview, "Rance") {
		t.Errorf("la vista previa arranca con %q, quiere la primera del grupo (Rance)", g.OptionPreview)
	}
}

// Una opción archivada sale del POS, así que tampoco puede salir en la vista previa: el operador
// leería la tarjeta y creería que ese grupo todavía ofrece algo que ya quitó.
func TestLaVistaPreviaNoMuestraArchivadas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewAdminService(st)

	gid, err := svc.CreateGroup(ctx, "Salsas archivadas", 0, 2)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	oid, err := svc.CreateOption(ctx, gid, "Salsa retirada", decimal.Zero, 1)
	if err != nil {
		t.Fatalf("CreateOption: %v", err)
	}
	if _, err := svc.CreateOption(ctx, gid, "Salsa vigente", decimal.Zero, 1); err != nil {
		t.Fatalf("CreateOption: %v", err)
	}
	if err := svc.SetOptionActive(ctx, oid, false); err != nil {
		t.Fatalf("SetOptionActive: %v", err)
	}

	page, err := svc.ListGroups(ctx, "", "archivadas", "name", "asc", 10, 0)
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	for _, g := range page.Items {
		if g.ID != gid {
			continue
		}
		if strings.Contains(g.OptionPreview, "retirada") {
			t.Errorf("la vista previa muestra una opción archivada: %q", g.OptionPreview)
		}
		if g.OptionCount != 1 {
			t.Errorf("optionCount = %d, quiere 1: el conteo y la vista previa cuentan lo mismo", g.OptionCount)
		}
		return
	}
	t.Fatal("el grupo no salió en la lista")
}

// UN GRUPO SIN OPCIONES ACTIVAS NO PUEDE TUMBAR LA PANTALLA ENTERA.
//
// Defecto encontrado en producción el 2026-09-11: `/catalogo/opciones` respondía 500 y el log decía
// `cannot scan NULL into *string` en la columna `option_preview`. La causa es que `string_agg` sobre
// un conjunto VACÍO devuelve NULL —el `::text` no lo cambia, un `NULL::text` sigue siendo NULL— y
// sqlc tipa esa columna como `string` no nulable, así que el scan revienta.
//
// Lo que lo hace grave no es el NULL sino el alcance: no se rompe ESE renglón, se rompe la consulta
// completa. Un solo grupo sin opciones activas —uno recién creado, o uno al que le archivaron la
// última— deja sin pantalla a todo el catálogo de modificadores. Medido en el ambiente de pruebas:
// la pestaña «Activos» respondía 200 y «Inactivos» y «Todos» daban 500.
//
// El caso es ordinario, no exótico: crear el grupo y todavía no agregarle opciones es el primer
// paso de darlo de alta.
func TestUnGrupoSinOpcionesActivasNoTumbaLaLista(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := app.NewAdminService(st)

	// Recién creado, sin una sola opción: el estado en el que queda cualquier grupo nuevo.
	vacio, err := svc.CreateGroup(ctx, "Salsas sin opciones", 0, 2)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	// Y el otro camino al mismo estado: tenía opciones y se archivaron todas.
	archivado, err := svc.CreateGroup(ctx, "Salsas todas archivadas", 0, 2)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	oid, err := svc.CreateOption(ctx, archivado, "La única que había", decimal.Zero, 1)
	if err != nil {
		t.Fatalf("CreateOption: %v", err)
	}
	if err := svc.SetOptionActive(ctx, oid, false); err != nil {
		t.Fatalf("SetOptionActive: %v", err)
	}

	// Las tres pestañas de la pantalla, porque el defecto no se veía en la primera.
	for _, status := range []string{"", "act", "inact"} {
		page, err := svc.ListGroups(ctx, status, "Salsas", "name", "asc", 50, 0)
		if err != nil {
			t.Fatalf("ListGroups(status=%q) falló con un grupo sin opciones activas: %v", status, err)
		}
		for _, g := range page.Items {
			if g.ID != vacio && g.ID != archivado {
				continue
			}
			if g.OptionPreview != "" {
				t.Errorf("el grupo %q no tiene opciones activas y su vista previa dice %q", g.Name, g.OptionPreview)
			}
			if g.OptionCount != 0 {
				t.Errorf("el grupo %q dice %d opciones y no tiene ninguna activa", g.Name, g.OptionCount)
			}
		}
	}
}
