//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// DESCARTAR EXIGE LA VERSIÓN QUE VIO LA TABLETA.
//
// Lo delató la prueba de punta a punta: la tableta B agrega a una cuenta y la tableta A, que todavía
// la ve sin ese producto, la descarta. Descartar no comparaba nada, así que A tiraba el producto de B
// y B no se enteraba: su agregado había respondido 200. Pasa igual en una cuenta nueva y en lo
// «Nuevo» de un pedido, porque las dos se descartan por el mismo camino.
func TestDiscardRefusesWhatAnotherTabletChanged(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	abrirCajaPrincipal(t, k.st, k.user)
	b := k.tenant(t, k.company)

	cases := []struct {
		name string
		// open devuelve la cuenta como la vio la tableta A.
		open func(t *testing.T) *app.DraftView
		// meanwhile es lo que hace la tableta B antes de que A descarte.
		meanwhile func(t *testing.T, v *app.DraftView)
	}{
		{
			name: "cuenta nueva: B le agrega un producto",
			open: func(t *testing.T) *app.DraftView { return k.newDraft(t, addOf(k.product, "1")) },
			meanwhile: func(t *testing.T, v *app.DraftView) {
				if _, err := k.drafts.AddLine(b, v.ID, addOf(k.product, "2")); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "cuenta nueva: B le cambia la cabecera",
			open: func(t *testing.T) *app.DraftView { return k.newDraft(t, addOf(k.product, "1")) },
			meanwhile: func(t *testing.T, v *app.DraftView) {
				name := "Mesa 4"
				if _, err := k.drafts.PatchHeader(b, v.ID, app.DraftHeaderPatch{ExpectedVersion: v.HeaderVersion,
					CustomerName: app.Field[string]{Set: true, Value: &name}}, k.user); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "«Nuevo» de un pedido: B le agrega desde el pedido",
			open: func(t *testing.T) *app.DraftView {
				ord := k.order(t, "1", "0", false)
				v, err := k.newOf(t, k.ctx, ord.ID, addOf(k.product, "1"))
				if err != nil {
					t.Fatal(err)
				}
				return v
			},
			meanwhile: func(t *testing.T, v *app.DraftView) {
				got, err := k.newOf(t, b, *v.OrderID, addOf(k.product, "1"))
				if err != nil {
					t.Fatal(err)
				}
				if got.ID != v.ID {
					t.Fatalf("B cayó en otra «Nuevo» (%s, no %s): la prueba no estaría probando la carrera", got.ID, v.ID)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			seen := c.open(t)
			c.meanwhile(t, seen)

			err := k.drafts.Discard(k.ctx, seen.ID, seen.Version, k.user)
			if !errors.Is(err, domain.ErrDraftChanged) {
				t.Fatalf("descartar con la versión vieja = %v: se tiró lo que la otra tableta acababa de hacer sin avisarle a nadie", err)
			}
			if got := draftStatus(t, k.st, seen.ID); got != domain.DraftCapturing {
				t.Fatalf("la cuenta quedó %s tras el rechazo: el 409 dice que no se aplicó nada", got)
			}

			fresh, err := k.drafts.Get(k.ctx, seen.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := k.drafts.Discard(k.ctx, seen.ID, fresh.Version, k.user); err != nil {
				t.Fatalf("descartar con la versión de la recarga = %v: tras ver la cuenta como quedó sí se puede", err)
			}
		})
	}
}

// LA VERSIÓN SE COMPARA CON LA CUENTA BLOQUEADA, NO ANTES.
//
// Concurrencia real y resultado determinista: la cuenta está tomada por otra transacción, el descarte
// de A espera el candado (se confirma en pg_stat_activity, no con un sleep) y, mientras espera, la
// otra transacción cambia la cuenta y suelta. Comparar la versión antes de bloquear —o no compararla—
// deja pasar el descarte sobre lo que ya cambió.
func TestDiscardComparesTheVersionUnderTheLock(t *testing.T) {
	t.Parallel()
	k := newDraftsKit(t)
	cafe := makeProduct(t, k.st, "Café del candado", pesos("30"), false)
	seen := k.newDraft(t, addOf(cafe, "1"))
	ctx := context.Background()

	other, err := k.st.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Rollback(ctx) //nolint:errcheck // tras el commit no hay nada que deshacer
	if _, err := other.Exec(ctx, `select 1 from order_drafts where id = $1 for update`, seen.ID); err != nil {
		t.Fatal(err)
	}

	a := k.tenant(t, k.company)
	done := make(chan error, 1)
	go func() { done <- k.drafts.Discard(a, seen.ID, seen.Version, k.user) }()

	waitForALockWaiter(t, k.st.Pool)
	// Lo que hace cualquier escritura de otra tableta: avanzar la versión de la cuenta.
	if _, err := other.Exec(ctx, `update order_drafts set version = version + 1, updated_at = now() where id = $1`, seen.ID); err != nil {
		t.Fatal(err)
	}
	if err := other.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, domain.ErrDraftChanged) {
			t.Fatalf("descartar tras esperar el candado = %v: la cuenta cambió mientras esperaba y se descartó igual", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("el descarte no terminó al soltarse el candado")
	}
	if got := draftStatus(t, k.st, seen.ID); got != domain.DraftCapturing {
		t.Fatalf("la cuenta quedó %s", got)
	}
}

// waitForALockWaiter espera a que alguna sesión de ESTA base esté detenida en un candado. Cada prueba
// tiene su propia base, así que lo que espera aquí es el descarte que la prueba lanzó.
func waitForALockWaiter(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		if err := pool.QueryRow(context.Background(), `
			select count(*) from pg_stat_activity
			 where datname = current_database() and wait_event_type = 'Lock'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("el descarte nunca llegó a esperar el candado")
}

// El cierre de caja descarta desde la fila de cuentas vivas, sin abrir la cuenta: la fila tiene que
// traer la misma versión que la cuenta, o ese descarte nunca podría pasar (o pasaría a ciegas).
func TestLiveAccountsCarryTheDraftVersion(t *testing.T) {
	t.Parallel()
	k := newLiveKit(t)
	v := k.newDraft(t, addOf(k.product, "1"))
	v, err := k.drafts.AddLine(k.ctx, v.ID, addOf(k.product, "1"))
	if err != nil {
		t.Fatal(err)
	}
	live, err := k.accounts.Live(k.ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range live.Items {
		if it.Kind == "draft" && it.DraftID != nil && *it.DraftID == v.ID {
			if it.DraftVersion == nil || *it.DraftVersion != v.Version {
				t.Fatalf("la fila trae la versión %v y la cuenta va en la %d", it.DraftVersion, v.Version)
			}
			if err := k.drafts.Discard(k.ctx, v.ID, *it.DraftVersion, k.user); err != nil {
				t.Fatalf("descartar con la versión de la fila = %v", err)
			}
			return
		}
	}
	t.Fatal("la cuenta no salió en la fila")
}
