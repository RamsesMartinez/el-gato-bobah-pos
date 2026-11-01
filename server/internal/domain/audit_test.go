package domain

import (
	"errors"
	"testing"
	"time"
)

// La lista blanca de la bitácora es exactamente la del plan (data-model.md): una acción que falta
// aquí no se puede registrar, y una que sobra aparece en filtros sin que nada la produzca.
func TestAllAuditActionsIsTheWhitelistOfThePlan(t *testing.T) {
	want := []AuditAction{
		"order.created", "order.lines_added", "order.line_removed", "order.lines_moved", "order.delivered",
		"order.cancelled", "order.discount_set", "order.platform_ref_set", "payment.charged",
		"payment.voided", "payment.refunded", "stock.written_off", "cash.opened", "cash.closed",
		"cash.closed_without_opener_clock_out",
		"cash.movement", "cash.movement_corrected", "expense.created", "expense.paid", "expense.cancelled", "tip.paid_out",
		"price.set", "price.deleted", "price.synced", "attendance.clock_in", "attendance.clock_out",
		"attendance.corrected", "attendance.manual_entry", "attendance.settings_changed",
		"attendance.schedule_set", "user.tracks_attendance_changed", "payment.terminal_changed",
		"product.consumption_mode", "expense.received", "expense.count_changed", "cash.opening_backdated", "expense.stock_corrected", "stock.counted",
		"settings.costing_method", "waste.recorded", "waste.approved", "waste.rejected", "waste.undone",
		"settings.waste_threshold", "settings.count_limits_changed",
		"ingredient.created", "ingredient.updated", "ingredient.archived", "ingredient.reactivated",
		"ingredient.cost_corrected", "ingredient.unit_changed", "ingredient.unit_changed.detail",
		"ingredient.replaced", "presentation.created", "presentation.updated", "presentation.retired",
		"stock.count_confirm_asked", "expense.item_excluded", "cash.selling_register_changed",
	}
	got := AllAuditActions()
	if len(got) != len(want) {
		t.Fatalf("hay %d acciones, el plan tiene %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("acción %d = %q, quiere %q", i, got[i], want[i])
		}
		if !ValidAuditAction(string(want[i])) {
			t.Errorf("%q no es válida", want[i])
		}
	}
	for _, bad := range []string{"", "order.deleted", "ORDER.CREATED", "order.created "} {
		if ValidAuditAction(bad) {
			t.Errorf("%q pasó como acción válida", bad)
		}
	}
}

// Cada acción cae en uno de los cinco grupos que la bitácora cuenta; una sin grupo no saldría en
// ningún contador y la suma de los contadores dejaría de ser el total.
func TestEveryAuditActionHasAGroup(t *testing.T) {
	groups := map[AuditGroup]bool{}
	for _, a := range AllAuditActions() {
		g, ok := AuditGroupOf(a)
		if !ok {
			t.Errorf("%q no tiene grupo", a)
		}
		groups[g] = true
	}
	for _, g := range AllAuditGroups() {
		if !groups[g] {
			t.Errorf("el grupo %q no tiene ninguna acción", g)
		}
	}
	cases := map[AuditAction]AuditGroup{
		"payment.charged": AuditGroupMoney, "payment.terminal_changed": AuditGroupMoney, "payment.voided": AuditGroupCancel, "stock.written_off": AuditGroupCancel,
		"order.cancelled": AuditGroupCancel, "price.synced": AuditGroupPrice, "tip.paid_out": AuditGroupCash,
		"attendance.corrected": AuditGroupAttendance, "user.tracks_attendance_changed": AuditGroupAttendance,
	}
	for a, want := range cases {
		if g, _ := AuditGroupOf(a); g != want {
			t.Errorf("AuditGroupOf(%q) = %q, quiere %q", a, g, want)
		}
	}
}

// Un evento de persona lleva a la persona; uno de plataforma o del sistema no la lleva. Es el mismo
// check de la base, adelantado para que el error salga con nombre y no como 23514.
func TestAuditEventValidateTiesTheActorToItsKind(t *testing.T) {
	uid := int64(7)
	cases := []struct {
		name string
		ev   AuditEvent
		ok   bool
	}{
		{"persona", AuditEvent{Action: "order.created", ActorKind: ActorUser, ActorUserID: &uid, EntityType: "order", EntityID: 1}, true},
		{"persona sin id", AuditEvent{Action: "order.created", ActorKind: ActorUser, EntityType: "order", EntityID: 1}, false},
		{"plataforma", AuditEvent{Action: "order.created", ActorKind: ActorPlatform, ActorLabel: "uber", EntityType: "order", EntityID: 1}, true},
		{"plataforma con persona", AuditEvent{Action: "order.created", ActorKind: ActorPlatform, ActorUserID: &uid, EntityType: "order", EntityID: 1}, false},
		{"acción desconocida", AuditEvent{Action: "order.deleted", ActorKind: ActorSystem, EntityType: "order", EntityID: 1}, false},
		{"sin objeto", AuditEvent{Action: "price.synced", ActorKind: ActorSystem}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.ev.Validate()
			if (err == nil) != c.ok {
				t.Fatalf("Validate() = %v, quiere ok=%v", err, c.ok)
			}
			if err != nil && !errors.Is(err, ErrValidation) {
				t.Fatalf("el error no envuelve ErrValidation: %v", err)
			}
		})
	}
}

// Un filtro inválido se RECHAZA (constitución V): caer a un default mostraría una bitácora que
// parece correcta y no es la que se pidió.
func TestAuditFilterValidateRejectsInsteadOfDefaulting(t *testing.T) {
	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	key := "0123456789abcdef0123456789abcdef"
	ok := AuditFilter{From: d("2026-10-01"), To: d("2026-10-11"), Size: 25}
	cases := []struct {
		name string
		mut  func(*AuditFilter)
		ok   bool
	}{
		{"normal", func(*AuditFilter) {}, true},
		{"tamaño 50", func(f *AuditFilter) { f.Size = 50 }, true},
		{"tamaño 30", func(f *AuditFilter) { f.Size = 30 }, false},
		{"tamaño 0", func(f *AuditFilter) { f.Size = 0 }, false},
		{"página negativa", func(f *AuditFilter) { f.Page = -1 }, false},
		{"página que desborda el offset", func(f *AuditFilter) { f.Page = 21474837 }, false},
		{"última página permitida", func(f *AuditFilter) { f.Page = 10000 }, true},
		{"desde después de hasta", func(f *AuditFilter) { f.From = d("2026-10-12") }, false},
		{"más de un año", func(f *AuditFilter) { f.From = d("2025-01-01") }, false},
		{"acción conocida", func(f *AuditFilter) { f.Action = "price.set" }, true},
		{"acción desconocida", func(f *AuditFilter) { f.Action = "order.deleted" }, false},
		{"grupo conocido", func(f *AuditFilter) { f.Group = "cancel" }, true},
		{"grupo desconocido", func(f *AuditFilter) { f.Group = "todo" }, false},
		{"clave bien formada", func(f *AuditFilter) { f.DeviceKey = key }, true},
		{"clave abreviada", func(f *AuditFilter) { f.DeviceKey = "a3f9" }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := ok
			c.mut(&f)
			err := f.Validate()
			if (err == nil) != c.ok {
				t.Fatalf("Validate() = %v, quiere ok=%v", err, c.ok)
			}
			if err != nil && !errors.Is(err, ErrValidation) {
				t.Fatalf("no envuelve ErrValidation: %v", err)
			}
		})
	}
}

// Acción y grupo juntos se intersectan; sin ninguno, no se filtra por acción.
func TestAuditFilterActions(t *testing.T) {
	if got := (AuditFilter{}).Actions(); got != nil {
		t.Fatalf("sin filtro = %v, quiere nil", got)
	}
	// Tres de precio, qué resta un producto (spec 037), el costeo del almacén (spec 038) y el tope de
	// aprobación de la merma (spec 039): todas son configuración de lo que cuesta o se resta. Más las
	// once de los artículos del almacén (spec 042) y los límites del conteo (spec 044).
	if got := (AuditFilter{Group: "price"}).Actions(); len(got) != 18 {
		t.Fatalf("grupo precios = %v", got)
	}
	if got := (AuditFilter{Group: "price", Action: "price.set"}).Actions(); len(got) != 1 || got[0] != "price.set" {
		t.Fatalf("grupo y acción = %v", got)
	}
	// Una acción fuera de su grupo no devuelve nada, en vez de ignorar el grupo.
	if got := (AuditFilter{Group: "price", Action: "payment.charged"}).Actions(); got == nil || len(got) != 0 {
		t.Fatalf("acción de otro grupo = %#v, quiere lista vacía", got)
	}
}
