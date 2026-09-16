package httpapi

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Menús de plataforma (spec 020): leer lo publicado y decir en qué difiere del catálogo.
//
// Handlers finos: decodifican, llaman al servicio y mapean el error. **Ninguno escribe en una
// plataforma** — el guardia real está en el transporte de internal/uber, que rechaza todo verbo
// distinto de GET antes de abrir el socket.

func conexionDeRuta(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// maxExternalID es el mismo tope que el `check` de la migración. Se valida AQUÍ, en la frontera, y
// no solo en la base.
const maxExternalID = 200

// externalIDDeRuta lee el id de la plataforma y lo acota ANTES de que toque nada.
//
// Sin la cota, un id de 900 KB llega hasta la consulta, Postgres lo rechaza —un byte nulo da
// `22021`— y el error se envuelve interpolando el valor: `fmt.Errorf("... %q", externalID)`. Ese
// mensaje termina íntegro en la bitácora, que rota a 1 MB por 10 respaldos: **doce peticiones
// borran todo el histórico de eventos de seguridad**, y destruir el rastro es justo lo que quiere
// quien ya se hizo de esa cuenta.
//
// Es el mismo defecto que la tercera ronda de docs/security-owasp.md ya cerró para `username`. Se
// reintrodujo aquí, y por eso la cota va en la frontera: lo que no entra no se puede registrar.
func externalIDDeRuta(r *http.Request) (string, error) {
	crudo, err := url.PathUnescape(chi.URLParam(r, "externalId"))
	if err != nil {
		return "", domain.ErrValidation
	}
	if crudo == "" || len(crudo) > maxExternalID {
		return "", domain.ErrValidation
	}
	// Un byte nulo no lo rechaza PathUnescape y sí Postgres, con un 500 en vez de un 400.
	if strings.ContainsRune(crudo, 0) || !utf8.ValidString(crudo) {
		return "", domain.ErrValidation
	}
	return crudo, nil
}

// GET /admin/platform-menus/connections
func (h *Handlers) ListPlatformConnections(w http.ResponseWriter, r *http.Request) {
	cons, err := h.menusPlataforma.ListarConexiones(r.Context())
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"connections": cons})
}

type altaDeConexionBody struct {
	PlatformID      int16  `json:"platformId"`
	ExternalStoreID string `json:"externalStoreId"`
	Label           string `json:"label"`
}

// POST /admin/platform-menus/connections
func (h *Handlers) CreatePlatformConnection(w http.ResponseWriter, r *http.Request) {
	var body altaDeConexionBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.PlatformID == 0 {
		Error(w, domain.ErrValidation)
		return
	}
	id, err := h.menusPlataforma.CrearConexion(r.Context(), app.AltaDeConexion{
		PlatformID: body.PlatformID, ExternalStoreID: body.ExternalStoreID, Label: body.Label,
	})
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusCreated, map[string]any{"id": id})
}

// DELETE /admin/platform-menus/connections/{id}
//
// Se lleva en cascada las lecturas Y el emparejamiento: hasta una sesión completa de trabajo
// manual. La pantalla muestra cuántas parejas se pierden antes de confirmar, con el conteo que
// devuelve GET .../links/count.
func (h *Handlers) DeletePlatformConnection(w http.ResponseWriter, r *http.Request) {
	id, err := conexionDeRuta(r)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	if err := h.menusPlataforma.BorrarConexion(r.Context(), id); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /admin/platform-menus/connections/{id}/links/count
func (h *Handlers) CountPlatformLinks(w http.ResponseWriter, r *http.Request) {
	id, err := conexionDeRuta(r)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	n, err := h.menusPlataforma.ParejasQueSePierden(r.Context(), id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"links": n})
}

// POST /admin/platform-menus/connections/{id}/read
//
// Responde 202 DE INMEDIATO: la lectura corre aparte, con su propio contexto. El menú real pesa
// 211 KB y esperarlo aquí dejaría una conexión ocupada, que es lo que SC-006 prohíbe.
func (h *Handlers) ReadPlatformMenu(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	id, err := conexionDeRuta(r)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	lecturaID, inicio, err := h.menusPlataforma.DispararLectura(r.Context(), u.CompanyID, id)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusAccepted, map[string]any{
		"readId": lecturaID, "status": "en_curso", "startedAt": inicio,
	})
}

// GET /admin/platform-menus/connections/{id}/reads
func (h *Handlers) ListPlatformMenuReads(w http.ResponseWriter, r *http.Request) {
	id, err := conexionDeRuta(r)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	limite, err := limiteDeQuery(r.URL.Query(), 20)
	if err != nil {
		Error(w, err)
		return
	}
	lecturas, err := h.menusPlataforma.ListarLecturas(r.Context(), id, limite)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, map[string]any{"reads": lecturas})
}

// claseDeQuery lee el nivel que se está emparejando o comparando.
//
// Un valor desconocido se RECHAZA; nunca cae a un default. Un `kind` mal escrito que se convirtiera
// en "platillo" devolvería una pantalla que se ve correcta y muestra otra cosa — el default es para
// el parámetro AUSENTE, jamás para el presente y malformado (principio V).
func claseDeQuery(q url.Values) (domain.ClaseDeItem, error) {
	crudo := q.Get("kind")
	if crudo == "" {
		return domain.ItemPlatillo, nil
	}
	c := domain.ClaseDeItem(crudo)
	if !domain.ClaseDeItemValida(c) {
		return "", domain.ErrValidation
	}
	return c, nil
}

// GET /admin/platform-menus/connections/{id}/pairing?kind=platillo|opcion
func (h *Handlers) PlatformMenuPairing(w http.ResponseWriter, r *http.Request) {
	id, err := conexionDeRuta(r)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	clase, err := claseDeQuery(r.URL.Query())
	if err != nil {
		Error(w, err)
		return
	}
	emp, err := h.menusPlataforma.Emparejar(r.Context(), id, clase)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, emp)
}

type parejaBody struct {
	LocalID   int64  `json:"localId"`
	LocalKind string `json:"localKind"`
	Kind      string `json:"kind"`
	// Reemplazar una pareja ya confirmada se pide explícito: el `PUT` a secas responde 409 para que
	// la pantalla pregunte antes de pisar una decisión que alguien tomó a mano.
	Replace bool `json:"replace"`
}

// PUT /admin/platform-menus/connections/{id}/links/{externalId}
//
// El {externalId} viaja url-encoded y **se guarda sin normalizar** (FR-020): los ids de Uber traen
// acentos y emoji, y recortarlos rompe la pareja contra la siguiente lectura.
func (h *Handlers) SetPlatformItemLink(w http.ResponseWriter, r *http.Request) {
	u, ok := userFrom(r.Context())
	if !ok {
		Error(w, domain.ErrUnauthorized)
		return
	}
	id, err := conexionDeRuta(r)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	externalID, err := externalIDDeRuta(r)
	if err != nil {
		Error(w, err)
		return
	}
	var body parejaBody
	if err := Decode(r, &body); err != nil {
		Error(w, err)
		return
	}
	if body.LocalID == 0 {
		Error(w, domain.ErrValidation)
		return
	}
	err = h.menusPlataforma.GuardarPareja(r.Context(), app.AltaDePareja{
		ConexionID: id, ExternalID: externalID,
		Clase:      domain.ClaseDeItem(body.Kind),
		LocalID:    body.LocalID,
		ClaseLocal: domain.ClaseLocal(body.LocalKind),
		UsuarioID:  u.ID,
		Reemplazar: body.Replace,
	})
	if err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /admin/platform-menus/connections/{id}/links/{externalId}
func (h *Handlers) DeletePlatformItemLink(w http.ResponseWriter, r *http.Request) {
	id, err := conexionDeRuta(r)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	externalID, err := externalIDDeRuta(r)
	if err != nil {
		Error(w, err)
		return
	}
	if err := h.menusPlataforma.BorrarPareja(r.Context(), id, externalID); err != nil {
		Error(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// clasesDeDiferenciaDeQuery lee qué clases de diferencia se piden.
//
// Por omisión **solo las accionables**: precio y disponibilidad. Los «solo en un lado» van en otra
// pestaña porque la mayoría nunca se van a emparejar a propósito —el POS tiene 174 productos
// activos y la plataforma 65 platillos— y mezclarlos ahoga todos los días lo que sí hay que
// corregir.
func clasesDeDiferenciaDeQuery(q url.Values) (map[domain.ClaseDeDiferencia]bool, error) {
	crudas, hay := q["kind"]
	if !hay {
		return map[domain.ClaseDeDiferencia]bool{
			domain.DifPrecio: true, domain.DifDisponibilidad: true,
		}, nil
	}
	validas := map[domain.ClaseDeDiferencia]bool{
		domain.DifPrecio: true, domain.DifDisponibilidad: true,
		domain.DifSoloEnPlataforma: true, domain.DifSoloEnCatalogo: true,
	}
	out := map[domain.ClaseDeDiferencia]bool{}
	for _, c := range crudas {
		k := domain.ClaseDeDiferencia(c)
		if !validas[k] {
			return nil, domain.ErrValidation
		}
		out[k] = true
	}
	return out, nil
}

// GET /admin/platform-menus/connections/{id}/differences
func (h *Handlers) PlatformMenuDifferences(w http.ResponseWriter, r *http.Request) {
	id, err := conexionDeRuta(r)
	if err != nil {
		Error(w, domain.ErrValidation)
		return
	}
	clases, err := clasesDeDiferenciaDeQuery(r.URL.Query())
	if err != nil {
		Error(w, err)
		return
	}
	cmp, err := h.menusPlataforma.Diferencias(r.Context(), id, clases)
	if err != nil {
		Error(w, err)
		return
	}
	JSON(w, http.StatusOK, cmp)
}
