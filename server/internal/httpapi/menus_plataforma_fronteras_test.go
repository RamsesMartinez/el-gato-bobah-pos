package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// LA FRONTERA DEL `externalId`, que llega por la ruta y se guarda sin normalizar.
//
// Existe porque un id largo era un camino directo a borrar la bitácora: se interpolaba crudo en el
// mensaje de error —`fmt.Errorf("... %q", externalID)`— y de ahí a `slog.Error`. Con la bitácora
// rotando a 1 MB por 10 respaldos, **doce peticiones se llevaban todo el histórico de eventos de
// seguridad**, y destruir el rastro es justo lo que quiere quien ya se hizo de la cuenta.
//
// Es el mismo defecto que la tercera ronda de docs/security-owasp.md cerró para `username`, y se
// reintrodujo aquí. La cota va en la frontera porque lo que no entra no se puede registrar.
func TestElIdDeLaPlataformaSeAcotaEnLaFrontera(t *testing.T) {
	casos := []struct {
		nombre string
		valor  string
		pasa   bool
	}{
		{nombre: "el caso de todos los días", valor: "Chamoyada_de_Mango", pasa: true},
		{nombre: "con acentos y emoji, que los ids reales traen", valor: "Chamoyada_de_Mojit🌿", pasa: true},
		{nombre: "justo en el tope", valor: strings.Repeat("a", 200), pasa: true},
		{nombre: "vacío", valor: "", pasa: false},
		{nombre: "un carácter arriba del tope", valor: strings.Repeat("a", 201), pasa: false},
		// El que borra la bitácora. 900 KB caben en una URL: el tope lo pone MaxHeaderBytes, 1 MiB.
		{nombre: "novecientos mil caracteres", valor: strings.Repeat("a", 900_000), pasa: false},
		// PathUnescape NO lo rechaza y Postgres sí, con un 500 en vez de un 400.
		{nombre: "con un byte nulo", valor: "abc\x00def", pasa: false},
		{nombre: "con UTF-8 inválido", valor: "abc\xff\xfe", pasa: false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := externalIDDeRuta(conRuta(httptest.NewRequest(http.MethodPut, "/x", nil), c.valor))
			if c.pasa {
				if err != nil {
					t.Fatalf("%q debería pasar: %v", c.nombre, err)
				}
				if got != c.valor {
					// FR-020: se guarda TAL CUAL. Recortarlo o normalizarlo rompe la pareja contra
					// la siguiente lectura, y los ids de Uber traen acentos y emoji.
					t.Errorf("el id se alteró: %q → %q", c.valor, got)
				}
				return
			}
			if err == nil {
				t.Fatalf("%q debería rechazarse y dio %q", c.nombre, got)
			}
			// 422 y no 500: un dato malo del cliente no es una falla del servidor, y el 500 es el
			// que arrastra el valor al log.
			if err != domain.ErrValidation {
				t.Errorf("debería ser ErrValidation: %v", err)
			}
		})
	}
}

// El nivel que se empareja o compara no cae a un default cuando viene mal escrito.
//
// Un `kind` desconocido que se convirtiera en «platillo» devolvería una pantalla que se ve correcta
// y muestra otra cosa. El default es para el parámetro AUSENTE, jamás para el presente y malformado.
func TestElNivelMalEscritoSeRechaza(t *testing.T) {
	if c, err := claseDeQuery(url.Values{}); err != nil || c != domain.ItemPlatillo {
		t.Errorf("ausente debería caer a platillo: %q %v", c, err)
	}
	for _, bueno := range []string{"platillo", "opcion", "grupo"} {
		if _, err := claseDeQuery(url.Values{"kind": {bueno}}); err != nil {
			t.Errorf("%q debería aceptarse: %v", bueno, err)
		}
	}
	for _, malo := range []string{"PLATILLO", "platillos", "dish", " platillo", "1"} {
		if _, err := claseDeQuery(url.Values{"kind": {malo}}); err == nil {
			t.Errorf("%q debería rechazarse, no caer a platillo", malo)
		}
	}
}

// La lista de diferencias abre con lo ACCIONABLE, y un filtro inventado se rechaza.
func TestLasClasesDeDiferenciaPorOmisionSonLasAccionables(t *testing.T) {
	porOmision, err := clasesDeDiferenciaDeQuery(url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	// Con 109 sin emparejar, mezclar los «solo en un lado» ahogaría todos los días lo que sí hay
	// que corregir — y la mayoría nunca se van a emparejar a propósito.
	if !porOmision[domain.DifPrecio] || !porOmision[domain.DifDisponibilidad] {
		t.Error("por omisión faltan precio o disponibilidad")
	}
	if porOmision[domain.DifSoloEnPlataforma] || porOmision[domain.DifSoloEnCatalogo] {
		t.Error("por omisión NO deben venir los «solo en un lado»")
	}
	if _, err := clasesDeDiferenciaDeQuery(url.Values{"kind": {"precio", "inventada"}}); err == nil {
		t.Error("una clase inventada debería rechazarse, no ignorarse")
	}
}

// conRuta mete el RouteContext de chi en el request, que es de donde `chi.URLParam` lee.
func conRuta(r *http.Request, valor string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("externalId", url.PathEscape(valor))
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}
