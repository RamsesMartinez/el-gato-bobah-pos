// Package secrets guarda secretos de terceros —credenciales de las plataformas, llaves de firma— de
// modo que un respaldo de la base no los revele.
//
// En producción y en el ambiente de pruebas cifra Cloud KMS: la llave nunca sale de Google, y solo
// la cuenta de servicio de SU ambiente puede usarla. En la máquina de quien programa cifra AES-GCM
// con una llave local, porque ahí no hay cuenta de servicio; config.Validate impide que la API de
// producción arranque con ese respaldo.
//
// Cada valor va ATADO a un contexto (AAD) — empresa, plataforma y tipo — que no se guarda con él
// sino que se exige al descifrar. Un valor copiado a la fila de otra empresa no descifra: es una
// segunda barrera además de RLS.
package secrets

import (
	"context"
	"errors"
)

var (
	// ErrUnreadable: el valor no se puede descifrar AQUÍ — lo cifró otro ambiente (un respaldo de
	// producción restaurado en local), otra empresa (AAD distinta) o está alterado. No es una falla
	// del sistema: es una credencial que hay que volver a capturar.
	ErrUnreadable = errors.New("secrets: the value cannot be decrypted in this environment")
	// ErrUnavailable: el servicio de llaves no respondió o negó el permiso. Distinto de
	// ErrUnreadable a propósito: confundirlos mandaría a recapturar una credencial que está bien.
	ErrUnavailable = errors.New("secrets: the key service is unavailable")
)

// El primer byte de cada cifrado dice quién lo hizo. Sin él, un valor de producción restaurado en
// local se intentaría descifrar con la llave local y el error diría «alterado» en vez de «es de otro
// ambiente»; con él se sabe sin viaje de red.
const (
	formatKMS   byte = 1
	formatLocal byte = 2
)

// Cipher es lo que tienen en común KMS y el respaldo local, y lo que envuelve la caché.
type Cipher interface {
	Encrypt(ctx context.Context, plaintext []byte, aad string) ([]byte, error)
	Decrypt(ctx context.Context, ciphertext []byte, aad string) ([]byte, error)
}
