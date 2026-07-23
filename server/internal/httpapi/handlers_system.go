package httpapi

import "net/http"

// Version expone la versión del backend (SHA + fecha del build, inyectados por ldflags) para el
// pie de "Detalles del sistema" del front. Autenticado (cualquier rol) pero sin tenant: es info
// global de despliegue, no datos de empresa. No se expone en /healthz para no filtrar la versión
// a quien no está logueado.
func (h *Handlers) Version(w http.ResponseWriter, _ *http.Request) {
	JSON(w, http.StatusOK, map[string]string{"version": h.version, "builtAt": h.builtAt})
}
