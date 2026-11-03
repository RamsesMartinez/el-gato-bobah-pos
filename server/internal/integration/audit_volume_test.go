//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// auditLoadCeiling es lo que puede tardar abrir la bitácora con el rango más ancho que acepta (un
// año) sobre un año al tope. La pantalla corre cuatro consultas por carga —lista, total, contadores
// y aparatos— y las cuatro recorren el rango entero salvo la lista.
const auditLoadCeiling = 1500 * time.Millisecond

// LA BITÁCORA ABRE CON UN AÑO ENTERO DE EVENTOS. Un rango largo no es raro: es lo primero que se
// prueba al buscar «cuándo empezó esto». Sin medirlo, el costo de los contadores y del selector de
// aparatos (que no paginan) solo aparece en producción, cuando el año ya se llenó.
//
// 2,000 eventos al día son unos 400 pedidos con cinco acciones cada uno, por encima de un local con
// mucho movimiento. Y otra empresa del mismo tamaño: RLS agrega su predicado, y un índice mal
// elegido se queda descartando filas ajenas.
func TestTheAuditLogLoadsAYearWithinTheCeiling(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("siembra un año de bitácora; se omite en -short")
	}
	st := newTestStore(t)
	ctx := context.Background()
	co := makeCompany(t, st, "bitacora-volumen")
	other := makeCompany(t, st, "bitacora-volumen-otra")
	ana := makeUserIn(t, st, co, "ana-volumen", "gerente")
	luis := makeUserIn(t, st, other, "luis-volumen", "gerente")
	for _, c := range []struct{ company, user int64 }{{co, ana}, {other, luis}} {
		if _, err := st.Pool.Exec(ctx, `
			insert into audit_events (company_id, occurred_at, actor_kind, actor_user_id, action, entity_type, entity_id, device_key)
			select $1, now() - (g * interval '1 minute' * 0.72), 'user', $2,
			       (array['order.created','payment.charged','order.delivered','price.set','order.cancelled'])[1 + g % 5],
			       'order', g, md5((g % 12)::text)
			  from generate_series(1, 730000) g`, c.company, c.user); err != nil {
			t.Fatalf("sembrar el año: %v", err)
		}
	}
	if _, err := st.Pool.Exec(ctx, `analyze audit_events`); err != nil {
		t.Fatal(err)
	}

	k := &draftsKit{appSt: appRoleStore(t)}
	tctx := k.tenant(t, co)
	svc := app.NewAuditService(k.appSt)
	today := time.Now()
	to := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	whole := domain.AuditFilter{From: to.AddDate(0, 0, -365), To: to, Size: 50}
	// Con persona y tipo, el índice por aparato ya no cubre el filtro y la consulta vuelve a la
	// tabla: es el caso caro del selector, no el de sin filtros.
	filtered := whole
	filtered.ActorUserID = &ana
	filtered.Group = domain.AuditGroupMoney
	for name, f := range map[string]domain.AuditFilter{"sin filtros": whole, "persona y tipo": filtered} {
		if _, err := svc.List(tctx, f); err != nil { // calienta el caché: se mide la consulta, no el disco
			t.Fatal(err)
		}
		start := time.Now()
		page, err := svc.List(tctx, f)
		took := time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d eventos, %d aparatos, %v", name, page.Total, len(page.DeviceKeys), took)
		if len(page.DeviceKeys) != 12 {
			t.Errorf("%s: el selector ofrece %d aparatos, quiere 12", name, len(page.DeviceKeys))
		}
		if took > auditLoadCeiling {
			t.Errorf("%s: abrir la bitácora con un año tardó %v, techo %v", name, took, auditLoadCeiling)
		}
	}
}
