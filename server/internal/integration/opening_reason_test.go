//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// LA APERTURA SE CUENTA A CIEGAS Y UNA DIFERENCIA CON EL CIERRE ANTERIOR PIDE MOTIVO (punto 6).
// El servidor lo exige: una pantalla recargada a media apertura no lo evade (EB-25). El primer turno
// de la caja no tiene contra qué compararse (EB-26).
func TestOpeningDifferentFromLastCloseNeedsReason(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_apertura_motivo", "cajero")
	reg := principalRegister(t, st)

	quinientos := dec("500")
	if _, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &quinientos, Motivo: "prueba"}, cajero); err != nil {
		t.Fatalf("primer turno sin motivo: %v", err)
	}
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &quinientos, Motivo: "prueba"}); err != nil {
		t.Fatal(err)
	}

	otro := dec("450")
	if _, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &otro, Motivo: "prueba"}, cajero); !errors.Is(err, domain.ErrOpeningReasonRequired) {
		t.Fatalf("abrir con 450 tras cerrar con 500 sin motivo: err = %v", err)
	}
	if _, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &otro, Motivo: "prueba", Reason: "other"}, cajero); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("«otro» sin texto: err = %v", err)
	}
	v, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &otro, Motivo: "prueba", Reason: "float_changed"}, cajero)
	if err != nil {
		t.Fatalf("abrir con motivo: %v", err)
	}
	var motivo string
	_ = st.Pool.QueryRow(ctx, `select opening_reason from register_sessions where id = $1`, v.ID).Scan(&motivo)
	if motivo != "float_changed" {
		t.Fatalf("el motivo no se guardó: %q", motivo)
	}
}

// Lo mismo que se contó al cerrar no pide nada.
func TestOpeningEqualToLastCloseNeedsNoReason(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_apertura_igual", "cajero")
	reg := principalRegister(t, st)
	fondo := dec("300")
	if _, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &fondo, Motivo: "prueba"}, cajero); err != nil {
		t.Fatal(err)
	}
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &fondo, Motivo: "prueba"}); err != nil {
		t.Fatal(err)
	}
	if _, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &fondo, Motivo: "prueba"}, cajero); err != nil {
		t.Fatalf("mismo conteo no pide motivo: %v", err)
	}
}

// LA APERTURA SE COMPARA CONTRA EL FONDO QUE SE DEJÓ AL CERRAR, no contra todo lo contado (decisión
// del dueño del 2026-10-10): al cerrar se retira lo vendido y se deja un fondo; abrir con ese fondo
// no pide motivo, y abrir con otra cifra sí.
func TestOpeningComparesAgainstFloatLeftAtClose(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_fondo_dejado", "cajero")
	reg := principalRegister(t, st)
	fondo := dec("500")
	if _, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &fondo, Motivo: "prueba"}, cajero); err != nil {
		t.Fatal(err)
	}
	contado, deja := dec("2300"), dec("500")
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &contado, Motivo: "prueba", FloatLeft: &deja}); err != nil {
		t.Fatal(err)
	}
	if _, err := back.OpenSession(ctx, reg, app.AperturaCmd{Total: &fondo, Motivo: "prueba"}, cajero); err != nil {
		t.Fatalf("abrir con el fondo que se dejó no pide motivo: %v", err)
	}
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &fondo, Motivo: "prueba", FloatLeft: &contado}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("dejar de fondo más de lo contado: err = %v", err)
	}
}
