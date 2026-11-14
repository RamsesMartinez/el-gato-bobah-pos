//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// EL AVISO EN VIVO SALE SOLO SI LA VENTA CONFIRMA (spec 084, R3): el trigger avisa con NOTIFY, que
// Postgres entrega al confirmar y descarta al revertir; la API lo pasa al Broker de esa empresa.
func TestNotificationListenerPublishesOnCommitOnly(t *testing.T) {
	st := newTestStore(t)
	broker := realtime.NewBroker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener := app.NewNotificationListener(basesPorPrueba[pruebaRaiz(t.Name())], broker)
	go listener.Run(ctx)
	events, unsub := broker.Subscribe(defaultCompanyID)
	defer unsub()
	if !listener.WaitReady(5 * time.Second) {
		t.Fatal("el listener no se conectó")
	}

	ing := pieceIngredient(t, st, "En vivo 084")
	adjust(t, st, "ingrediente", ing, "10")
	alertOn(t, st, ing, "5", true, false)

	// Revertida: nada en vivo.
	tx, err := st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `insert into stock_movements (item_type, ingredient_id, movement_type, quantity) values ('ingrediente', $1, 'venta', -7)`, ing); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	select {
	case ev := <-events:
		t.Fatalf("una venta revertida avisó en vivo: %+v", ev)
	case <-time.After(500 * time.Millisecond):
	}

	// Confirmada: un evento para su empresa.
	adjust(t, st, "ingrediente", ing, "-7")
	select {
	case ev := <-events:
		if ev.Type != "notifications.changed" {
			t.Fatalf("evento %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("el aviso abierto no llegó en vivo")
	}
}
