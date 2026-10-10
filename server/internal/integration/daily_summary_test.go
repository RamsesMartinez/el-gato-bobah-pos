//go:build integration

package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

type correoEnviado struct {
	to      []string
	subject string
	body    string
}

// EL RESUMEN DEL CIERRE LLEGA UNA VEZ POR DÍA DE NEGOCIO (punto 7; EB-37). Una segunda pasada, o
// una segunda caja del mismo día, no lo vuelve a mandar. Va a los correos de Configuración.
func TestDailySummaryIsSentOncePerBusinessDay(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	orders := app.NewOrdersService(st, clock)
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_resumen", "admin")
	reg := principalRegister(t, st)
	sinArqueoPorTerminal(t, st)

	var enviados []correoEnviado
	enviar := func(to []string, subject, body string) error {
		enviados = append(enviados, correoEnviado{to, subject, body})
		return nil
	}
	resumen := app.NewDailySummaryService(st, clock, enviar)
	if err := resumen.SetEmails(ctx, []string{"Dueno@Ejemplo.com"}); err != nil {
		t.Fatal(err)
	}

	abrirCajaPrincipal(t, st, cajero)
	cobrarConPropina(t, ctx, st, orders, "resumen", "100", "10", cajero, paymentMethodID(t, st, "Efectivo"))
	entregarPendientes(t, st)
	total := dec("110")
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &total, Motivo: "prueba", Propinas: "quedan_en_caja"}); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := resumen.SendPending(ctx, defaultCompanyID); err != nil {
			t.Fatal(err)
		}
	}
	if len(enviados) != 1 {
		t.Fatalf("se mandaron %d correos del mismo día; quería 1", len(enviados))
	}
	c := enviados[0]
	if len(c.to) != 1 || c.to[0] != "dueno@ejemplo.com" {
		t.Fatalf("destinatarios = %v", c.to)
	}
	for _, debe := range []string{"Propinas por entregar", "Salidas sin concepto"} {
		if !strings.Contains(c.body, debe) {
			t.Fatalf("el resumen no dice %q:\n%s", debe, c.body)
		}
	}
}

// Si el correo falla, el cierre ya ocurrió y el envío se reintenta después (EB-39); sin correos
// configurados no se manda nada y no truena (EB-40).
func TestDailySummaryFailureRetriesAndNoEmailsIsQuiet(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_resumen_falla", "admin")
	reg := principalRegister(t, st)
	abrirCajaPrincipal(t, st, cajero)
	cero := dec("0")
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &cero, Motivo: "prueba"}); err != nil {
		t.Fatal(err)
	}

	intentos := 0
	enviar := func([]string, string, string) error { intentos++; return errors.New("smtp caído") }
	resumen := app.NewDailySummaryService(st, clock, enviar)
	if err := resumen.SendPending(ctx, defaultCompanyID); err != nil {
		t.Fatalf("sin correos configurados no es un error: %v", err)
	}
	if intentos != 0 {
		t.Fatal("se intentó mandar sin destinatarios")
	}
	if err := resumen.SetEmails(ctx, []string{"a@b.mx"}); err != nil {
		t.Fatal(err)
	}
	_ = resumen.SendPending(ctx, defaultCompanyID)
	_ = resumen.SendPending(ctx, defaultCompanyID)
	if intentos != 2 {
		t.Fatalf("un envío fallido debe reintentarse: %d intentos", intentos)
	}
	if err := resumen.SetEmails(ctx, []string{"no-es-correo"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("correo inválido: %v", err)
	}
}
