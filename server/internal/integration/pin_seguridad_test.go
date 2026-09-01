//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"uuid"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/auth"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

func svcConPin(t *testing.T, st *store.Store) *app.AuthService {
	t.Helper()
	jm := auth.NewManager("integration-test-secret-of-32+bytes-minimum", clock)
	return app.NewAuthService(st, jm, clock)
}

// FR-010. Un id que no existe y un PIN incorrecto tienen que dar EL MISMO error: distinguirlos
// convierte el endpoint en un enumerador de usuarios de la empresa.
//
// La igualación de latencia ya existe con auth.CheckDummySecret. Este test es lo que impide que un
// refactor la quite sin que nadie note que el endpoint cambió de naturaleza.
func TestElDesbloqueoNoDistingueIdInexistenteDePinMalo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := svcConPin(t, st)
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")

	ana := makeUser(t, st, "ana_seg", "cajero")
	if err := users.SetPIN(ctx, ana, "4827"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}

	_, errPinMalo := svc.PinSwitch(ctx, ana, "0000", ana)
	_, errNoExiste := svc.PinSwitch(ctx, 999999, "4827", ana)

	if !errors.Is(errPinMalo, domain.ErrInvalidCredentials) {
		t.Fatalf("PIN incorrecto = %v, quiere ErrInvalidCredentials", errPinMalo)
	}
	if !errors.Is(errNoExiste, domain.ErrInvalidCredentials) {
		t.Fatalf("id inexistente = %v, quiere ErrInvalidCredentials", errNoExiste)
	}
	if errPinMalo.Error() != errNoExiste.Error() {
		t.Errorf("los dos errores se distinguen (%q vs %q): el endpoint enumera usuarios",
			errPinMalo, errNoExiste)
	}

	// Y la latencia tampoco los distingue: sin el bcrypt de descarte, el id inexistente vuelve
	// mucho más rápido y eso solo basta para enumerar.
	t0 := time.Now()
	_, _ = svc.PinSwitch(ctx, ana, "0000", ana)
	conBcrypt := time.Since(t0)
	t0 = time.Now()
	_, _ = svc.PinSwitch(ctx, 999999, "4827", ana)
	sinUsuario := time.Since(t0)

	// El umbral es holgado a propósito: lo que se detecta es un orden de magnitud —bcrypt contra
	// un retorno inmediato—, no ruido de milisegundos.
	if sinUsuario*4 < conBcrypt {
		t.Errorf("el id inexistente vuelve mucho antes (%v contra %v): la latencia enumera usuarios",
			sinUsuario, conBcrypt)
	}
}

// EL OBJETIVO DE LA FEATURE, de punta a punta: dos personas cobran en la MISMA estación
// identificándose con su PIN, y el arqueo las separa.
//
// Sin el cambio de operador, las dos ventas quedarían a nombre de quien dejó la tableta abierta y
// la tabla "Cobrado por" del arqueo —que ya existe— reportaría una sola persona.
func TestDosPersonasEnLaMismaEstacionSeSeparanEnElArqueo(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	svc := svcConPin(t, st)
	users := app.NewUsersService(st, nil, false, "pepper-de-prueba")
	orders := app.NewOrdersService(st, clock)
	backoffice := app.NewBackofficeService(st, clock)

	ana := makeUser(t, st, "ana_estacion", "cajero")
	luis := makeUser(t, st, "luis_estacion", "cajero")
	if err := users.SetPIN(ctx, luis, "4827"); err != nil {
		t.Fatalf("SetPIN: %v", err)
	}
	prod := makeProduct(t, st, "Café estación", decimal.RequireFromString("100"), false)
	efectivo := paymentMethodID(t, st, "Efectivo")
	principal := registerID(t, st, "Caja principal")
	abrirCajaPrincipal(t, st, ana)

	cobrar := func(quien int64) {
		t.Helper()
		o, err := orders.Create(ctx, app.CreateOrderCmd{
			ClientUUID: uuid.New(), ServiceType: "mostrador", OpenedBy: quien,
			Lines:    []domain.OrderLineInput{{ProductID: prod, Qty: decimal.RequireFromString("1")}},
			Payments: []app.PaymentInput{{MethodID: efectivo, Amount: decimal.RequireFromString("100")}},
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := orders.DeliverAll(ctx, o.ID); err != nil {
			t.Fatalf("DeliverAll: %v", err)
		}
	}

	cobrar(ana)
	// La estación se bloquea y la desbloquea Luis con su PIN: a partir de aquí él es el operador.
	sesion, err := svc.PinSwitch(ctx, luis, "4827", ana)
	if err != nil {
		t.Fatalf("PinSwitch: %v", err)
	}
	cobrar(sesion.User.ID)

	vista, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	por := map[string]string{}
	for _, c := range vista.Cashiers {
		por[c.Name] = c.Cash.String()
	}
	if por["Test ana_estacion"] != "100" {
		t.Errorf("efectivo de Ana = %q, quiere 100", por["Test ana_estacion"])
	}
	if por["Test luis_estacion"] != "100" {
		t.Errorf("efectivo de Luis = %q, quiere 100: el cambio de operador no se reflejó", por["Test luis_estacion"])
	}
}
