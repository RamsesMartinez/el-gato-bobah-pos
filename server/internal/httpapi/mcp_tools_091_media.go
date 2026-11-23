package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// Las fotos del catálogo por el MCP (spec 091), para la computadora que tiene el catálogo de fotos.
// Cada herramienta manda el cuerpo de la pantalla a su ruta: tipo, medidas y peso los rechaza el
// servidor igual que a la pantalla. La vista previa valida los archivos sin subir nada.

//nolint:gosec // G101: patrones de rutas; gosec confunde palabras con credenciales.
const (
	pProductImage  = "/api/v1/admin/products/{id}/image"
	pCategoryImage = "/api/v1/admin/categories/{id}/image"
	pCategory      = "/api/v1/admin/categories/{id}"
)

func init() {
	for name, routes := range map[string][]mcpRoute{
		"product_image_set":     {{http.MethodPut, pProductImage}, get(pProductImage)},
		"product_image_remove":  {{http.MethodDelete, pProductImage}, get(pProductImage)},
		"category_image_set":    {{http.MethodPut, pCategoryImage}, get(pCategoryImage)},
		"category_image_remove": {{http.MethodDelete, pCategoryImage}, get(pCategoryImage)},
		"category_family_set":   {{http.MethodPatch, pCategory}, get(pCategoryImage)},
	} {
		mcpToolRoutes[name] = routes
	}
	for _, n := range []string{"product_image_set", "product_image_remove"} {
		mcpListTool[n] = "products_list"
	}
	for _, n := range []string{"category_image_set", "category_image_remove", "category_family_set"} {
		mcpListTool[n] = "categories_list"
	}
	register091Media()
}

type imageFiles struct {
	Image1200 string `json:"image_1200_base64" jsonschema:"WebP de 1200×1200 en base64 estándar, hasta 400 KB"`
	Image400  string `json:"image_400_base64" jsonschema:"WebP de 400×400 en base64 estándar, hasta 100 KB"`
}

// summary resume un archivo para la bitácora y la huella: su tamaño y su sha256.
func summary(b64 string) map[string]any {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		sum := sha256.Sum256([]byte(b64))
		return map[string]any{"invalid_base64": true, "sha256": hex.EncodeToString(sum[:])}
	}
	sum := sha256.Sum256(raw)
	return map[string]any{"bytes": len(raw), "sha256": hex.EncodeToString(sum[:])}
}

func (f imageFiles) logged() map[string]any {
	return map[string]any{"image_1200": summary(f.Image1200), "image_400": summary(f.Image400)}
}

// upload valida en el servidor, sin subir nada: la vista previa falla con el mismo mensaje que daría
// al aplicar.
func (f imageFiles) upload() (app.ImageUpload, error) {
	large, err1 := base64.StdEncoding.DecodeString(f.Image1200)
	small, err2 := base64.StdEncoding.DecodeString(f.Image400)
	if f.Image1200 == "" || f.Image400 == "" || err1 != nil || err2 != nil {
		return app.ImageUpload{}, toolErr("Manda image_1200_base64 e image_400_base64, cada una en base64 estándar.")
	}
	up := app.ImageUpload{Large: large, Small: small}
	if err := app.ValidateUpload(up); err != nil {
		return app.ImageUpload{}, toolErr("%s", operatorMessage(err.Error()))
	}
	return up, nil
}

type productImageSetArgs struct {
	writeCommon
	ProductID int64 `json:"product_id" jsonschema:"producto (de products_list)"`
	imageFiles
}

func (a productImageSetArgs) forLog() any {
	return map[string]any{"product_id": a.ProductID, "files": a.logged(), "dry_run": a.DryRun, "idempotency_key": a.IdempotencyKey}
}

type categoryImageSetArgs struct {
	writeCommon
	CategoryID int64 `json:"category_id" jsonschema:"categoría (de categories_list)"`
	imageFiles
}

func (a categoryImageSetArgs) forLog() any {
	return map[string]any{"category_id": a.CategoryID, "files": a.logged(), "dry_run": a.DryRun, "idempotency_key": a.IdempotencyKey}
}

type productImageRemoveArgs struct {
	writeCommon
	ProductID int64 `json:"product_id" jsonschema:"producto (de products_list)"`
}

type categoryImageRemoveArgs struct {
	writeCommon
	CategoryID int64 `json:"category_id" jsonschema:"categoría (de categories_list)"`
}

type categoryFamilyArgs struct {
	writeCommon
	CategoryID int64  `json:"category_id" jsonschema:"categoría (de categories_list)"`
	FoodFamily string `json:"food_family" jsonschema:"cold_drink, coffee, tea, frappe, soda, soup, snack, wings, dessert, breakfast o dish; vacío = genérica"`
}

func imageState(tool, pattern string) func(s *mcpSession, ctx context.Context, id int64) any {
	return func(s *mcpSession, ctx context.Context, id int64) any {
		return s.soft(ctx, tool, pattern, map[string]int64{"id": id}, nil)
	}
}

func register091Media() {
	prodState := imageState("product_image_set", pProductImage)
	writeTool(writeSpec[productImageSetArgs]{
		name: "product_image_set",
		route: func(_ *mcpSession, _ context.Context, in productImageSetArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("product_id", in.ProductID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if _, err := in.upload(); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPut, pProductImage, map[string]int64{"id": in.ProductID}, nil,
				imageBody{Image1200: in.Image1200, Image400: in.Image400}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in productImageSetArgs, _ json.RawMessage) any {
			return prodState(s, ctx, in.ProductID)
		},
		preview: func(_ *mcpSession, _ context.Context, in productImageSetArgs) (any, []string) {
			return map[string]any{"files": in.logged(), "checked": "tipo, medidas y peso: aceptados"}, nil
		},
	})

	removeState := imageState("product_image_remove", pProductImage)
	writeTool(writeSpec[productImageRemoveArgs]{
		name: "product_image_remove",
		route: func(_ *mcpSession, _ context.Context, in productImageRemoveArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("product_id", in.ProductID); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodDelete, pProductImage, map[string]int64{"id": in.ProductID}, nil, nil, nil
		},
		state: func(s *mcpSession, ctx context.Context, in productImageRemoveArgs, _ json.RawMessage) any {
			return removeState(s, ctx, in.ProductID)
		},
	})

	catState := imageState("category_image_set", pCategoryImage)
	writeTool(writeSpec[categoryImageSetArgs]{
		name: "category_image_set",
		route: func(_ *mcpSession, _ context.Context, in categoryImageSetArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("category_id", in.CategoryID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if _, err := in.upload(); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodPut, pCategoryImage, map[string]int64{"id": in.CategoryID}, nil,
				imageBody{Image1200: in.Image1200, Image400: in.Image400}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in categoryImageSetArgs, _ json.RawMessage) any {
			return catState(s, ctx, in.CategoryID)
		},
		preview: func(_ *mcpSession, _ context.Context, in categoryImageSetArgs) (any, []string) {
			return map[string]any{"files": in.logged(), "checked": "tipo, medidas y peso: aceptados"}, nil
		},
	})

	catRemoveState := imageState("category_image_remove", pCategoryImage)
	writeTool(writeSpec[categoryImageRemoveArgs]{
		name: "category_image_remove",
		route: func(_ *mcpSession, _ context.Context, in categoryImageRemoveArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("category_id", in.CategoryID); err != nil {
				return "", "", nil, nil, nil, err
			}
			return http.MethodDelete, pCategoryImage, map[string]int64{"id": in.CategoryID}, nil, nil, nil
		},
		state: func(s *mcpSession, ctx context.Context, in categoryImageRemoveArgs, _ json.RawMessage) any {
			return catRemoveState(s, ctx, in.CategoryID)
		},
	})

	famState := imageState("category_family_set", pCategoryImage)
	writeTool(writeSpec[categoryFamilyArgs]{
		name: "category_family_set",
		route: func(_ *mcpSession, _ context.Context, in categoryFamilyArgs) (string, string, map[string]int64, url.Values, any, error) {
			if err := positive("category_id", in.CategoryID); err != nil {
				return "", "", nil, nil, nil, err
			}
			if in.FoodFamily != "" && !domain.ValidFoodFamily(in.FoodFamily) {
				return "", "", nil, nil, nil, toolErr("food_family %q no es una familia: usa cold_drink, coffee, tea, frappe, soda, soup, snack, wings, dessert, breakfast o dish (o vacío).", in.FoodFamily)
			}
			return http.MethodPatch, pCategory, map[string]int64{"id": in.CategoryID}, nil,
				map[string]any{"foodFamily": in.FoodFamily}, nil
		},
		state: func(s *mcpSession, ctx context.Context, in categoryFamilyArgs, _ json.RawMessage) any {
			return famState(s, ctx, in.CategoryID)
		},
	})
}
