package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// El identificador con el que una plataforma de reparto nombra a un pedido.
//
// Es TEXTO libre y se guarda tal cual: Uber usa un UUID de 36 caracteres, DiDi un entero de 19
// dígitos y Rappi uno de 10. Cualquier normalización más allá de recortar los extremos —mayúsculas,
// guiones, longitud— destruye lo único que sirve para encontrar el pedido en el documento de pago.

// MaxPlatformRefLen es cota de cordura, no regla de negocio: el formato más largo que se conoce son
// los 36 caracteres del UUID de Uber. Existe contra un pegado accidental de media pantalla, y se
// cuenta en CARACTERES y no en bytes para decir lo mismo que el check de Postgres, que usa
// `length()`. Con `len()` un folio con acentos se rechazaría aquí y pasaría allá.
const MaxPlatformRefLen = 64

// NormalizePlatformRef recorta los espacios de los extremos y rechaza lo que no es un folio.
//
// La cadena vacía se rechaza en vez de convertirse en "sin folio": guardar "" haría que dos pedidos
// sin folio chocaran contra el índice único —o peor, que pasaran los dos sin que nadie note que uno
// está vacío—. La ausencia se representa como ausencia, que aquí es no llamar a esta función.
func NormalizePlatformRef(raw string) (string, error) {
	ref := strings.TrimSpace(raw)
	if ref == "" {
		return "", fmt.Errorf("%w: el folio de la plataforma no puede ir vacío", ErrValidation)
	}
	// Se mide DESPUÉS de recortar: medir antes rechazaría un folio que cabe, solo por venir pegado
	// con espacios del reporte.
	if n := utf8.RuneCountInString(ref); n > MaxPlatformRefLen {
		return "", fmt.Errorf("%w: el folio de la plataforma tiene %d caracteres y el máximo es %d",
			ErrValidation, n, MaxPlatformRefLen)
	}
	return ref, nil
}
