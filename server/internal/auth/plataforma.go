package auth

import (
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// PlatformAccessTokenTTL es lo que vive un acceso de la consola.
//
// NO es el mecanismo de revocación: retirarle el acceso a un operador tiene que morder en el
// siguiente request, no en quince minutos, así que el middleware de la consola vuelve a leer si
// sigue activo en cada llamada. Esto es la segunda capa — un token robado deja de servir solo.
const PlatformAccessTokenTTL = 15 * time.Minute

// ClaimsDePlataforma es lo que viaja en un token de la consola: quién es y hasta cuándo.
//
// Sin empresa y sin rol de negocio, y eso es deliberado: la consola no tiene tenant, y un claim de
// empresa aquí sería el primer paso para que alguien la trate como una superficie más del POS.
type ClaimsDePlataforma struct {
	OperadorID int64  `json:"oid"`
	Username   string `json:"usr"`
	Name       string `json:"name"`
	jwt.RegisteredClaims
}

// ManagerDePlataforma firma y valida las sesiones de la consola.
//
// Es un tipo aparte del Manager del negocio, no el mismo con otro secreto, porque las dos cosas
// que emite son distintas —un usuario de una empresa y un operador sin empresa— y porque así
// ningún handler puede pasarle por error el manager equivocado: no compilaría.
type ManagerDePlataforma struct {
	secret []byte
	now    func() time.Time
}

func NewManagerDePlataforma(secret string, now func() time.Time) *ManagerDePlataforma {
	if now == nil {
		now = time.Now
	}
	return &ManagerDePlataforma{secret: []byte(secret), now: now}
}

// Issue firma un acceso corto para un operador de plataforma.
func (m *ManagerDePlataforma) Issue(o domain.Operador) (string, error) {
	now := m.now()
	claims := ClaimsDePlataforma{
		OperadorID: o.ID,
		Username:   o.Username,
		Name:       o.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(o.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(PlatformAccessTokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse verifica la firma con el secreto de la CONSOLA y devuelve los claims.
func (m *ManagerDePlataforma) Parse(token string) (*ClaimsDePlataforma, error) {
	c := &ClaimsDePlataforma{}
	_, err := jwt.ParseWithClaims(token, c, func(t *jwt.Token) (any, error) {
		// Sin esto, un token con alg "none" —o con RS256 y la llave pública como secreto— se
		// aceptaría. Es el mismo cierre que ya hace el manager del negocio.
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	return c, nil
}
