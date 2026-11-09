package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/alerts"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// reportServerErrors avisa por correo de cada respuesta 500 (spec 070) y recupera los panics: es el
// reemplazo de middleware.Recoverer. Solo encola el aviso; todo lo lento pasa en el Notifier.
func reportServerErrors(n *alerts.Notifier, env string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// El traceInfo nace aquí y no en RequestLogger: RequireAuth lo llena más adentro y este
			// middleware necesita leer quién era al terminar.
			ti := traceFrom(r.Context())
			if ti == nil {
				ti = &traceInfo{}
				r = r.WithContext(context.WithValue(r.Context(), traceKey{}, ti))
			}
			sw := &serverErrorWriter{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				rec := recover()
				if rec == nil {
					if sw.status == http.StatusInternalServerError {
						n.Report(newServerAlert(r, ti, env, sw.err, sw.pcs))
					}
					return
				}
				// La forma en que net/http corta una respuesta a propósito; no es un error.
				if rec == http.ErrAbortHandler { //nolint:errorlint // ErrAbortHandler se compara por identidad, como hace net/http
					panic(rec)
				}
				pcs := make([]uintptr, 64)
				pcs = pcs[:runtime.Callers(2, pcs)]
				a := newServerAlert(r, ti, env, nil, pcs)
				a.Panic = true
				a.Kind = fmt.Sprintf("panic:%T", rec)
				a.Message = domain.SanitizeAlertText(fmt.Sprint(rec))
				slog.Error("panic", "request_id", a.RequestID, "route", a.Route, "panic", a.Message,
					"stack", strings.Join(a.Frames, " | "))
				if !sw.wrote {
					JSON(sw, http.StatusInternalServerError, errorEnvelope{Error: errorBody{Code: "INTERNAL", Message: "Error interno del servidor"}})
				}
				n.Report(a)
			}()
			next.ServeHTTP(sw, r)
		})
	}
}

func newServerAlert(r *http.Request, ti *traceInfo, env string, err error, pcs []uintptr) domain.ServerAlert {
	route := ""
	if rc := chi.RouteContext(r.Context()); rc != nil {
		route = rc.RoutePattern()
	}
	if route == "" {
		route = "(sin ruta)"
	}
	a := domain.ServerAlert{
		At: time.Now(), Environment: env, Method: r.Method, Route: route,
		RequestID: requestIDFrom(r.Context()), UserID: ti.userID, UserName: ti.userName,
		CompanyID: ti.companyID, Kind: alerts.ErrorKind(err), Frames: framesOf(pcs),
		Message: "500 sin error registrado",
	}
	if err != nil {
		a.Message = domain.SanitizeAlertText(err.Error())
	}
	return a
}

func framesOf(pcs []uintptr) []string {
	if len(pcs) == 0 {
		return nil
	}
	var frames []runtime.Frame
	it := runtime.CallersFrames(pcs)
	for {
		f, more := it.Next()
		frames = append(frames, f)
		if !more {
			break
		}
	}
	return domain.ProjectFrames(frames)
}

// serverErrorWriter ve el estado final y guarda el error que Error() le entrega.
type serverErrorWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
	err    error
	pcs    []uintptr
}

func (w *serverErrorWriter) WriteHeader(code int) {
	if !w.wrote {
		w.status, w.wrote = code, true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *serverErrorWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Flush mantiene vivo el SSE de /events, igual que captureWriter.
// Un Flush antes del primer Write ya mandó el 200: se anota para que un panic posterior no pinte un
// 500 encima de una respuesta enviada.
func (w *serverErrorWriter) Flush() {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *serverErrorWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// recordServerError le entrega a reportServerErrors el error de un 500 y dónde se produjo.
// Error() solo recibe el writer, así que el writer es el canal: se recorre la cadena de Unwrap
// hasta el que lo acepta. Un wrapper nuevo sin Unwrap no rompe el aviso: sale «sin error
// registrado», que delata el wrapper.
func recordServerError(w http.ResponseWriter, err error) {
	for w != nil {
		if sw, ok := w.(*serverErrorWriter); ok {
			pcs := make([]uintptr, 64)
			sw.err, sw.pcs = err, pcs[:runtime.Callers(3, pcs)]
			return
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return
		}
		w = u.Unwrap()
	}
}
