package domain

import (
	"crypto/sha256"
	"strconv"
	"uuid"
)

// DerivedUUID es un UUID determinista de las partes (versión 8 de RFC 9562). Las escrituras del MCP
// lo usan como id de la cuenta y como clientUuid del cobro, a partir de la empresa, el token y la
// clave de idempotencia: un reintento manda el mismo id, y la ruta lo reconoce aunque el registro de
// idempotencia se haya perdido. Cada parte va con su largo delante para que ("15","1x") y ("151","x")
// no compartan bytes.
func DerivedUUID(parts ...string) uuid.UUID {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(strconv.Itoa(len(p))))
		h.Write([]byte{':'})
		h.Write([]byte(p))
	}
	var u uuid.UUID
	copy(u[:], h.Sum(nil))
	u[6] = (u[6] & 0x0F) | 0x80
	u[8] = (u[8] & 0x3F) | 0x80
	return u
}
