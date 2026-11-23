package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/objstore"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// CatalogMediaService pone y quita las fotos de productos y categorías, y la familia de comida de
// una categoría (spec 091).
//
// El orden importa y es el mismo en todos los caminos: comprobar que la entidad es de la empresa,
// validar, subir, guardar la URL en una transacción y, ya confirmada, borrar los archivos viejos. Así
// un fallo a la mitad nunca deja un registro que apunta a un archivo que no existe; lo peor que
// deja es un archivo sin usar, que queda anotado para borrarse después.
type CatalogMediaService struct {
	store   *store.Store
	objects *objstore.Client // nil = fotos apagadas en este despliegue
	baseURL string
}

// NewCatalogMediaService arma el servicio. objects nil apaga las escrituras de fotos.
func NewCatalogMediaService(s *store.Store, objects *objstore.Client, baseURL string) *CatalogMediaService {
	return &CatalogMediaService{store: s, objects: objects, baseURL: strings.TrimRight(baseURL, "/")}
}

// ImageUpload son las dos variantes ya preparadas por el navegador o por la computadora del catálogo.
type ImageUpload struct {
	Large []byte // 1200×1200
	Small []byte // 400×400
}

// Enabled dice si este despliegue puede guardar fotos.
func (s *CatalogMediaService) Enabled() bool { return s.objects != nil }

// ValidateUpload aplica las reglas del servidor sin subir nada: la vista previa del MCP la usa.
func ValidateUpload(up ImageUpload) error {
	if err := domain.ValidateWebP(up.Large, domain.MediaLargeSide, domain.MaxMediaLargeBytes); err != nil {
		return err
	}
	return domain.ValidateWebP(up.Small, domain.MediaSmallSide, domain.MaxMediaSmallBytes)
}

// maxSweep es cuántos borrados pendientes se reintentan al terminar cada escritura: acota lo que una
// subida puede tardar de más cuando el almacenamiento estuvo caído un rato.
const maxSweep = 10

type mediaEntity struct {
	kind  domain.MediaKind
	lock  func(q *db.Queries, ctx context.Context, id int64) (*string, error)
	save  func(q *db.Queries, ctx context.Context, id int64, url *string) error
	set   domain.AuditAction
	unset domain.AuditAction
	label string
}

var mediaEntities = map[domain.MediaKind]mediaEntity{
	domain.MediaProducts: {
		kind: domain.MediaProducts, label: "product",
		lock: func(q *db.Queries, ctx context.Context, id int64) (*string, error) {
			return q.LockProductImage(ctx, id)
		},
		save: func(q *db.Queries, ctx context.Context, id int64, url *string) error {
			return q.SetProductImage(ctx, db.SetProductImageParams{ID: id, ImageUrl: url})
		},
		set: domain.AuditProductImageSet, unset: domain.AuditProductImageRemoved,
	},
	domain.MediaCategories: {
		kind: domain.MediaCategories, label: "category",
		lock: func(q *db.Queries, ctx context.Context, id int64) (*string, error) {
			return q.LockCategoryImage(ctx, id)
		},
		save: func(q *db.Queries, ctx context.Context, id int64, url *string) error {
			return q.SetCategoryImage(ctx, db.SetCategoryImageParams{ID: id, ImageUrl: url})
		},
		set: domain.AuditCategoryImageSet, unset: domain.AuditCategoryImageRemoved,
	},
}

func entityOf(kind domain.MediaKind) (mediaEntity, error) {
	e, ok := mediaEntities[kind]
	if !ok {
		return mediaEntity{}, fmt.Errorf("%w: clase de foto desconocida", domain.ErrValidation)
	}
	return e, nil
}

func mediaNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: no existe", domain.ErrNotFound)
	}
	return err
}

// cleanupTimeout acota la limpieza que sigue al commit. Corre con un contexto que no se cancela si
// quien subió se va (cerró la tableta, se cayó la red): ya se guardó la foto, y lo viejo tiene que
// borrarse o quedar anotado de todos modos.
const cleanupTimeout = 20 * time.Second

func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
}

// lockEntity serializa las escrituras de foto de una entidad desde antes de subir hasta después de
// borrar lo viejo. Sin él, dos subidas a la vez se borran una a la otra: A sube Y, B vuelve a subir
// X, A confirma y borra X, B confirma X, y el producto queda apuntando a un archivo que ya no existe.
// El candado es de sesión sobre la conexión que el request ya tiene apartada para su empresa.
func (s *CatalogMediaService) lockEntity(ctx context.Context, company int64, kind domain.MediaKind, id int64) (func(), error) {
	key := fmt.Sprintf("media:%d/%s/%d", company, kind, id)
	q := s.store.QC(ctx)
	if err := q.LockMediaEntity(ctx, key); err != nil {
		return nil, err
	}
	return func() {
		uctx, cancel := detached(ctx)
		defer cancel()
		if err := q.UnlockMediaEntity(uctx, key); err != nil {
			slog.ErrorContext(ctx, "media_unlock_failed", "entity", key, "error", err.Error())
		}
	}, nil
}

// SetImage pone o reemplaza la foto y devuelve su URL pública.
func (s *CatalogMediaService) SetImage(ctx context.Context, kind domain.MediaKind, id int64, up ImageUpload, actor int64) (string, error) {
	if s.objects == nil {
		return "", domain.ErrMediaDisabled
	}
	e, err := entityOf(kind)
	if err != nil {
		return "", err
	}
	company, ok := store.CompanyFrom(ctx)
	if !ok {
		return "", domain.ErrUnauthorized
	}
	if err := ValidateUpload(up); err != nil {
		return "", err
	}
	// Primero que exista y sea de la empresa (RLS): a un id ajeno no se le sube nada.
	if _, err := s.Info(ctx, kind, id); err != nil {
		return "", err
	}
	unlock, err := s.lockEntity(ctx, company, kind, id)
	if err != nil {
		return "", err
	}
	defer unlock()
	current, err := s.Info(ctx, kind, id)
	if err != nil {
		return "", err
	}

	hash := domain.MediaHash(up.Large)
	small := domain.MediaObjectKey(company, kind, id, hash, domain.MediaSmallSide)
	large := domain.MediaObjectKey(company, kind, id, hash, domain.MediaLargeSide)
	newURL := domain.MediaURL(s.baseURL, large)
	same := current.ImageURL != nil && *current.ImageURL == newURL

	// La chica primero: si la grande falla, ninguna URL apunta todavía a esta foto, y la chica se
	// borra (salvo que sea la de la foto vigente, subida otra vez).
	if err := s.objects.Put(ctx, small, up.Small, domain.MediaContentType, domain.MediaCacheControl); err != nil {
		slog.WarnContext(ctx, "media_put_failed", "kind", kind, "id", id, "error", err.Error())
		return "", fmt.Errorf("%w: %w", domain.ErrMediaUnavailable, err)
	}
	if err := s.objects.Put(ctx, large, up.Large, domain.MediaContentType, domain.MediaCacheControl); err != nil {
		slog.WarnContext(ctx, "media_put_failed", "kind", kind, "id", id, "error", err.Error())
		if !same {
			s.deleteKeys(ctx, []string{small})
		}
		return "", fmt.Errorf("%w: %w", domain.ErrMediaUnavailable, err)
	}
	if same {
		return newURL, nil
	}

	var old string
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		cur, err := e.lock(q, ctx, id)
		if err != nil {
			return err
		}
		if cur != nil {
			old = *cur
		}
		if err := e.save(q, ctx, id, &newURL); err != nil {
			return err
		}
		// Si estas llaves esperaban borrarse (se regresó a una foto de antes), ya no.
		if err := q.DeleteMediaPendingByKeys(ctx, []string{small, large}); err != nil {
			return err
		}
		return auditEntity(ctx, q, e.set, e.label, id, nil, &actor, urlOrNil(old), newURL)
	})
	if err != nil {
		// Un error del commit puede ser ambiguo (se aplicó y se perdió la respuesta): solo se borra lo
		// recién subido si de verdad no quedó como la foto.
		cctx, cancel := detached(ctx)
		defer cancel()
		if info, ierr := s.Info(cctx, kind, id); ierr == nil && (info.ImageURL == nil || *info.ImageURL != newURL) {
			s.deleteKeys(ctx, []string{small, large})
		}
		return "", mediaNotFound(err)
	}
	s.deleteOld(ctx, company, e.kind, id, old)
	s.sweep(ctx)
	return newURL, nil
}

// RemoveImage quita la foto. Sin foto no hace nada y no falla.
func (s *CatalogMediaService) RemoveImage(ctx context.Context, kind domain.MediaKind, id int64, actor int64) error {
	e, err := entityOf(kind)
	if err != nil {
		return err
	}
	company, ok := store.CompanyFrom(ctx)
	if !ok {
		return domain.ErrUnauthorized
	}
	if _, err := s.Info(ctx, kind, id); err != nil {
		return err
	}
	unlock, err := s.lockEntity(ctx, company, kind, id)
	if err != nil {
		return err
	}
	defer unlock()
	var old string
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		cur, err := e.lock(q, ctx, id)
		if err != nil {
			return err
		}
		if cur == nil {
			return nil
		}
		old = *cur
		if err := e.save(q, ctx, id, nil); err != nil {
			return err
		}
		return auditEntity(ctx, q, e.unset, e.label, id, nil, &actor, old, nil)
	})
	if err != nil {
		return mediaNotFound(err)
	}
	if old != "" && s.objects != nil {
		s.deleteOld(ctx, company, e.kind, id, old)
		s.sweep(ctx)
	}
	return nil
}

// SetCategoryFamily elige la familia de una categoría; "" vuelve a la genérica.
func (s *CatalogMediaService) SetCategoryFamily(ctx context.Context, id int64, family string, actor int64) error {
	var next *db.FoodFamily
	if family != "" {
		if !domain.ValidFoodFamily(family) {
			return fmt.Errorf("%w: familia de comida desconocida", domain.ErrValidation)
		}
		f := db.FoodFamily(family)
		next = &f
	}
	return mediaNotFound(s.store.WithTx(ctx, func(q *db.Queries) error {
		prev, err := q.LockCategoryFamily(ctx, id)
		if err != nil {
			return err
		}
		if prev == family {
			return nil
		}
		if err := q.SetCategoryFoodFamily(ctx, db.SetCategoryFoodFamilyParams{ID: id, FoodFamily: next}); err != nil {
			return err
		}
		return auditEntity(ctx, q, domain.AuditCategoryFamilySet, "category", id, nil, &actor, prev, family)
	}))
}

func urlOrNil(u string) any {
	if u == "" {
		return nil
	}
	return u
}

// deleteOld borra las dos variantes de la foto anterior, solo si la armó esta API para esta misma
// empresa y entidad. Una URL escrita a mano o de otro producto se deja: borrarla sería borrar un
// archivo ajeno.
func (s *CatalogMediaService) deleteOld(ctx context.Context, company int64, kind domain.MediaKind, id int64, old string) {
	if old == "" {
		return
	}
	keys, ok := domain.MediaKeysFromURL(s.baseURL, old, company, kind, id)
	if !ok {
		return
	}
	s.deleteKeys(ctx, keys)
}

// deleteKeys borra; lo que falla queda pendiente. Nunca regresa error: la foto ya está como debe.
func (s *CatalogMediaService) deleteKeys(ctx context.Context, keys []string) {
	ctx, cancel := detached(ctx)
	defer cancel()
	for _, k := range keys {
		if err := s.objects.Delete(ctx, k); err != nil {
			slog.WarnContext(ctx, "media_delete_failed", "key", k, "error", err.Error())
			if perr := s.store.QC(ctx).AddMediaPendingDeletion(ctx, db.AddMediaPendingDeletionParams{
				ObjectKey: k, LastError: shortError(err)}); perr != nil {
				slog.ErrorContext(ctx, "media_pending_save_failed", "key", k, "error", perr.Error())
			}
		}
	}
}

// sweep reintenta los borrados pendientes de la empresa de la sesión (RLS los acota).
func (s *CatalogMediaService) sweep(ctx context.Context) {
	ctx, cancel := detached(ctx)
	defer cancel()
	q := s.store.QC(ctx)
	rows, err := q.ListMediaPendingDeletions(ctx, maxSweep)
	if err != nil {
		slog.WarnContext(ctx, "media_sweep_list_failed", "error", err.Error())
		return
	}
	for _, r := range rows {
		if err := s.objects.Delete(ctx, r.ObjectKey); err != nil {
			_ = q.BumpMediaPendingDeletion(ctx, db.BumpMediaPendingDeletionParams{ID: r.ID, LastError: shortError(err)})
			continue
		}
		if err := q.DeleteMediaPendingDeletion(ctx, r.ID); err != nil {
			slog.WarnContext(ctx, "media_sweep_delete_row_failed", "id", r.ID, "error", err.Error())
		}
	}
}

// shortError recorta el error del almacenamiento al tope de la columna y le quita cualquier
// querystring (una URL firmada lleva la credencial en ella).
func shortError(err error) string {
	msg := err.Error()
	for _, f := range strings.Fields(msg) {
		if u, perr := url.Parse(f); perr == nil && u.RawQuery != "" {
			msg = strings.ReplaceAll(msg, "?"+u.RawQuery, "")
		}
	}
	if r := []rune(msg); len(r) > 300 {
		msg = string(r[:300])
	}
	return msg
}

// ImageInfo es la foto actual de un producto o categoría, para la vista previa del MCP.
type ImageInfo struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	ImageURL   *string `json:"imageUrl"`
	FoodFamily *string `json:"foodFamily,omitempty"` // solo categorías; "" = genérica
}

// Info lee la foto actual. Un id de otra empresa es «no existe» (RLS).
func (s *CatalogMediaService) Info(ctx context.Context, kind domain.MediaKind, id int64) (ImageInfo, error) {
	q := s.store.QC(ctx)
	switch kind {
	case domain.MediaProducts:
		r, err := q.ProductImageInfo(ctx, id)
		if err != nil {
			return ImageInfo{}, mediaNotFound(err)
		}
		return ImageInfo{ID: r.ID, Name: r.Name, ImageURL: r.ImageUrl}, nil
	case domain.MediaCategories:
		r, err := q.CategoryImageInfo(ctx, id)
		if err != nil {
			return ImageInfo{}, mediaNotFound(err)
		}
		return ImageInfo{ID: r.ID, Name: r.Name, ImageURL: r.ImageUrl, FoodFamily: &r.FoodFamily}, nil
	}
	return ImageInfo{}, fmt.Errorf("%w: clase de foto desconocida", domain.ErrValidation)
}
