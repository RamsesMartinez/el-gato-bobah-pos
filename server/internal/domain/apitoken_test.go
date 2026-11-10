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
	if len(seen) != 33 || writes != 11 {
		t.Fatalf("el contrato tiene 33 herramientas y 11 escrituras; hay %d y %d", len(seen), writes)
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
	} {
		if got := ToolErrorHintFor(409, code, "la caja está cerrada", ""); !strings.Contains(got, want) || !strings.Contains(got, "la caja está cerrada") {
			t.Errorf("%s: %q no dice %q", code, got, want)
		}
	}
	if m := MissingScopeMessage(ScopeWriteExpenses); !strings.Contains(m, "write:expenses") || !strings.Contains(m, "Integraciones") {
		t.Fatalf("mensaje de alcance: %q", m)
	}
}
