package httpapi

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Pantalla de Ventas: el análisis de lo que ya pasó, distinto del tablero de pedidos.
//
// Son DOS endpoints y no uno que devuelva lista + resumen juntos. La razón es el gesto más repetido
// de la pantalla: paginar. Cambiar de página no cambia el resumen, y con una sola respuesta cada
// tap del paginador volvería a agregar todo el rango para tirar el resultado. Separados, la llave
// del resumen no lleva la página y el caché del cliente la conserva.

// GET /sales?preset=&from=&to=&status=&serviceType=&sort=&dir=&page=&pageSize=
func (h *Handlers) ListSales(w http.ResponseWriter, r *http.Request) {
	f, err := h.filtroDeVentas(r)
	if err != nil {
		Error(w, err)
		return
	}
	page, err := h.sales.List(r.Context(), f)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, page)
}

// GET /sales/summary — mismos filtros de rango y tipo; el de estado NO aplica.
func (h *Handlers) SalesSummary(w http.ResponseWriter, r *http.Request) {
	f, err := h.filtroDeVentas(r)
	if err != nil {
		Error(w, err)
		return
	}
	sum, err := h.sales.Summary(r.Context(), f)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, sum)
}

// filtroDeVentas traduce el query string y deja que el dominio decida si es aceptable.
//
// El handler solo transporta: parsea texto a tipos. Quién dice que un estado o una columna de orden
// existen es `domain.SalesFilter.Validate`, que se prueba sin base de datos.
//
// Nada cae a un default en silencio. Un `sort` desconocido, un rango invertido o una página absurda
// se rechazan como 400: una pantalla que se ve correcta y responde algo que nadie pidió es peor que
// un error, porque nadie la audita.
func (h *Handlers) filtroDeVentas(r *http.Request) (domain.SalesFilter, error) {
	// url.Query() DESCARTA en silencio lo que no pudo parsear y devuelve el resto: un query string
	// con un punto y coma —que Go dejó de aceptar como separador— haría que todos los filtros se
	// perdieran y la pantalla contestara los defaults como si nadie hubiera pedido nada. Parsear a
	// mano es lo que convierte eso en un 400.
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return domain.SalesFilter{}, fmt.Errorf("%w: los filtros de la petición no se pudieron leer", domain.ErrValidation)
	}

	desde, err := parseDate(q.Get("from"), time.Time{})
	if err != nil {
		return domain.SalesFilter{}, err
	}
	hasta, err := parseDate(q.Get("to"), time.Time{})
	if err != nil {
		return domain.SalesFilter{}, err
	}
	// La zona del NEGOCIO, no la del servidor: "hoy" en un local de México a las 19:00 ya es
	// mañana en UTC, y el rango saldría corrido justo en la hora de más venta.
	rango, err := domain.ResolveRange(q.Get("preset"), desde, hasta, h.sales.Now(), h.sales.Location(r.Context()))
	if err != nil {
		return domain.SalesFilter{}, err
	}

	f := domain.SalesFilter{
		Range:       rango,
		Status:      q.Get("status"),
		ServiceType: q.Get("serviceType"),
		Sort:        valorODefault(q.Get("sort"), "fecha"),
		Dir:         valorODefault(q.Get("dir"), "desc"),
	}

	limit, offset, err := paginaDeQuery(q)
	if err != nil {
		return domain.SalesFilter{}, err
	}
	f.Limit, f.Offset = limit, offset

	if err := f.Validate(); err != nil {
		return domain.SalesFilter{}, err
	}
	return f, nil
}

// valorODefault aplica el default solo al parámetro AUSENTE. Uno presente y desconocido llega a
// Validate y se rechaza; ese es justamente el punto.
func valorODefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
