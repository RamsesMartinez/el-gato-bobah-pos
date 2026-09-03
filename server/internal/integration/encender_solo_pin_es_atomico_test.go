//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// ENCENDER EL MODO Y BORRAR LOS PINs TIENEN QUE SER LA MISMA OPERACIÓN.
//
// Eran dos autocommits seguidos, y el reintento NO reparaba: al segundo intento el ajuste ya decía
// `pin_only_unlock = true`, así que `encendiendo` daba falso y el borrado no volvía a correr nunca.
// El negocio quedaba en modo de solo-PIN —donde el PIN ES la identidad y se prueba contra toda la
// plantilla a la vez— con los PINs de cuatro dígitos de antes, que además ya traen su huella de
// búsqueda porque se calcula siempre que hay secreto. Espacio de 10,000, y si cae el del admin
// quien ataca recibe rol de admin. Nada en la pantalla lo delataba.
//
// Se prueba por el lado que se puede provocar: si el borrado no se puede hacer, el ajuste NO puede
// quedar encendido. Aquí lo impide un disparador que hace fallar el update de PINs.
func TestSiElBorradoDePinsFallaElModoNoQuedaEncendido(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")

	dueno := makeUser(t, st, "dueno_atomico", "admin")
	ana := makeUser(t, st, "ana_atomica", "cajero")
	if err := users.SetPIN(ctx, ana, "4827"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}

	if _, err := st.Pool.Exec(ctx, `
		create or replace function romper_borrado_de_pins() returns trigger as $$
		begin
			if new.pin_hash is null and old.pin_hash is not null then
				raise exception 'borrado de PINs interrumpido';
			end if;
			return new;
		end $$ language plpgsql;
		create trigger romper_borrado before update on users
		for each row execute function romper_borrado_de_pins();`); err != nil {
		t.Fatalf("preparar el fallo: %v", err)
	}
	defer func() {
		_, _ = st.Pool.Exec(context.Background(), `drop trigger if exists romper_borrado on users`)
	}()

	if err := encenderSoloPin(t, st, dueno); err == nil {
		t.Fatal("el encendido dijo que funcionó aunque el borrado de PINs falló")
	}

	// Y el ajuste NO quedó encendido: si quedara, el reintento vería `encendiendo=false` y el
	// borrado no volvería a correr jamás.
	ajustes, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ajustes.PinOnlyUnlock {
		t.Error("el modo quedó encendido con los PINs viejos puestos: el reintento ya no los borra y el negocio se queda con PINs de cuatro dígitos en un modo de seis")
	}

	// El PIN de cuatro dígitos de Ana sigue ahí, que es coherente: no se borró nada y el modo no se
	// encendió. Lo que no puede pasar es la mezcla.
	var hay bool
	if err := st.Pool.QueryRow(ctx, `select pin_hash is not null from users where id = $1`, ana).Scan(&hay); err != nil {
		t.Fatalf("leer el PIN: %v", err)
	}
	if !hay {
		t.Error("se borró el PIN aunque el encendido falló")
	}
	_ = domain.DefaultIdentity()
}
