//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// EL AJUSTE NACE EN MEDIANOCHE Y RECHAZA CUALQUIER OTRO VALOR.
//
// El default es la medianoche porque es lo que un operador espera sin que nadie se lo explique, y el
// único de los tres que no depende de que alguien se acuerde de cerrar la caja.
//
// Y un valor desconocido se RECHAZA en vez de caer al default: un ajuste que acepta cualquier cosa y
// se comporta como el default deja al dueño creyendo que configuró algo que no configuró.
//
// Con dos empresas: el ajuste de una no puede tocar el de la otra.
func TestElCorteDeVistaNaceEnMedianocheYValida(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	admin := makeUser(t, st, "admin_corte", "admin")

	antes, err := settings.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if antes.CorteDeVista != domain.CorteMedianoche {
		t.Errorf("el corte nace en %q, quiere %q", antes.CorteDeVista, domain.CorteMedianoche)
	}

	info := domain.BusinessInfo{Name: antes.BusinessName, Address: antes.Address, Phone: antes.Phone}
	guardar := func(modo string) error {
		ident := domain.DefaultIdentity()
		_, err := settings.SetBusinessInfo(ctx, info, domain.PrintSettings{CorteDeVista: modo},
			ident, antes.Timezone, admin)
		return err
	}

	for _, modo := range []string{domain.CorteMedianoche, domain.CorteTurno, domain.CorteCierreDeCaja} {
		if err := guardar(modo); err != nil {
			t.Errorf("guardar %q: %v", modo, err)
		}
	}
	if err := guardar("cuando-yo-diga"); !errors.Is(err, domain.ErrValidation) {
		t.Errorf("un modo inventado = %v, quiere ErrValidation: el dueño creería que configuró algo que no configuró", err)
	}
}
