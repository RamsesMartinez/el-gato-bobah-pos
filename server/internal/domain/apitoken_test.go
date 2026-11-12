package domain

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

var secretoDePrueba = strings.Repeat("ab", 32)

func TestParseAPIToken(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		company int64
		id      int64
		ok      bool
	}{
		{"canónico", "egbpat_2_15_" + secretoDePrueba, 2, 15, true},
		{"ida y vuelta", FormatAPIToken(9, 1, secretoDePrueba), 9, 1, true},
		{"empresa con cero a la izquierda: dos textos serían el mismo token", "egbpat_02_15_" + secretoDePrueba, 0, 0, false},
		{"id cero", "egbpat_2_0_" + secretoDePrueba, 0, 0, false},
		{"empresa cero", "egbpat_0_1_" + secretoDePrueba, 0, 0, false},
		{"signo: strconv lo aceptaría", "egbpat_+2_15_" + secretoDePrueba, 0, 0, false},
		{"secreto corto", "egbpat_2_15_" + secretoDePrueba[:63], 0, 0, false},
		{"secreto largo", "egbpat_2_15_" + secretoDePrueba + "a", 0, 0, false},
		{"secreto en mayúsculas", "egbpat_2_15_" + strings.ToUpper(secretoDePrueba), 0, 0, false},
		{"separador de más", "egbpat_2_15__" + secretoDePrueba, 0, 0, false},
		{"otro prefijo", "egbtok_2_15_" + secretoDePrueba, 0, 0, false},
		{"espacios alrededor", " egbpat_2_15_" + secretoDePrueba, 0, 0, false},
		{"empresa que desborda int64", "egbpat_99999999999999999999_15_" + secretoDePrueba, 0, 0, false},
		{"vacío", "", 0, 0, false},
		{"un JWT de la pantalla", "eyJhbGciOiJIUzI1NiJ9.e30.x", 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseAPIToken(c.raw)
			if !c.ok {
				if !errors.Is(err, ErrUnauthorized) {
					t.Fatalf("esperaba ErrUnauthorized, salió %v (%+v)", err, got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.CompanyID != c.company || got.TokenID != c.id || got.Secret != secretoDePrueba {
				t.Fatalf("salió %+v", got)
			}
		})
	}
}

func TestAPITokenSecretHelpers(t *testing.T) {
	if h := APITokenSecretHash(secretoDePrueba); len(h) != 32 {
		t.Fatalf("la huella debe medir 32 bytes, mide %d", len(h))
	}
	if string(APITokenSecretHash("a")) == string(APITokenSecretHash("b")) {
		t.Fatal("dos secretos distintos dieron la misma huella")
	}
	if APITokenHint(secretoDePrueba) != "abab" {
		t.Fatalf("pista: %q", APITokenHint(secretoDePrueba))
	}
	if !APITokenSecretMatches(secretoDePrueba, APITokenSecretHash(secretoDePrueba)) {
		t.Fatal("el secreto correcto no coincidió con su huella")
	}
	if APITokenSecretMatches(strings.Repeat("cd", 32), APITokenSecretHash(secretoDePrueba)) {
		t.Fatal("un secreto ajeno coincidió con la huella")
	}
	if APITokenSecretMatches(secretoDePrueba, nil) {
		t.Fatal("una huella vacía no puede coincidir con nada")
	}
}

func TestNormalizeAPITokenInput(t *testing.T) {
	cases := []struct {
		name   string
		in     APITokenInput
		scopes []APIScope
		ok     bool
	}{
		{"lectura se agrega sola", APITokenInput{Name: "Claude", ExpiresInDays: 90}, []APIScope{ScopeRead}, true},
		{"orden canónico y sin duplicados", APITokenInput{Name: "x", ExpiresInDays: 30,
			Scopes: []string{"write:catalog", "read", "write:expenses", "write:catalog"}},
			[]APIScope{ScopeRead, ScopeWriteExpenses, ScopeWriteCatalog}, true},
		{"los alcances de la 073 en su orden", APITokenInput{Name: "x", ExpiresInDays: 90,
			Scopes: []string{"write:orders", "write:purchases", "write:cash"}},
			[]APIScope{ScopeRead, ScopeWriteCash, ScopeWritePurchases, ScopeWriteOrders}, true},
		{"nombre con espacios se recorta", APITokenInput{Name: "  Claude  ", ExpiresInDays: 180}, []APIScope{ScopeRead}, true},
		{"nombre vacío tras recortar", APITokenInput{Name: "   ", ExpiresInDays: 90}, nil, false},
		{"nombre de 61", APITokenInput{Name: strings.Repeat("a", 61), ExpiresInDays: 90}, nil, false},
		{"alcance desconocido", APITokenInput{Name: "x", ExpiresInDays: 90, Scopes: []string{"write:users"}}, nil, false},
		{"alcance vacío", APITokenInput{Name: "x", ExpiresInDays: 90, Scopes: []string{""}}, nil, false},
		{"vigencia que no es opción", APITokenInput{Name: "x", ExpiresInDays: 365}, nil, false},
		{"sin vigencia: un token sin fecha es lo que se prohíbe", APITokenInput{Name: "x"}, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.in.Normalize()
			if !c.ok {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("esperaba ErrValidation, salió %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(c.in.Name) != got.Name {
				t.Fatalf("nombre %q", got.Name)
			}
			if len(got.Scopes) != len(c.scopes) {
				t.Fatalf("alcances %v, esperaba %v", got.Scopes, c.scopes)
			}
			for i := range c.scopes {
				if got.Scopes[i] != c.scopes[i] {
					t.Fatalf("alcances %v, esperaba %v", got.Scopes, c.scopes)
				}
			}
		})
	}
}

func TestScopeAllows(t *testing.T) {
	lectura := []APIScope{ScopeRead}
	gastos := []APIScope{ScopeRead, ScopeWriteExpenses}
	if !ScopeAllows(lectura, ScopeRead) || ScopeAllows(lectura, ScopeWriteExpenses) {
		t.Fatal("un token de lectura debe poder leer y no escribir gastos")
	}
	if ScopeAllows(gastos, ScopeWriteCatalog) {
		t.Fatal("escribir gastos no da escribir el catálogo")
	}
	if ScopeAllows([]APIScope{ScopeRead, ScopeWriteExpenses}, ScopeWritePurchases) {
		t.Fatal("capturar gastos no da capturar compras con mercancía")
	}
	if ScopeAllows([]APIScope{ScopeRead, ScopeWriteCash}, ScopeWriteOrders) {
		t.Fatal("la caja no da cancelar pedidos")
	}
	if ScopeAllows(nil, ScopeRead) {
		t.Fatal("sin alcances no se puede nada")
	}
}

func TestDecideIdempotency(t *testing.T) {
	a, b := []byte("huella-a"), []byte("huella-b")
	cases := []struct {
		name     string
		existing *IdempotencyRecord
		want     IdempotencyDecision
	}{
		{"clave nueva", nil, IdemProceed},
		{"misma huella, terminada: devuelve lo guardado sin repetir", &IdempotencyRecord{Status: IdemStatusDone, ArgsHash: a}, IdemReplay},
		{"otra huella con la misma clave", &IdempotencyRecord{Status: IdemStatusDone, ArgsHash: b}, IdemConflict},
		{"otra huella mientras la primera sigue", &IdempotencyRecord{Status: IdemStatusPending, ArgsHash: b}, IdemConflict},
		{"la primera sigue en curso", &IdempotencyRecord{Status: IdemStatusPending, ArgsHash: a}, IdemInProgress},
		{"la primera terminó en error interno o se cortó", &IdempotencyRecord{Status: IdemStatusUnknown, ArgsHash: a}, IdemUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DecideIdempotency(c.existing, a); got != c.want {
				t.Fatalf("salió %v, esperaba %v", got, c.want)
			}
		})
	}
}

func TestValidIdempotencyKey(t *testing.T) {
	for k, ok := range map[string]bool{
		"gasto-2026-11-10-001": true, "a1b2c3d4": true, "x:y.z_w-1": true,
		"corta": false, strings.Repeat("a", 101): false, "con espacio 123": false, "ñandú-1234": false, "": false,
	} {
		if ValidIdempotencyKey(k) != ok {
			t.Errorf("%q: esperaba %v", k, ok)
		}
	}
}

func TestIdempotencyArgsHashIncludesTheTool(t *testing.T) {
	args := []byte(`{"amount":"10.00"}`)
	if string(IdempotencyArgsHash("expense_create", args)) == string(IdempotencyArgsHash("expense_pay", args)) {
		t.Fatal("la misma clave con otra herramienta debe verse como argumentos distintos")
	}
	if len(IdempotencyArgsHash("x", args)) != 32 {
		t.Fatal("la huella mide 32 bytes")
	}
}

func TestMCPToolCatalog(t *testing.T) {
	snake := regexp.MustCompile(`^[a-z]+(_[a-z]+)*$`)
	seen := map[string]bool{}
	writes := 0
	for _, tool := range MCPTools() {
		if seen[tool.Name] {
			t.Errorf("%s repetida", tool.Name)
		}
		seen[tool.Name] = true
		if !snake.MatchString(tool.Name) {
			t.Errorf("%s no es snake_case en inglés", tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("%s sin descripción", tool.Name)
		}
		if tool.Write {
			writes++
			if tool.Scope == ScopeRead || !strings.HasPrefix(string(tool.Scope), "write:") {
				t.Errorf("%s escribe con alcance %q", tool.Name, tool.Scope)
			}
			if tool.Permission == "" {
				t.Errorf("%s escribe sin permiso: toda escritura de la pantalla tiene uno", tool.Name)
			}
		} else if tool.Scope != ScopeRead {
			t.Errorf("%s lee con alcance %q", tool.Name, tool.Scope)
		}
		if tool.Permission != "" && !KnownPermission(tool.Permission) {
			t.Errorf("%s pide un permiso que no existe: %s", tool.Name, tool.Permission)
		}
		if _, ok := MCPToolByName(tool.Name); !ok {
			t.Errorf("%s no se encuentra por nombre", tool.Name)
		}
	}
	if len(seen) != 54 || writes != 25 {
		t.Fatalf("el contrato tiene 54 herramientas y 25 escrituras (076); hay %d y %d", len(seen), writes)
	}
	// Las de la 073, con el alcance y el permiso de contracts/mcp-tools.md.
	for name, want := range map[string]MCPTool{
		"articles_search":         {Scope: ScopeRead, Permission: PermExpensesCapture},
		"units_list":              {Scope: ScopeRead, Permission: PermExpensesCapture},
		"presentations_list":      {Scope: ScopeRead, Permission: PermInventoryReceive},
		"pending_receipts_list":   {Scope: ScopeRead, Permission: PermInventoryReceive},
		"purchase_create":         {Scope: ScopeWritePurchases, Permission: PermExpensesCapture, Write: true},
		"purchase_receive":        {Scope: ScopeWritePurchases, Permission: PermInventoryReceive, Write: true},
		"stock_count_current":     {Scope: ScopeRead, Permission: PermInventoryCount},
		"stock_count_open":        {Scope: ScopeWriteInventory, Permission: PermInventoryCount, Write: true},
		"stock_count_capture":     {Scope: ScopeWriteInventory, Permission: PermInventoryCount, Write: true},
		"stock_count_close":       {Scope: ScopeWriteInventory, Permission: PermInventoryCount, Write: true},
		"stock_count_discard":     {Scope: ScopeWriteInventory, Permission: PermInventoryCount, Write: true},
		"order_cancel":            {Scope: ScopeWriteOrders, Permission: PermOrdersCancel, Write: true},
		"cash_denominations_list": {Scope: ScopeRead, Permission: PermCashOperate},
		"cash_session_attendance": {Scope: ScopeRead, Permission: PermCashOperate},
		"cash_session_open":       {Scope: ScopeWriteCash, Permission: PermCashOperate, Write: true},
		"cash_session_close":      {Scope: ScopeWriteCash, Permission: PermCashOperate, Write: true},
	} {
		got, ok := MCPToolByName(name)
		if !ok || got.Scope != want.Scope || got.Permission != want.Permission || got.Write != want.Write {
			t.Errorf("%s: %+v, esperaba alcance %s, permiso %s, escritura %v", name, got, want.Scope, want.Permission, want.Write)
		}
	}
	if _, ok := MCPToolByName("users_create"); ok {
		t.Fatal("una herramienta fuera del catálogo se encontró")
	}
}

func TestToolErrorHint(t *testing.T) {
	cases := []struct {
		status int
		msg    string
		want   []string
	}{
		{400, "falta la categoría", []string{"falta la categoría", "Revisa los argumentos"}},
		{422, "la caja está cerrada", []string{"la caja está cerrada", "Revisa los argumentos"}},
		{401, "", []string{"no es válido", "Integraciones"}},
		{403, "Tu usuario no puede capturar gastos", []string{"Tu usuario no puede capturar gastos", "administra los roles"}},
		{404, "", []string{"No existe en esta empresa", "expenses_list"}},
		{409, "el gasto ya está pagado", []string{"el gasto ya está pagado"}},
		{429, "", []string{"Demasiadas llamadas"}},
		{500, "boom: pq: detalle interno", []string{"Error interno", "Revisa con una lectura"}},
		{503, "", []string{"Error interno"}},
	}
	for _, c := range cases {
		got := ToolErrorHint(c.status, c.msg, "expenses_list")
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%d: %q no contiene %q", c.status, got, w)
			}
		}
		if c.status >= 500 && strings.Contains(got, "pq:") {
			t.Errorf("%d: un error interno no se le muestra al agente: %q", c.status, got)
		}
	}
	// Los rechazos de caja que un agente puede corregir dicen cómo, no solo qué pasó.
	for code, want := range map[string]string{
		"EXPENSE_OUTSIDE_COUNT":     "include_in_count: false",
		"EXPENSE_BEFORE_LAST_CLOSE": "include_in_count: false",
		"NO_OPEN_REGISTER":          "cash_registers_list",
		"PLATFORM_PRICE_MANAGED":    "plataforma conectada",
		"OPENING_REASON_REQUIRED":   "opening_reason",
		"TIPS_DECISION_REQUIRED":    "tips_decision",
		"REFUND_FOLIO_REQUIRED":     "card_folio",
		"TERMINAL_COUNT_REQUIRED":   "terminal_counts",
		"CLOCK_IN_REQUIRED":         "entrada",
		"OPENER_NOT_CLOCKED_OUT":    "authorized_close_reason",
		"NEEDS_CONFIRM":             "confirmed: true",
		"INGREDIENT_MERGED":         "articles_search",
		"OPEN_ORDERS":               "order_cancel",
		"UNPAID_ORDERS":             "pantalla",
		// Spec 074: el protocolo de la edición de gastos.
		"EXPENSE_EDIT_STALE": "vista previa",
		"DECISION_REQUIRED":  "decisions",
	} {
		if got := ToolErrorHintFor(409, code, "la caja está cerrada", ""); !strings.Contains(got, want) || !strings.Contains(got, "la caja está cerrada") {
			t.Errorf("%s: %q no dice %q", code, got, want)
		}
	}
	if m := MissingScopeMessage(ScopeWriteExpenses); !strings.Contains(m, "write:expenses") || !strings.Contains(m, "Integraciones") {
		t.Fatalf("mensaje de alcance: %q", m)
	}
}

// Las escrituras de la spec 074 van en los alcances que ya existen (D1, D2): un empaque de compra es
// configuración del almacén, y corregir un gasto es del alcance del permiso de editarlo.
func TestMCPToolCatalog074(t *testing.T) {
	for _, c := range []struct {
		name  string
		scope APIScope
		perm  Permission
	}{
		{"presentation_create", ScopeWriteInventory, PermInventoryConfigure},
		{"presentation_update", ScopeWriteInventory, PermInventoryConfigure},
		{"expense_edit", ScopeWriteExpenses, PermExpensesEdit},
	} {
		tool, ok := MCPToolByName(c.name)
		if !ok {
			t.Errorf("%s no está en el catálogo", c.name)
			continue
		}
		if tool.Scope != c.scope || tool.Permission != c.perm || !tool.Write {
			t.Errorf("%s = %+v; quiere alcance %s, permiso %s, escritura", c.name, tool, c.scope, c.perm)
		}
	}
}

// Spec 076: el nombre repetido. La pantalla ofrece «Abrir ese» o «Reactivar» con el id del insumo que
// ya tiene el nombre; el agente solo ve la pista, así que el id y si está archivado van en ella.
func TestToolErrorHintNamesTheExistingArticle(t *testing.T) {
	cases := []struct {
		name string
		code string
		ref  ExistingRef
		want []string
		not  []string
	}{
		{"nombre de un activo", "NAME_TAKEN", ExistingRef{ID: 749}, []string{"ya existe", "id 749", "Usa ese insumo"}, []string{"archivado"}},
		// Spec 077 (D6): un archivado ya se reactiva por MCP; mandarlo a la pantalla era cierto en la 076.
		{"nombre de un archivado", "NAME_TAKEN", ExistingRef{ID: 495, Archived: true}, []string{"id 495", "archivado", "ingredient_update", "active: true"}, []string{"pantalla"}},
		{"otro nombre de otro insumo", "ALT_NAME_TAKEN", ExistingRef{ID: 12}, []string{"id 12", "Quita ese nombre"}, nil},
		{"ficha cambiada", "STALE", ExistingRef{}, []string{"vuelve a intentar"}, []string{"id 0"}},
		{"sin id no inventa uno", "NAME_TAKEN", ExistingRef{}, []string{"ya existe"}, []string{"id 0"}},
	}
	for _, c := range cases {
		got := ToolErrorHintWith(409, c.code, "ya existe", "", c.ref)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: %q no dice %q", c.name, got, w)
			}
		}
		for _, n := range c.not {
			if strings.Contains(got, n) {
				t.Errorf("%s: %q no debería decir %q", c.name, got, n)
			}
		}
	}
}

// Las escrituras de insumos van en el alcance de almacén con el permiso de configurarlo (spec 076, D1),
// y la lista de empaques sigue siendo lectura con el permiso de recibir.
func TestMCPToolCatalog076(t *testing.T) {
	for _, c := range []struct {
		name  string
		scope APIScope
		perm  Permission
		write bool
	}{
		{"ingredient_create", ScopeWriteInventory, PermInventoryConfigure, true},
		{"ingredient_update", ScopeWriteInventory, PermInventoryConfigure, true},
		{"presentations_list", ScopeRead, PermInventoryReceive, false},
	} {
		tool, ok := MCPToolByName(c.name)
		if !ok {
			t.Errorf("%s no está en el catálogo", c.name)
			continue
		}
		if tool.Scope != c.scope || tool.Permission != c.perm || tool.Write != c.write {
			t.Errorf("%s = %s/%s/%v, quería %s/%s/%v", c.name, tool.Scope, tool.Permission, tool.Write, c.scope, c.perm, c.write)
		}
	}
}

// Spec 077: desactivar un insumo. La regla es del servicio (#203, 042); la pista dice al agente qué
// hacer en palabras de la operación: la existencia se resuelve con merma o ajuste, y solo la persona
// decide archivarlo con ella; una receta activa se arregla en la pantalla y nada se desactiva en cascada.
func TestToolErrorHintForArchivingAnIngredient(t *testing.T) {
	for code, want := range map[string][]string{
		"ARCHIVE_HAS_STOCK": {"waste_record", "stock_adjust", "confirm_on_hand: true", "persona"},
		"IN_USE":            {"pantalla", "receta"},
	} {
		got := ToolErrorHintFor(409, code, "todavía tiene existencia", "")
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: %q no dice %q", code, got, w)
			}
		}
	}
}
