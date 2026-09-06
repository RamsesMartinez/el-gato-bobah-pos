//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// UN ROBO EN UNA TABLETA NO PUEDE TUMBAR LA OTRA.
//
// La constitución dice que un `RefreshReused` revoca toda la FAMILIA. El código decía eso mismo en
// un comentario y hacía otra cosa: revocaba por `user_id`, o sea todas las sesiones de la persona.
// No había con qué hacerlo bien — nada en el esquema decía qué credenciales descienden de qué login.
//
// En este negocio dos estaciones comparten cuenta, así que la diferencia no es teórica: un robo
// detectado en la tableta de la barra dejaba a la caja pidiendo contraseña, con el cliente enfrente
// y sin que nadie entendiera por qué.
//
// El test sigue la cadena COMPLETA —entrar, rotar, reusar— porque el defecto solo aparece cuando la
// familia se hereda de verdad: si la rotación estrenara cadena, el reuso no tendría nada que cortar.
func TestElReusoRevocaSoloLaFamiliaComprometida(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	svc := app.NewAuthService(st, jm, clock)

	usuario := makeUser(t, st, "cajero_familias", "cajero")

	// Dos estaciones, la MISMA cuenta y cada una su propia cadena: la barra y la caja.
	vence := clock().Add(8 * time.Hour)
	barraToken := emitirRefreshDeFamilia(t, st, usuario, vence, uuid.New())
	cajaToken := emitirRefreshDeFamilia(t, st, usuario, vence, uuid.New())

	// La barra rota su credencial, como hace el front al volver el foco a la ventana. La rotada
	// tiene que HEREDAR la familia: si estrenara cadena, el reuso no tendría nada que cortar.
	rotada, err := svc.Refresh(ctx, defaultCompanyID, barraToken)
	if err != nil {
		t.Fatalf("rotar la de la barra: %v", err)
	}

	// Y ahora alguien presenta la credencial VIEJA de la barra: eso es reuso.
	if _, err := svc.Refresh(ctx, defaultCompanyID, barraToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("el reuso tenía que rechazarse con 401 y fue: %v", err)
	}

	// La cadena de la barra queda cortada, incluida la que acababa de rotar.
	if _, err := svc.Refresh(ctx, defaultCompanyID, rotada.RefreshToken); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("la credencial rotada de la barra sigue viva tras el reuso: el ladrón sigue dentro (%v)", err)
	}

	// Y LA CAJA SIGUE TRABAJANDO. Es lo que se rompía.
	if _, err := svc.Refresh(ctx, defaultCompanyID, cajaToken); err != nil {
		t.Errorf("un robo en la barra tumbó la sesión de la caja: %v — con el cliente enfrente y sin "+
			"que nadie entienda por qué", err)
	}
}

// Una credencial SIN familia —emitida antes de 0064— cae al castigo viejo en vez de quedarse sin
// ninguno. Sin linaje no se puede saber qué cortar, y no cortar nada dejaría al ladrón dentro.
func TestUnaCredencialSinFamiliaSigueRevocandoPorUsuario(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	svc := app.NewAuthService(st, jm, clock)

	usuario := makeUser(t, st, "cajero_sin_familia", "cajero")
	vieja := emitirRefresh(t, st, usuario, clock().Add(8*time.Hour))
	otra := emitirRefresh(t, st, usuario, clock().Add(8*time.Hour))
	if _, err := st.Pool.Exec(ctx,
		`update refresh_tokens set family_id = null where user_id = $1`, usuario); err != nil {
		t.Fatalf("dejarlas sin familia: %v", err)
	}
	// Revocada a mano: presentarla otra vez es, por definición, reuso.
	if _, err := st.Pool.Exec(ctx,
		`update refresh_tokens set revoked_at = now() where token_hash = $1`,
		auth.HashToken(vieja)); err != nil {
		t.Fatalf("revocar: %v", err)
	}

	if _, err := svc.Refresh(ctx, defaultCompanyID, vieja); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("el reuso tenía que rechazarse y fue: %v", err)
	}
	if _, err := svc.Refresh(ctx, defaultCompanyID, otra); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("sin familia el castigo tiene que ser el viejo —revocar por usuario— y la otra "+
			"credencial siguió viva: el ladrón se queda dentro (%v)", err)
	}
}

// emitirRefreshDeFamilia siembra una credencial dentro de una cadena dada, que es lo que deja armar
// las dos estaciones sin pasar por el login.
func emitirRefreshDeFamilia(t *testing.T, st *store.Store, userID int64, vence time.Time, familia uuid.UUID) string {
	t.Helper()
	token, hash, err := auth.NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken: %v", err)
	}
	if _, err := st.Pool.Exec(context.Background(),
		`insert into refresh_tokens (company_id, user_id, token_hash, expires_at, family_id)
		 values ($1, $2, $3, $4, $5)`, defaultCompanyID, userID, hash, vence, familia); err != nil {
		t.Fatalf("sembrar refresh: %v", err)
	}
	return token
}
