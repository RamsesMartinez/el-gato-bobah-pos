package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// EL ENDPOINT DE MEDICIÓN NO PUEDE ESTORBAR NUNCA (US3, FR-004).
//
// Responde 204 pase lo que pase. El cliente no espera la respuesta ni la puede usar: un 400 solo
// serviría para que alguien lo vea en la pestaña de red y crea que algo se rompió.
//
// El servicio va con store nil a propósito: todos los casos de aquí se resuelven ANTES de tocar la
// base, y si alguno la tocara el test truena con un nil pointer — que es exactamente lo que hay que
// enterarse.
func TestLaMedicionSiempreResponde204(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}, Usage: app.NewUsageService(nil)})

	casos := []struct {
		nombre string
		cuerpo string
	}{
		{"lote vacío", `{"eventos":[]}`},
		{"sin el campo", `{}`},
		{"cuerpo que no es JSON", `no soy json`},
		{"pantalla inventada", `{"eventos":[{"pantalla":"la-que-no-existe"}]}`},
		{"acción inventada", `{"eventos":[{"pantalla":"pos","accion":"hacer-magia"}]}`},
		{"nombre absurdo", `{"eventos":[{"pantalla":"` + strings.Repeat("a", 5000) + `"}]}`},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/usage", strings.NewReader(c.cuerpo))
			req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: 7, CompanyID: 1, Role: domain.RoleCajero}))
			w := httptest.NewRecorder()
			h.RegistrarUso(w, req)

			if w.Code != http.StatusNoContent {
				t.Fatalf("status = %d, quiere 204 — y sin cuerpo: %s", w.Code, w.Body.String())
			}
			if w.Body.Len() != 0 {
				t.Fatalf("respondió con cuerpo (%s): el cliente no lo espera ni lo puede usar", w.Body.String())
			}
		})
	}
}

// EL LIMITADOR EXISTE Y MUERDE, y este test es su único testigo posible.
//
// Como el endpoint responde 204 pase lo que pase, un limitador roto —o desconectado por un refactor
// del router— no se nota por ninguna vía: ni un error, ni un log, ni una respuesta distinta. El
// principio V no deja mergear un control de seguridad sin su test, y aquí el principio se queda
// corto: sin el test, el control es indistinguible de no existir.
func TestLaMedicionSeLimitaPorUsuario(t *testing.T) {
	h := NewHandlers(Deps{Cfg: config.Config{}, Usage: app.NewUsageService(nil)})
	ctx := context.Background()
	for i := 0; i < usoMax+1; i++ {
		h.usoIngesta.record(ctx, "uso:7")
	}

	// Con el limitador puesto, un lote VÁLIDO no llega al servicio (que tiene store nil). Si el
	// limitador no mordiera, esto sería un nil pointer — y ese pánico es el fallo.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("el lote pasó el limitador y llegó a escribir (%v): el tope no está cableado", r)
		}
	}()
	cuerpo, _ := json.Marshal(map[string]any{"eventos": []map[string]string{{"pantalla": "pos"}}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/usage", bytes.NewReader(cuerpo))
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, AuthUser{ID: 7, CompanyID: 1, Role: domain.RoleCajero}))
	w := httptest.NewRecorder()
	h.RegistrarUso(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, quiere 204 incluso al limitar: el cliente no puede enterarse", w.Code)
	}
}
