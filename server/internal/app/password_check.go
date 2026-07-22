package app

import (
	"context"
	"fmt"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/hibp"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
)

// checkPasswordStrength aplica la política local (domain.ValidatePassword) y, si está activo,
// HIBP (k-anonymity). Fail-open ante fallo de red de HIBP: no bloquea (un POS puede quedar sin
// internet) pero registra el evento. Una contraseña filtrada SÍ se rechaza. Compartida por el
// alta/reset de usuarios y por el flujo de recuperación.
func checkPasswordStrength(ctx context.Context, pw string, hc *hibp.Client, enabled bool) error {
	if err := domain.ValidatePassword(pw); err != nil {
		return err
	}
	if !enabled || hc == nil {
		return nil
	}
	pwned, err := hc.Pwned(ctx, pw)
	if err != nil {
		logging.SecurityEvent(ctx, "hibp_check_failed", "error", err.Error())
		return nil // fail-open
	}
	if pwned {
		return fmt.Errorf("%w: apareció en una filtración de datos conocida, elige otra", domain.ErrWeakPassword)
	}
	return nil
}
