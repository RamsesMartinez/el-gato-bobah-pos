package logging

import (
	"context"
	"log/slog"
)

// SecurityEvent registra un evento relevante para detección de intrusiones (login
// fallido, 403, lockout, reuso de refresh…) con una clave estable para poder alertar o
// grepear aparte del ruido de requests:
//
//	msg="security" security_event="<name>" ...kv
//
// Nivel Warn a propósito: no es un error del sistema, pero un operador sí debe poder
// verlo y filtrarlo. NUNCA se le pasan secretos (tokens, passwords, PINs); solo ids y
// metadatos suficientes para correlacionar con los logs de request por X-Request-Id.
func SecurityEvent(ctx context.Context, event string, args ...any) {
	slog.Default().WarnContext(ctx, "security", append([]any{"security_event", event}, args...)...)
}
