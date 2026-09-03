//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// LEER EL VENCIMIENTO DE LA ESTACIÓN Y REVOCARLO TIENEN QUE SER LA MISMA SENTENCIA.
//
// El relevo LEÍA el vencimiento con un select, insertaba la sesión nueva y recién entonces revocaba
// la vieja con un update `:exec`, cuyo número de filas nadie miraba. En READ COMMITTED, dos
// peticiones con la misma cookie ven las dos el token vivo en la lectura, las dos insertan, y el
// update de la perdedora toca cero filas sin error: de un refresh salen dos vivos. Es el estado que
// motivó la feature —un usuario de producción con 4 sesiones vivas— y además cerrar sesión revoca
// una y deja la otra. El camino de /auth/refresh ya lo cerraba con `RevokeRefreshTokenIfActive` y
// su chequeo de filas; el del relevo nació sin él.
//
// El test es DETERMINISTA a propósito: dos goroutines no sirven de prueba porque el bcrypt del PIN
// las serializa en una caja con pocos núcleos y el verde saldría por el número de CPUs, no por el
// código. Lo que se prueba es el mecanismo: tomar la estación dos veces seguidas tiene que fallar la
// segunda. Con un select y un update separados, las dos lecturas tendrían éxito.
func TestTomarLaEstacionDosVecesSoloFuncionaLaPrimera(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	ana := makeUser(t, st, "ana_toma", "cajero")
	token, hash, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken: %v", err)
	}
	_ = token
	if _, err := st.Q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{
		UserID: ana, TokenHash: hash, ExpiresAt: clock().Add(8 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	if _, err := st.Q.TomarSesionDeEstacion(ctx, db.TomarSesionDeEstacionParams{
		TokenHash: hash, ExpiresAt: clock(), UserID: ana,
	}); err != nil {
		t.Fatalf("la primera toma debía funcionar: %v", err)
	}

	_, err = st.Q.TomarSesionDeEstacion(ctx, db.TomarSesionDeEstacionParams{
		TokenHash: hash, ExpiresAt: clock(), UserID: ana,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("la segunda toma del mismo token devolvió %v: leer y revocar no son la misma sentencia, así que dos relevos simultáneos acuñan dos credenciales de una", err)
	}
}

// EL REFRESH QUE SE PRESENTA TIENE QUE SER DEL QUE ESTÁ OPERANDO LA ESTACIÓN.
//
// El relevo aceptaba CUALQUIER token vivo de la empresa: bastaba presentar el de otra estación para
// heredar su reloj y, de paso, revocárselo. La cookie es HttpOnly, `SameSite=Strict` y con `Path`
// acotado, así que desde el navegador no se llega; pero un token que se filtre por otro lado —un
// respaldo, un log— no debería servir para tomar la estación de alguien más.
func TestNoSeTomaLaEstacionConElRefreshDeOtro(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	svc := app.NewAuthService(st, jm, clock)
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")

	ana := makeUser(t, st, "ana_ajena", "cajero")
	beto := makeUser(t, st, "beto_ajena", "cajero")
	luis := makeUser(t, st, "luis_ajena", "cajero")
	hash, err := auth.HashSecret("Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	for _, u := range []int64{ana, beto} {
		if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, u, hash); err != nil {
			t.Fatalf("set password: %v", err)
		}
	}
	if err := users.SetPIN(ctx, luis, "4827"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}

	deBeto, err := svc.Login(ctx, "beto_ajena", "gatobobah", "Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Ana es quien opera la estación, pero presenta el refresh de Beto.
	if _, err := svc.PinSwitchEnEstacion(ctx, luis, "4827", ana, deBeto.RefreshToken); err == nil {
		t.Error("se tomó la estación con el refresh de otra persona: hereda su reloj y se lo revoca")
	}

	// Y la sesión de Beto no se tocó.
	if _, err := svc.Refresh(ctx, defaultCompanyID, deBeto.RefreshToken); err != nil {
		t.Errorf("el intento fallido revocó la sesión de Beto: %v", err)
	}
}
