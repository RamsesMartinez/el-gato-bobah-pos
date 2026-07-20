package domain

import "time"

// RefreshVerdict clasifica un refresh token presentado en /auth/refresh.
type RefreshVerdict int

const (
	RefreshValid   RefreshVerdict = iota // rotar y emitir sesión nueva
	RefreshExpired                       // vencido sin revocar: expiración normal, re-login
	RefreshReused                        // revocado y reaparece: robo/reuso → revocar la familia
)

// ClassifyRefresh decide qué hacer con un refresh presentado. Se comprueba `revoked`
// primero: que reaparezca un token ya revocado significa que rotó (uso legítimo previo)
// y alguien reusó la copia vieja — o fue robado. No podemos distinguir al atacante del
// usuario, así que ese caso obliga a revocar toda la familia. Un token no revocado pero
// vencido es solo expiración (el usuario estuvo ausente), no un ataque.
func ClassifyRefresh(revoked bool, expiresAt, now time.Time) RefreshVerdict {
	if revoked {
		return RefreshReused
	}
	if expiresAt.Before(now) {
		return RefreshExpired
	}
	return RefreshValid
}
