//go:build integration

package integration

import (
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// Una cuenta en captura de mostrador con un producto que se apagó para Mostrador lo muestra «ya no
// se vende» desde que se apaga, no hasta que enviar la rechaza (revisión de código de la 092). Una
// cuenta de plataforma con el mismo producto lo sigue teniendo disponible: la plataforma ya lo vendió.
func TestDraftRespectsHiddenAtCounter(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	admin, actx := adminAsApp(t, k.st, defaultCompanyID, "cuenta")
	id := makeProduct(t, k.st, "Crepa de temporada", dec("70"), false)

	counter := k.newDraft(t, addOf(id, "1"))
	if err := updateWith(t, actx, admin, k.st, id, &app.SalesInput{Counter: false, Web: true, AllowOversell: true}); err != nil {
		t.Fatal(err)
	}
	v, err := k.drafts.Get(k.ctx, counter.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Lines) != 1 || v.Lines[0].Available {
		t.Fatalf("la cuenta de mostrador sigue ofreciendo un producto apagado para Mostrador: %+v", v.Lines)
	}
	if v.Total.IsPositive() {
		t.Fatalf("la cuenta cobraría %s por un producto que ya no se vende en mostrador", v.Total)
	}
}
