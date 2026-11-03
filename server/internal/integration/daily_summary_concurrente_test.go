//go:build integration

package integration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
)

// DOS COPIAS DE LA API A LA HORA DEL RESUMEN (spec 041). Durante una subida azul/verde corren dos
// procesos, cada uno con su temporizador y su pool. El candado es el insert de ClaimSummarySend:
// la prueba de pasadas en serie (TestDailySummaryIsSentOncePerBusinessDay) no lo ejercita, porque
// la segunda pasada encuentra la fila ya `sent`. Aquí compiten a la vez, con un envío lento que
// abre la ventana entre tomar la fila y marcarla.
func TestTwoAPICopiesSendTheDailySummaryOnce(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_resumen_dos_copias", "admin")
	reg := principalRegister(t, st)
	abrirCajaPrincipal(t, st, cajero)
	cero := dec("0")
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &cero, Motivo: "prueba"}); err != nil {
		t.Fatal(err)
	}

	var enviados atomic.Int32
	enviar := func([]string, string, string) error {
		enviados.Add(1)
		time.Sleep(100 * time.Millisecond)
		return nil
	}
	// Dos procesos = dos pools del rol de app, que es como corre SendSummariesPeriodically.
	copias := []*app.DailySummaryService{
		app.NewDailySummaryService(appRoleStore(t), clock, enviar),
		app.NewDailySummaryService(appRoleStore(t), clock, enviar),
	}
	if err := app.NewDailySummaryService(st, clock, enviar).SetEmails(ctx, []string{"dueno@ejemplo.com"}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	inicio := make(chan struct{})
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			if err := copias[i%2].SendPending(ctx, defaultCompanyID); err != nil {
				t.Error(err)
			}
		}()
	}
	close(inicio)
	wg.Wait()

	if n := enviados.Load(); n != 1 {
		t.Fatalf("con dos copias de la API compitiendo salieron %d correos del mismo cierre; quería 1", n)
	}
}

// TestShutdownDuringASummaryPassLetsItFinish: el apagado detiene el ciclo del resumen, pero la
// pasada que va a medio envío termina con su propio contexto. Cortarla después de mandar el
// correo y antes de marcarlo dejaba la fila «pending», y a los 10 minutos la otra copia lo
// volvía a mandar: el dueño recibía el cierre dos veces.
func TestShutdownDuringASummaryPassLetsItFinish(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_resumen_apagado", "admin")
	reg := principalRegister(t, st)
	abrirCajaPrincipal(t, st, cajero)
	cero := dec("0")
	if _, err := back.CloseSession(ctx, reg, cajero, app.CierreCmd{Total: &cero, Motivo: "prueba"}); err != nil {
		t.Fatal(err)
	}

	loopCtx, apagar := context.WithCancel(ctx)
	enviar := func([]string, string, string) error {
		apagar() // llega la señal justo mientras se manda
		return nil
	}
	if err := app.NewDailySummaryService(st, clock, enviar).SetEmails(ctx, []string{"dueno@ejemplo.com"}); err != nil {
		t.Fatal(err)
	}
	svc := app.NewDailySummaryService(appRoleStore(t), clock, enviar)
	termino := make(chan struct{})
	go func() {
		app.SendSummariesPeriodically(loopCtx, time.Hour, svc, func(context.Context) ([]int64, error) {
			return []int64{defaultCompanyID}, nil
		})
		close(termino)
	}()
	select {
	case <-termino:
	case <-time.After(5 * time.Second):
		t.Fatal("el ciclo del resumen no terminó tras cancelar su contexto")
	}

	var estado string
	if err := st.Pool.QueryRow(ctx, `select status from daily_summary_sends where company_id = $1`, defaultCompanyID).Scan(&estado); err != nil {
		t.Fatal(err)
	}
	if estado != "sent" {
		t.Fatalf("el resumen se mandó pero quedó %q: otra copia lo volvería a mandar", estado)
	}
}
