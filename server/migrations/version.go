package migrations

import (
	"fmt"
	"strconv"
	"strings"
)

// VersionMaxima devuelve el número de la última migración embebida en el binario.
//
// Es la versión de esquema de la instalación: la API corre `goose up` al arrancar y se apaga si
// falla, así que un binario que está sirviendo ya aplicó todas las suyas.
//
// Existe para que la consola de plataforma pueda mostrar ese dato SIN leer `goose_db_version`: su
// rol de base solo puede leer `companies` y `platform_operators`, y abrirle la bitácora de goose
// sería una excepción más a esa lista por un número que el binario ya trae dentro.
func VersionMaxima() (int64, error) {
	entradas, err := FS.ReadDir(".")
	if err != nil {
		return 0, fmt.Errorf("leer las migraciones embebidas: %w", err)
	}
	var max int64
	for _, e := range entradas {
		nombre := e.Name()
		if !strings.HasSuffix(nombre, ".sql") {
			continue
		}
		// goose nombra "NNNN_lo_que_hace.sql"; el número es todo lo que va antes del primer "_".
		num, _, ok := strings.Cut(nombre, "_")
		if !ok {
			continue
		}
		v, err := strconv.ParseInt(num, 10, 64)
		if err != nil {
			continue
		}
		if v > max {
			max = v
		}
	}
	if max == 0 {
		return 0, fmt.Errorf("ninguna migración embebida tiene número: %d archivos leídos", len(entradas))
	}
	return max, nil
}
