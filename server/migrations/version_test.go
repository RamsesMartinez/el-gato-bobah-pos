package migrations

import (
	"strings"
	"testing"
)

// La consola de plataforma muestra en qué versión de esquema está la instalación, y ese número
// sale de AQUÍ y no de `goose_db_version`, por una razón concreta: el rol de la consola solo puede
// leer `companies` y `platform_operators`. Darle acceso a la bitácora de goose sería una cuarta
// excepción a la lista de "solo lo que necesita" para un número que el binario ya trae dentro.
//
// La equivalencia se sostiene porque la API migra al arrancar y NO sirve si eso falla: un binario
// que está atendiendo requests ya aplicó todas sus migraciones.
func TestVersionMaxima(t *testing.T) {
	v, err := VersionMaxima()
	if err != nil {
		t.Fatalf("VersionMaxima: %v", err)
	}
	// El número exacto se mueve con cada migración; lo que no puede pasar es que salga cero
	// —significaría que no se leyó ningún archivo— ni que se quede en un valor viejo porque el
	// parseo tomó el primero en vez del mayor.
	if v < 68 {
		t.Fatalf("VersionMaxima() = %d: el repo tiene al menos la migración 68, así que se está leyendo mal el directorio embebido", v)
	}
}

// DOS MIGRACIONES CON EL MISMO NÚMERO NO LLEGAN A DEVELOP. Pasó el 2026-11-23: dos ramas en paralelo
// (el menú público y los traspasos de almacén) se mergearon cada una con su 0123, y goose se niega a
// migrar con una versión repetida: la API nueva no arranca. La regla de AGENTS.md («la que llegue
// segunda renumera») dependía de acordarse; esto falla en el CI del PR.
func TestEveryMigrationVersionIsUnique(t *testing.T) {
	entries, err := FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, e := range entries {
		name := e.Name()
		if len(name) < 5 || name[4] != '_' || !strings.HasSuffix(name, ".sql") {
			continue
		}
		if prev, ok := seen[name[:4]]; ok {
			t.Errorf("la versión %s está dos veces: %s y %s. Renumera la que llegó después a la siguiente libre", name[:4], prev, name)
		}
		seen[name[:4]] = name
	}
}
