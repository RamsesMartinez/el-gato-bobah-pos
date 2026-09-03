//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// US3. Una tableta encendida el viernes no puede seguir autenticada el lunes.
//
// Antes de esto la sesión duraba 30 DÍAS, así que la tableta que alguien dejó abierta atribuía a
// esa persona todo lo que se cobrara durante un mes. El bloqueo por inactividad protege los
// minutos; esto protege los días.
func TestLaSesionCaducaAlTerminarElTurno(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_caduca", "admin")

	// Turno de 8 horas, que es el default del negocio.
	antes, _ := settings.Get(ctx)
	info := domain.BusinessInfo{Name: antes.BusinessName, Address: antes.Address, Phone: antes.Phone}
	ident := domain.DefaultIdentity()
	if _, err := settings.SetBusinessInfo(ctx, info, domain.PrintSettings{}, ident, antes.Timezone, admin); err != nil {
		t.Fatalf("ajustes: %v", err)
	}

	ahora := fixedNow
	reloj := func() time.Time { return ahora }
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", reloj)
	svc := app.NewAuthService(st, jm, reloj)

	ana := makeUser(t, st, "ana_caduca", "cajero")
	hash, err := auth.HashSecret("Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, ana, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}

	sesion, err := svc.Login(ctx, "ana_caduca", "gatobobah", "Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// A mitad del turno la sesión sigue viva: renovarla es el caso normal de todo el día.
	ahora = ahora.Add(4 * time.Hour)
	renovada, err := svc.Refresh(ctx, defaultCompanyID, sesion.RefreshToken)
	if err != nil {
		t.Fatalf("a las 4 horas la sesión debe seguir viva: %v", err)
	}

	// Pasado el turno, ya no. Y no basta el PIN: hace falta usuario y contraseña.
	ahora = ahora.Add(9 * time.Hour)
	if _, err := svc.Refresh(ctx, defaultCompanyID, renovada.RefreshToken); err == nil {
		t.Fatal("pasadas las 8 horas la sesión debe caducar y exigir credenciales completas")
	}
}

// El plazo sale del AJUSTE del negocio, no de una constante: un local con turnos de 12 horas lo
// sube, y uno que quiera más control lo baja. Sin esto, el ajuste sería decorativo.
func TestElPlazoDeLaSesionSaleDelAjusteDelNegocio(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_plazo", "admin")

	antes, _ := settings.Get(ctx)
	info := domain.BusinessInfo{Name: antes.BusinessName, Address: antes.Address, Phone: antes.Phone}
	ident := domain.DefaultIdentity()
	ident.SessionHours = 2
	if _, err := settings.SetBusinessInfo(ctx, info, domain.PrintSettings{}, ident, antes.Timezone, admin); err != nil {
		t.Fatalf("ajustes: %v", err)
	}

	ahora := fixedNow
	reloj := func() time.Time { return ahora }
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", reloj)
	svc := app.NewAuthService(st, jm, reloj)

	ana := makeUser(t, st, "ana_plazo", "cajero")
	hash, _ := auth.HashSecret("Contrasena-Larga-1!")
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, ana, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}

	sesion, err := svc.Login(ctx, "ana_plazo", "gatobobah", "Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	// Con el plazo en 2 horas, a las 3 ya caducó — con el default de 8 seguiría viva, que es
	// justo lo que este test distingue.
	ahora = ahora.Add(3 * time.Hour)
	if _, err := svc.Refresh(ctx, defaultCompanyID, sesion.RefreshToken); err == nil {
		t.Fatal("con el plazo en 2 horas la sesión debe caducar a las 3")
	}
}
