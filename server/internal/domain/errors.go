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
	// ErrPaymentNeedsRegister: se pagó con un método que mueve el cajón sin decir de qué caja
	// salió. Es obligatorio y no opcional: efectivo que sale sin movimiento de caja descuadra el
	// corte, y el descuadre se descubre horas después sin saber de dónde vino.
	ErrPaymentNeedsRegister = fmt.Errorf("%w: un pago en efectivo debe indicar la caja de la que sale", ErrValidation)
	// ErrPaymentsBelowAmount: se intentó dar por pagado un gasto con pagos que no cubren el
	// importe. Envuelve ErrValidation para llegar como 4xx con mensaje accionable.
	ErrPaymentsBelowAmount = fmt.Errorf("%w: los pagos registrados no cubren el importe del gasto", ErrValidation)
	// ErrNoOpenRegister: se quiso cobrar sin la caja principal abierta. No es una validación de
	// forma sino una regla del negocio: una venta cobrada fuera de un arqueo es dinero que el corte
	// no ve, y el faltante se descubre al cerrar sin manera de reconstruir de dónde salió.
	// Solo la caja PRINCIPAL habilita el cobro — las secundarias existen para traspasos y gastos.
	ErrNoOpenRegister = errors.New("no hay una caja abierta: abre el turno antes de cobrar")
	// ErrInvalidTimezone: la zona horaria capturada no es un nombre IANA real. Se rechaza al
	// GUARDAR y no al usar: donde se usa está el camino de una venta, que cae a UTC antes que
	// tumbar un cobro, y sin este rechazo ese fallback correría las fechas en silencio.
	ErrInvalidTimezone = fmt.Errorf("%w: la zona horaria no existe (usa un nombre como America/Mexico_City)", ErrValidation)
	// ErrPlatformNotFound: la plataforma de reparto que mandó el cliente no es de esta empresa.
	// Se resuelve bajo RLS y se rechaza: los chequeos de llave foránea de Postgres saltan RLS, así
	// que un id ajeno pasaría y —si el código cayera a margen 0— la venta se cobraría a precio de
	// mostrador en Uber, con el ticket bien impreso y el descuadre apareciendo semanas después al
	// conciliar el depósito.
	ErrPlatformNotFound = errors.New("esa plataforma de reparto no existe en este negocio")
)
