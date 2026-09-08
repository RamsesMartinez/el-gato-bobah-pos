package domain

import (
	"fmt"
	"strings"
	"unicode"
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
	// Un carácter de control —el byte NUL, sobre todo— se rechaza AQUÍ y no en Postgres. Sin esto,
	// `{"platformOrderRef":"AB\u0000C"}` pasa la validación (TrimSpace no lo quita, no queda vacío y
	// son 4 runas) y revienta en el driver con `invalid byte sequence for encoding "UTF8"`, que sale
	// como 500. El principio V pide 400 para la entrada absurda: un 500 dice "el servidor se rompió"
	// y manda a revisar logs por un dato que el cliente mandó mal.
	if i := strings.IndexFunc(ref, esDeControl); i >= 0 {
		return "", fmt.Errorf("%w: el folio de la plataforma trae un carácter que no se puede guardar",
			ErrValidation)
	}
	// Se mide DESPUÉS de recortar: medir antes rechazaría un folio que cabe, solo por venir pegado
	// con espacios del reporte.
	if n := utf8.RuneCountInString(ref); n > MaxPlatformRefLen {
		return "", fmt.Errorf("%w: el folio de la plataforma tiene %d caracteres y el máximo es %d",
			ErrValidation, n, MaxPlatformRefLen)
	}
	return ref, nil
}

// esDeControl: cualquier carácter de control, no solo el NUL. Postgres rechaza el NUL y los demás
// no aportan nada a un identificador — llegan por un pegado sucio o por alguien probando.
func esDeControl(r rune) bool { return unicode.IsControl(r) }

// PlatformRefDelPedido decide si este pedido puede llevar folio, y lo normaliza.
//
// Vive en `domain` y no en el servicio porque es una REGLA pura —un pedido que no es de plataforma
// no lleva folio de plataforma— y las reglas se prueban sin base de datos. El check del esquema es
// la red de abajo, pero devolvería un 500 opaco en vez de un 400 que dice qué pasa.
//
// nil de entrada = no se mandó folio, que es una salida explícita y no un error.
func PlatformRefDelPedido(ref *string, plataforma *int16) (*string, error) {
	if ref == nil {
		return nil, nil
	}
	if plataforma == nil {
		return nil, fmt.Errorf("%w: un pedido que no es de plataforma no lleva folio de plataforma",
			ErrValidation)
	}
	limpio, err := NormalizePlatformRef(*ref)
	if err != nil {
		return nil, err
	}
	return &limpio, nil
}
