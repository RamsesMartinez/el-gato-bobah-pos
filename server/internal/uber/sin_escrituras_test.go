package uber

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// archivosDondeSePermiteNombrarUnVerboDeEscritura: el guardia (que menciona los verbos para
// rechazarlos) y el cliente de token (que hace el único POST legítimo, contra el host de auth).
var archivosDondeSePermiteNombrarUnVerboDeEscritura = map[string]bool{
	"solo_lectura.go":        true,
	"solo_lectura_test.go":   true,
	"token.go":               true,
	"token_test.go":          true,
	"sin_escrituras_test.go": true,
}

// LA ALARMA TEMPRANA de que nadie agregó una escritura (FR-007).
//
// El transporte de solo_lectura.go es la garantía: bloquea el verbo en tiempo de ejecución. Esta
// prueba es lo que hace que el intento falle en `go test` en vez de en producción, y nombra el
// archivo y la línea para que quien lo agregó sepa de inmediato qué tocó.
//
// Por qué un parse del AST y no un grep: un grep no distingue un `http.MethodPost` dentro de un
// comentario o de una cadena de uno que se ejecuta, y falsos positivos en un gate enseñan a
// ignorarlo.
func TestNingunaEscrituraEnElPaquete(t *testing.T) {
	prohibidos := map[string]bool{"MethodPost": true, "MethodPut": true, "MethodPatch": true, "MethodDelete": true}

	archivos, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, ruta := range archivos {
		if archivosDondeSePermiteNombrarUnVerboDeEscritura[filepath.Base(ruta)] {
			continue
		}
		f, err := parser.ParseFile(fset, ruta, nil, 0)
		if err != nil {
			t.Fatalf("parsear %s: %v", ruta, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok || ident.Name != "http" || !prohibidos[sel.Sel.Name] {
				return true
			}
			t.Errorf("%s: http.%s — este paquete es de SOLO LECTURA y un PUT de menú reemplaza el menú publicado completo, sin deshacer",
				fset.Position(sel.Pos()), sel.Sel.Name)
			return true
		})
		// Y la forma literal, que el chequeo de arriba no ve: `req, _ := http.NewRequest("PUT", …)`.
		for _, verbo := range []string{`"POST"`, `"PUT"`, `"PATCH"`, `"DELETE"`} {
			cuerpo, err := os.ReadFile(ruta)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(cuerpo), verbo) {
				t.Errorf("%s: el verbo %s aparece como literal; este paquete es de solo lectura", ruta, verbo)
			}
		}
	}
}

// FR-008: NO SE AUTOMATIZA EL PORTAL DE COMERCIOS CON UN NAVEGADOR.
//
// Es la misma clase de prohibición que la de escritura, y la que más caro sale: los términos
// mexicanos de Uber prohíben explícitamente «lanzar cualquier programa o script con el objeto de
// extraer… datos de cualquier parte de los Servicios», y la terminación es inmediata y sin causa
// pactada. El beneficio serían minutos de trabajo a la semana; el costo del peor caso es la cuenta
// de la que vive el negocio.
//
// Se vigila en go.mod porque es ahí donde entraría: nadie escribe un navegador a mano.
func TestNingunaDependenciaDeAutomatizacionDeNavegador(t *testing.T) {
	mod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("leer go.mod: %v", err)
	}
	for _, sospechosa := range []string{"chromedp", "go-rod", "playwright", "puppeteer", "selenium", "agouti"} {
		if strings.Contains(string(mod), sospechosa) {
			t.Errorf("go.mod trae %q: automatizar el portal de comercios está prohibido por los términos de la plataforma, y la terminación es inmediata", sospechosa)
		}
	}
}
