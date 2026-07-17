package domain

import "errors"

// Sentinel domain errors. The HTTP layer maps these to status codes + error codes.
var (
	ErrNotFound           = errors.New("no encontrado")
	ErrUnauthorized       = errors.New("no autenticado")
	ErrForbidden          = errors.New("sin permisos")
	ErrInvalidCredentials = errors.New("credenciales inválidas")
	ErrValidation         = errors.New("datos inválidos")
	ErrConflict           = errors.New("conflicto")
)
