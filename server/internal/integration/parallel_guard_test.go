//go:build integration

package integration

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// serialByDesign son las pruebas que corren solas aunque no toquen migraciones. Cada una dice por
// qué: una entrada sin razón es una prueba lenta que nadie se atrevió a revisar.
var serialByDesign = map[string]string{
	// Mide una consulta contra un techo de 30 ms; con otras pruebas peleando por el mismo Postgres
	// el techo se rompe por ruido y no por la consulta.
	"TestLiveAccountsStaysFast": "mide tiempo contra un techo",
}

// TestEveryIntegrationTestDeclaresHowItRuns vigila que la suite siga corriendo en paralelo.
//
// Cada prueba estrena su propia base clonada, así que no comparten datos; lo que comparten es el
// SERVIDOR de Postgres. En serie, la suite pasó de 72 s a 183 s en CI en tres días sin que
// ninguna prueba fuera lenta: eran 424 de ~0.25 s una tras otra. Una prueba nueva sin
// `t.Parallel()` vuelve a sumar su tiempo completo al de todas, y nada lo avisa.
//
// La excepción es el estado que no vive en la base de la prueba: bajar o subir migraciones con
// goose (cambia roles, que en Postgres son de todo el clúster, y goose guarda su configuración en
// variables globales) y reemplazar la bitácora global con slog.SetDefault (otra prueba en paralelo
// escribe en ella y el mensaje que se buscaba aparece, o falta, por azar). Esas pruebas NO pueden
// ir en paralelo, y la regla se revisa en las dos direcciones: una que las toca y pide
// `t.Parallel()` también falla.
func TestEveryIntegrationTestDeclaresHowItRuns(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	funcs := map[string]*ast.FuncDecl{}
	for _, f := range files {
		parsed, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parsear %s: %v", f, err)
		}
		for _, d := range parsed.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Body != nil {
				funcs[fn.Name.Name] = fn
			}
		}
	}

	touchesShared := sharedStateTouchers(funcs)

	var missing, forbidden []string
	for name, fn := range funcs {
		if !isTopLevelTest(fn) {
			continue
		}
		parallel := startsWithParallel(fn)
		_, byDesign := serialByDesign[name]
		switch {
		case touchesShared[name] && parallel:
			forbidden = append(forbidden, name)
		case !touchesShared[name] && !byDesign && !parallel:
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(forbidden)
	if len(missing) > 0 {
		t.Errorf("%d pruebas no llaman t.Parallel() como primera línea; cada una suma su tiempo entero al de la suite. "+
			"Agrégalo, o si de verdad tiene que correr sola, anótala en serialByDesign con la razón:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
	if len(forbidden) > 0 {
		t.Errorf("%d pruebas tocan estado del proceso o del servidor (goose, slog.SetDefault) y aun así piden "+
			"t.Parallel(); las demás pruebas lo usan al mismo tiempo:\n  %s",
			len(forbidden), strings.Join(forbidden, "\n  "))
	}
}

// sharedStateTouchers devuelve las funciones del paquete que llegan, directa o indirectamente, a
// goose o a slog.SetDefault.
func sharedStateTouchers(funcs map[string]*ast.FuncDecl) map[string]bool {
	calls := map[string][]string{}
	touches := map[string]bool{}
	for name, fn := range funcs {
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				if id, ok := x.X.(*ast.Ident); ok && (id.Name == "goose" || id.Name == "slog" && x.Sel.Name == "SetDefault") {
					touches[name] = true
				}
			case *ast.CallExpr:
				if id, ok := x.Fun.(*ast.Ident); ok {
					calls[name] = append(calls[name], id.Name)
				}
			}
			return true
		})
	}
	for changed := true; changed; {
		changed = false
		for name, callees := range calls {
			if touches[name] {
				continue
			}
			for _, c := range callees {
				if touches[c] {
					touches[name] = true
					changed = true
					break
				}
			}
		}
	}
	return touches
}

func isTopLevelTest(fn *ast.FuncDecl) bool {
	if !strings.HasPrefix(fn.Name.Name, "Test") || fn.Name.Name == "TestMain" {
		return false
	}
	params := fn.Type.Params.List
	if len(params) != 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "T"
}

// startsWithParallel exige que sea la PRIMERA sentencia: un t.Parallel() después de sembrar datos
// sigue siendo correcto, pero lo sembrado antes corre en serie y el ahorro se pierde sin aviso.
func startsWithParallel(fn *ast.FuncDecl) bool {
	if len(fn.Body.List) == 0 {
		return false
	}
	expr, ok := fn.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expr.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Parallel"
}
