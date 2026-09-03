package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxMotivo acota el texto con el que se justifica que una venta no ocurrió.
//
// No es defensa contra un atacante —el tope global del cuerpo ya está en un megabyte—: es que el
// motivo se lee en una pantalla de siete pulgadas y en el histórico del pedido. Doscientos
// caracteres son dos renglones largos, que es todo lo que alguien escribe con un cliente enfrente.
const MaxMotivo = 200

// MotivoValido recorta el motivo y lo rechaza si no queda nada.
//
// Existe porque el recorte vivía en UN solo camino: `refund` hacía `reason.trim()` en la pantalla y
// `cancel` solo comprobaba que la cadena no fuera vacía. Un espacio pasaba los dos lados y llegaba a
// la base, donde el `check` de 0007 lo da por bueno — y el histórico se queda con una cancelación
// sin motivo, que es justo lo que ese campo existe para impedir. Es el hermano que no se movió.
func MotivoValido(motivo string) (string, error) {
	m := strings.TrimSpace(motivo)
	if m == "" {
		return "", fmt.Errorf("%w: hace falta el motivo", ErrValidation)
	}
	if utf8.RuneCountInString(m) > MaxMotivo {
		return "", fmt.Errorf("%w: el motivo no puede pasar de %d caracteres", ErrValidation, MaxMotivo)
	}
	return m, nil
}
