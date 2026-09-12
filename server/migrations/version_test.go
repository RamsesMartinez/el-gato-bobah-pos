package migrations

import "testing"

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
