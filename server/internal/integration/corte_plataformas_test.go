//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// El fondo de apertura y los movimientos de efectivo se cuentan UNA sola vez en el corte, no una
// por cada método que toca el cajón.
//
// Hasta 0037 solo existía un método de cajón ("Efectivo"), así que sumarlo por método daba el
// resultado correcto por casualidad. Al desdoblar cada plataforma en "en línea" y "efectivo" hay
// cuatro, y sin este arreglo un turno con $1,500 de fondo y CERO ventas reporta $4,500 de faltante
// que no existe: los tres métodos nuevos esperan $1,500 cada uno y, como no se autodeclaran, el
// cierre los compara contra lo que el front no mandó.
func TestElFondoDeCajaSeCuentaUnaSolaVez(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_fondo", "cajero")
	principal := registerID(t, st, "Caja principal")
	fondo := decimal.RequireFromString("1500")
	if _, err := backoffice.OpenSession(ctx, principal, fondo, cajero); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	sess, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}

	// Sin ventas, el único método que espera dinero es el efectivo del mostrador, y espera
	// exactamente el fondo.
	var conFondo []string
	for _, tot := range sess.Totals {
		if tot.Expected.Equal(fondo) {
			conFondo = append(conFondo, tot.Name)
		}
	}
	if len(conFondo) != 1 {
		t.Fatalf("el fondo de $1,500 debe aparecer en UN solo método, apareció en %d: %v", len(conFondo), conFondo)
	}
	if conFondo[0] != "Efectivo" {
		t.Fatalf("el fondo debe ir al efectivo del mostrador, fue a %q", conFondo[0])
	}

	// Y el resto espera cero: no hubo ventas.
	for _, tot := range sess.Totals {
		if tot.Name == "Efectivo" {
			continue
		}
		if !tot.Expected.IsZero() {
			t.Fatalf("%q espera %s sin haber vendido nada", tot.Name, tot.Expected)
		}
	}
}

// Cerrar un turno sin ventas y declarando el fondo exacto no debe arrojar diferencia. Es la misma
// regla vista desde el cierre, que es donde el operador la sufre: un faltante inventado obliga a
// contar el cajón tres veces buscando dinero que nunca faltó.
func TestCerrarSinVentasNoInventaFaltante(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)

	cajero := makeUser(t, st, "cajero_cierre", "cajero")
	principal := registerID(t, st, "Caja principal")
	fondo := decimal.RequireFromString("1500")
	if _, err := backoffice.OpenSession(ctx, principal, fondo, cajero); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	// El operador cuenta el cajón y declara el fondo, que es lo único que hay.
	efectivo := paymentMethodID(t, st, "Efectivo")
	declarado := map[int]decimal.Decimal{int(efectivo): fondo}

	cerrada, err := backoffice.CloseSession(ctx, principal, cajero, declarado, "")
	if err != nil {
		t.Fatalf("CloseSession: %v", err)
	}
	for _, tot := range cerrada.Totals {
		dif := tot.Declared.Sub(tot.Expected)
		if !dif.IsZero() {
			t.Fatalf("%q cerró con diferencia de %s (esperado %s, declarado %s) sin haber vendido nada",
				tot.Name, dif, tot.Expected, tot.Declared)
		}
	}
}
