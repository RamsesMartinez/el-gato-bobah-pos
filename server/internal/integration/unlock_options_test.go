//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

func conPIN(t *testing.T, st *store.Store, userID int64, pin string) {
	t.Helper()
	users := app.NewUsersService(st, nil, false) // HIBP off en test
	if err := users.SetPIN(context.Background(), userID, pin); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}
}

// La rejilla de la pantalla de bloqueo. Se pinta en un mostrador a la vista del público, así que
// solo puede llevar lo mínimo para tocar un nombre.
func TestLaRejillaDeDesbloqueoSoloTraeLoMinimo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	svc := app.NewAuthService(st, jm, clock)

	conPin := makeUser(t, st, "ana_rejilla", "cajero")
	conPIN(t, st, conPin, "4827")
	// Sin PIN: tocarlo no la dejaría entrar, así que ofrecerlo sería mandarla a un callejón.
	makeUser(t, st, "luis_sinpin", "cajero")

	opciones, err := svc.UnlockOptions(ctx)
	if err != nil {
		t.Fatalf("UnlockOptions: %v", err)
	}
	nombres := map[string]bool{}
	for _, u := range opciones.Users {
		nombres[u.Name] = true
	}
	if !nombres["Test ana_rejilla"] {
		t.Error("quien tiene PIN no salió en la rejilla")
	}
	if nombres["Test luis_sinpin"] {
		t.Error("salió alguien SIN PIN: tocarlo no lo dejaría entrar")
	}
}

// Con solo-PIN la lista viaja VACÍA. Si listara nombres, el modo perdería su única ventaja —el tap
// que ahorra— y expondría la plantilla del negocio sin necesidad.
func TestConSoloPinLaRejillaVaVacia(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	svc := app.NewAuthService(st, jm, clock)
	settings := app.NewSettingsService(st)
	admin := makeUser(t, st, "admin_rejilla", "admin")

	u := makeUser(t, st, "ana_solopin", "cajero")
	conPIN(t, st, u, "482715")

	antes, _ := settings.Get(ctx)
	info := domain.BusinessInfo{Name: antes.BusinessName, Address: antes.Address, Phone: antes.Phone}
	ident := domain.DefaultIdentity()
	ident.PinOnlyUnlock = true
	if _, err := settings.SetBusinessInfo(ctx, info, domain.PrintSettings{}, ident, antes.Timezone, admin); err != nil {
		t.Fatalf("encender solo-PIN: %v", err)
	}

	opciones, err := svc.UnlockOptions(ctx)
	if err != nil {
		t.Fatalf("UnlockOptions: %v", err)
	}
	if !opciones.PinOnly {
		t.Error("el modo no se reportó como encendido")
	}
	if len(opciones.Users) != 0 {
		t.Errorf("con solo-PIN la lista trae %d nombres, quiere 0", len(opciones.Users))
	}
}

// FR-012. Hoy 2 de 8 usuarios activos no tienen PIN y no pueden quedar encerrados fuera por una
// funcionalidad que no eligieron. No basta con que no salgan en la rejilla: tienen que poder entrar.
func TestQuienNoTienePinSiguePudiendoEntrar(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	svc := app.NewAuthService(st, jm, clock)

	sinPin := makeUser(t, st, "sofia_sinpin", "cajero")
	hash, err := auth.HashSecret("Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, sinPin, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}

	if _, err := svc.Login(ctx, "sofia_sinpin", "gatobobah", "Contrasena-Larga-1!"); err != nil {
		t.Fatalf("quien no tiene PIN debe poder entrar con sus credenciales: %v", err)
	}
}
