package domain

import (
	_ "embed"
	"fmt"
	"strings"
)

// MinPasswordLen sigue NIST 800-63B: prioriza longitud sobre reglas de composición. 12 es un
// piso robusto para un sistema con datos financieros; no forzamos rotación ni mayúsculas/símbolos
// (contraproducentes según NIST). El chequeo contra brechas (HIBP) se hace fuera de domain (I/O).
const MinPasswordLen = 12

// commonPasswords se embebe del archivo (una por línea, minúsculas). Es la red de seguridad
// OFFLINE cuando HIBP no está disponible (fail-open): rechaza las contraseñas más trilladas que
// igual superan el mínimo de longitud (p. ej. "contraseña123", "administrador1"). Embed =
// dato de compilación, no I/O en runtime, así domain sigue puro.
//
//go:embed common_passwords.txt
var commonPasswordsRaw string

var commonPasswords = func() map[string]struct{} {
	m := make(map[string]struct{})
	for line := range strings.SplitSeq(commonPasswordsRaw, "\n") {
		if s := strings.TrimSpace(line); s != "" && !strings.HasPrefix(s, "#") {
			m[strings.ToLower(s)] = struct{}{}
		}
	}
	return m
}()

// ValidatePassword aplica la política local (longitud + blocklist de comunes). Devuelve
// ErrWeakPassword envuelto con el motivo concreto para mostrarlo al usuario. NO consulta HIBP
// (eso es I/O y vive en internal/hibp); el servicio combina ambos.
func ValidatePassword(pw string) error {
	if len([]rune(pw)) < MinPasswordLen {
		return fmt.Errorf("%w: usa al menos %d caracteres", ErrWeakPassword, MinPasswordLen)
	}
	// '@' se reserva como separador de usuario@empresa en el login: prohibirlo en la contraseña
	// mantiene ese carácter inequívoco en toda la superficie de auth. Todos los demás permitidos.
	if strings.ContainsRune(pw, '@') {
		return fmt.Errorf("%w: no puede contener el carácter @ (reservado para usuario@empresa)", ErrWeakPassword)
	}
	if _, bad := commonPasswords[strings.ToLower(strings.TrimSpace(pw))]; bad {
		return fmt.Errorf("%w: es una contraseña demasiado común, elige otra", ErrWeakPassword)
	}
	return nil
}
