//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/httpapi"
)

// Las fotos por la puerta para agentes (spec 091): la computadora del catálogo sube en lote con
// vista previa, idempotencia y bitácora, y las mismas reglas que la pantalla.
func TestMCPCatalogPhotos(t *testing.T) {
	t.Parallel()
	w := newMCPWorld(t)
	s3, c := newFakeS3(t)
	router := mcpRouter(w.app, w.jm, func(d *httpapi.Deps) { d.Media = app.NewCatalogMediaService(w.app, c, mediaBase) })
	tok := w.token(t, w.company, w.admin, "read", "write:catalog")
	product := makeProduct(t, w.owner, "Taro del lote", pesos("65"), false)
	var cat int64
	if err := w.owner.Pool.QueryRow(context.Background(), `select category_id from products where id = $1`, product).Scan(&cat); err != nil {
		t.Fatal(err)
	}
	b64 := func(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
	args := func(seed byte, extra map[string]any) map[string]any {
		m := map[string]any{"product_id": product, "image_1200_base64": b64(webpOf(1200, seed)), "image_400_base64": b64(webpOf(400, seed))}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}

	t.Run("la vista previa valida en el servidor y no sube nada", func(t *testing.T) {
		res := callTool(t, router, tok, "product_image_set", args(1, nil))
		previewed(t, res)
		if !strings.Contains(res.text, "Taro del lote") {
			t.Errorf("la vista previa no dice qué producto: %s", res.text)
		}
		if s3.count() != 0 {
			t.Fatalf("la vista previa subió %d archivos", s3.count())
		}
		bad := args(1, nil)
		bad["image_400_base64"] = b64(webpOf(300, 1))
		refused(t, callTool(t, router, tok, "product_image_set", bad), "debe medir 400×400")
	})

	t.Run("aplicar sube y la repetición no vuelve a subir", func(t *testing.T) {
		a := applied(t, callTool(t, router, tok, "product_image_set", args(1, map[string]any{"dry_run": false, "idempotency_key": "foto-taro-0001"})))
		url, _ := a["imageUrl"].(string)
		if !strings.HasSuffix(url, "-1200.webp") || s3.count() != 2 {
			t.Fatalf("url %q, archivos %d", url, s3.count())
		}
		again := callTool(t, router, tok, "product_image_set", args(1, map[string]any{"dry_run": false, "idempotency_key": "foto-taro-0001"}))
		if again.data["replayed"] != true {
			t.Fatalf("la misma llamada no se reconoció como repetición: %s", again.text)
		}
		other := callTool(t, router, tok, "product_image_set", args(2, map[string]any{"dry_run": false, "idempotency_key": "foto-taro-0001"}))
		if !other.isError {
			t.Fatal("otra foto con la misma clave se aplicó: la clave dejaría de proteger contra dobles")
		}
	})

	t.Run("la bitácora del MCP no guarda la foto", func(t *testing.T) {
		var n int
		if err := w.owner.Pool.QueryRow(context.Background(),
			`select count(*) from audit_events where action = 'mcp.tool_called' and after::text like '%UklGR%'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%d renglones de la bitácora llevan el base64 de la foto", n)
		}
		var withHash int
		if err := w.owner.Pool.QueryRow(context.Background(),
			`select count(*) from audit_events where action = 'mcp.tool_called' and after::text like '%sha256%'`).Scan(&withHash); err != nil || withHash == 0 {
			t.Fatalf("la bitácora no dice qué foto se subió (huella): %d %v", withHash, err)
		}
	})

	t.Run("familia y foto de la categoría, y quitar", func(t *testing.T) {
		applied(t, callTool(t, router, tok, "category_family_set", map[string]any{"category_id": cat, "food_family": "tea",
			"dry_run": false, "idempotency_key": "familia-0001"}))
		refused(t, callTool(t, router, tok, "category_family_set", map[string]any{"category_id": cat, "food_family": "taco"}), "familia")
		applied(t, callTool(t, router, tok, "category_image_set", map[string]any{"category_id": cat,
			"image_1200_base64": b64(webpOf(1200, 9)), "image_400_base64": b64(webpOf(400, 9)),
			"dry_run": false, "idempotency_key": "foto-cat-0001"}))
		applied(t, callTool(t, router, tok, "category_image_remove", map[string]any{"category_id": cat, "dry_run": false, "idempotency_key": "quita-cat-0001"}))
		applied(t, callTool(t, router, tok, "product_image_remove", map[string]any{"product_id": product, "dry_run": false, "idempotency_key": "quita-taro-0001"}))
		if s3.count() != 0 {
			t.Fatalf("quedaron %d archivos", s3.count())
		}
	})

	t.Run("sin el alcance de catálogo no se puede", func(t *testing.T) {
		ro := w.token(t, w.company, w.admin, "read")
		// Sin el alcance la herramienta no se registra para ese token (072): llamarla es «unknown tool».
		res := callTool(t, router, ro, "product_image_set", args(3, map[string]any{"dry_run": false, "idempotency_key": "foto-sin-0001"}))
		if !res.isError && len(res.rpcErr) == 0 {
			t.Fatalf("un token de solo lectura puso una foto: %s", res.result)
		}
		if contains(listTools(t, router, ro), "product_image_set") {
			t.Error("un token sin write:catalog ve product_image_set")
		}
		if s3.count() != 0 {
			t.Fatalf("quedaron %d archivos", s3.count())
		}
	})

	t.Run("otra empresa no ve ni toca la foto", func(t *testing.T) {
		ajeno := w.token(t, w.other, w.otherAdm, "read", "write:catalog")
		prev := callTool(t, router, ajeno, "product_image_set", args(4, nil))
		if strings.Contains(prev.text, "Taro del lote") {
			t.Errorf("otra empresa ve el producto en la vista previa: %s", prev.text)
		}
		res := callTool(t, router, ajeno, "product_image_set", args(4, map[string]any{"dry_run": false, "idempotency_key": "foto-ajena-001"}))
		if !res.isError || s3.count() != 0 {
			t.Fatalf("otra empresa puso la foto: %s (archivos %d)", res.text, s3.count())
		}
	})
}
