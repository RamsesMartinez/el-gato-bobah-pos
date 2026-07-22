package domain

import "regexp"

// Company es un tenant. El slug es la parte derecha del login username@slug y particiona todos
// los datos; debe ser único (lo garantiza el índice en BD) y estable-ish (el admin puede
// cambiarlo, pero rompe los logins guardados de sus empleados).
type Company struct {
	ID       int64  `json:"id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	IsActive bool   `json:"isActive"`
}

// slugRe: 2–40 chars, minúsculas/dígitos/guiones, sin guion al inicio/fin. Simple y URL/DNS-safe
// para poder usar el slug en subdominios o rutas a futuro.
var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])$`)

// ValidSlug reporta si s es un slug de empresa válido.
func ValidSlug(s string) bool { return slugRe.MatchString(s) }
