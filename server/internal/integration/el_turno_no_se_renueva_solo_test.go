//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
)

// EL DEFECTO QUE LA FEATURE VINO A CERRAR, VIVO EN LA OTRA PUERTA.
//
// Se corrigió en el relevo por PIN y quedó igual en `/auth/refresh`: cada rotación acuñaba un plazo
// COMPLETO nuevo, así que el turno se corría hacia adelante solo. Y no hace falta atacante para
// dispararlo: el front refresca al volver el foco a la ventana, así que una tableta que alguien
// toca cada rato no caduca nunca y `session_hours` es decorativo.
//
// El test refresca CADA HORA durante el doble del turno. Con el defecto, la sesión sigue viva al
// final; sin él muere al cumplirse las ocho horas del login, sin importar cuántas veces se rotara.
//
// El test que ya existía —refrescar a las 4 h y verificar a las 13 h— no lo atrapaba: con el
// defecto la renovada vencía a las 12 h y a las 13 h ya estaba muerta igual. Pasaba verde en los
// dos mundos.
func TestRefrescarCadaHoraNoAlargaElTurno(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	ahora := fixedNow
	reloj := func() time.Time { return ahora }
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", reloj)
	svc := app.NewAuthService(st, jm, reloj)

	ana := makeUser(t, st, "ana_no_renueva", "cajero")
	hash, err := auth.HashSecret("Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("HashSecret: %v", err)
	}
	if _, err := st.Pool.Exec(ctx, `update users set password_hash = $2 where id = $1`, ana, hash); err != nil {
		t.Fatalf("set password: %v", err)
	}

	sesion, err := svc.Login(ctx, "ana_no_renueva", "gatobobah", "Contrasena-Larga-1!")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	entro := ahora
	token := sesion.RefreshToken

	// El turno del default son 8 horas, así que el fin es este instante y no se mueve más.
	fin := entro.Add(8 * time.Hour)

	// Se rota cada hora hasta el doble del turno.
	for h := 1; h <= 16; h++ {
		ahora = entro.Add(time.Duration(h) * time.Hour)
		nueva, err := svc.Refresh(ctx, defaultCompanyID, token)
		if err != nil {
			if ahora.Before(fin) {
				t.Fatalf("a la hora %d la sesión debía seguir viva: %v", h, err)
			}
			return // murió al cumplirse el turno, que es lo que se pide
		}
		token = nueva.RefreshToken
		if v := venceDe(t, st, token); !v.Equal(fin) {
			t.Fatalf("a la hora %d la rotación corrió el fin del turno de %s a %s: el front rota solo al volver el foco, así que la tableta que alguien toca cada rato no caducaría nunca",
				h, fin.Format(time.RFC3339), v.Format(time.RFC3339))
		}
	}
	t.Fatalf("al doble del turno la sesión seguía viva: rotar el refresh nunca la deja morir")
}
