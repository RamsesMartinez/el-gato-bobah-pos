package domain

import (
	"errors"
	"fmt"
)

// Sentinel domain errors. The HTTP layer maps these to status codes + error codes.
var (
	ErrNotFound           = errors.New("no encontrado")
	ErrUnauthorized       = errors.New("no autenticado")
	ErrForbidden          = errors.New("sin permisos")
	ErrInvalidCredentials = errors.New("credenciales inválidas")
	ErrValidation         = errors.New("datos inválidos")
	ErrConflict           = errors.New("conflicto")
	// ErrDuplicateName: ya existe un producto (por empresa) con ese nombre. Envuelve ErrConflict
	// para heredar el 409 y a la vez dar un mensaje accionable en el alta/edición/duplicado.
	ErrDuplicateName   = fmt.Errorf("ya existe un producto con ese nombre (%w)", ErrConflict)
	ErrTooManyRequests = errors.New("demasiados intentos, espera un momento")
	// ErrWeakPassword: la contraseña no cumple la política. Se envuelve con el motivo concreto
	// (longitud / común / filtrada) para que el mensaje llegue al usuario (422).
	ErrWeakPassword = errors.New("contraseña insegura")
	// ErrResetInvalid: el enlace/token de recuperación no sirve (inexistente, ya usado o vencido).
	// Distinto de ErrInvalidCredentials para dar un mensaje accionable en la pantalla de reset.
	ErrResetInvalid = errors.New("el enlace de recuperación es inválido o expiró; solicita uno nuevo")
)
