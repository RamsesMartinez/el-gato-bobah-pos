package domain

import (
	"slices"
	"strings"
	"time"
	"unicode"
)

type Role string

const (
	RoleAdmin   Role = "admin"
	RoleGerente Role = "gerente"
	RoleCajero  Role = "cajero"
	RoleMesero  Role = "mesero"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleGerente, RoleCajero, RoleMesero:
		return true
	}
	return false
}

// In reports whether the role is one of the allowed roles.
func (r Role) In(roles ...Role) bool {
	return slices.Contains(roles, r)
}

type User struct {
	ID                 int64     `json:"id"`
	CompanyID          int64     `json:"companyId"`
	CompanySlug        string    `json:"companySlug,omitempty"` // se rellena al emitir sesión (no vive en la fila users)
	Name               string    `json:"name"`
	Username           *string   `json:"username,omitempty"`
	Role               Role      `json:"role"`
	IsActive           bool      `json:"isActive"`
	RecoveryEmail      *string   `json:"recoveryEmail,omitempty"`
	MustChangePassword bool      `json:"mustChangePassword"`
	CreatedAt          time.Time `json:"createdAt"`
}

// MaxUsuario acota lo que se acepta como usuario o como slug de empresa en la frontera.
//
// No es un límite de la base —las columnas son citext, sin tope— sino de lo que puede ser un
// nombre de verdad: el más largo en producción mide 14 caracteres. Existe porque lo que llega en el
// cuerpo viaja a dos lugares que no esperan un megabyte: la llave del limitador, que vive en un
// Redis de 128 MB con desalojo LRU —llenarlo tira los contadores del POS y el limitador falla
// ABIERTO—, y el evento de seguridad, que rota a 1 MB: once peticiones enormes se llevan toda la
// bitácora de intentos fallidos anteriores.
const MaxUsuario = 64

// NormalizarUsuario recorta y baja a minúsculas un usuario o un slug.
//
// Las columnas son citext, así que "admin" y "ADMIN" YA son el mismo usuario para autenticar. Lo
// que no es igual es lo que se arma con esa cadena: sin normalizar, cada grafía abre su propio
// contador en el limitador y el bloqueo por cuenta deja de morder — `admin` × `gatobobah` da 16,384
// llaves distintas para la misma credencial. Y citext tampoco ignora espacios: sin recortar,
// "soporte " y "soporte" son dos filas en una tabla cuyo índice único dice que no pueden serlo.
func NormalizarUsuario(u string) string {
	return strings.ToLower(strings.TrimSpace(u))
}

// UsuarioValido rechaza lo que no puede ser un usuario ni un slug.
//
// El '@' se prohíbe porque es el separador de usuario@empresa del login: un usuario que lo lleve
// dentro se teclea en la pantalla equivocada y enturbia la bitácora de accesos fallidos, que es
// donde se mira cuando algo va mal.
//
// Quien la use en un login tiene que rechazar EXACTAMENTE como se rechaza una contraseña
// equivocada —mismo código y misma latencia, con el bcrypt de descarte corrido igual—; si no, la
// forma del usuario se vuelve un oráculo de cuáles existen.
func UsuarioValido(u string) bool {
	u = NormalizarUsuario(u)
	if u == "" || len(u) > MaxUsuario {
		return false
	}
	for _, r := range u {
		if unicode.IsSpace(r) || r == '@' {
			return false
		}
	}
	return true
}
