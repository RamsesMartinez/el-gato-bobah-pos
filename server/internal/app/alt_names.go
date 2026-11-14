package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Nombres alternativos (spec 063). El choque se decide dentro de la transacción y bajo el candado de
// nombres del catálogo, tomado antes de leer cualquier fila.

// maxSimilarAlt es cuántos parecidos se ofrecen al escribir un alternativo (FR-020).
const maxSimilarAlt = 5

// SetProductAltNames reemplaza los alternativos de un producto y devuelve la lista como quedó.
func (s *AdminService) SetProductAltNames(ctx context.Context, id int64, names []string) ([]string, error) {
	var out []string
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockProductNames(ctx); err != nil {
			return err
		}
		own, err := q.GetProductForAltNames(ctx, id)
		if err != nil {
			return notFound(err)
		}
		clean, err := domain.CleanAltNames(own, names)
		if err != nil {
			return err
		}
		if len(clean) > 0 {
			switch c, err := q.FirstProductNameClash(ctx, db.FirstProductNameClashParams{Names: clean, ID: id}); {
			case err == nil:
				return domain.AltNameTaken{Name: c.Name, OwnerID: c.OwnerID, OwnerName: c.OwnerName, OwnerArchived: !c.OwnerActive, OwnerAlt: c.ViaAlt}
			case !errors.Is(err, pgx.ErrNoRows):
				return err
			}
		}
		if _, err := q.SetProductAltNames(ctx, db.SetProductAltNamesParams{Names: clean, ID: id}); err != nil {
			return err
		}
		out = clean
		return nil
	})
	return out, err
}

// productNameFree rechaza un nombre principal que ya es alternativo de un producto (FR-005). Va
// después de LockProductNames.
func productNameFree(ctx context.Context, q *db.Queries, name string) error {
	switch o, err := q.ProductAltNameOwner(ctx, name); {
	case err == nil:
		return domain.AltNameTaken{Name: name, OwnerID: o.ID, OwnerName: o.Name, OwnerArchived: !o.IsActive, OwnerAlt: true}
	case !errors.Is(err, pgx.ErrNoRows):
		return err
	}
	return nil
}

// SimilarProductAltNames son los productos con nombre o alternativo parecido a `name`, sin `id`.
func (s *AdminService) SimilarProductAltNames(ctx context.Context, id int64, name string) ([]domain.SimilarAlt, error) {
	if err := validSimilarQuery(name); err != nil {
		return nil, err
	}
	rows, err := s.store.QC(ctx).ProductAltNameCandidates(ctx)
	if err != nil {
		return nil, err
	}
	cands := []domain.AltCandidate{}
	for _, r := range rows {
		cands = append(cands, domain.AltCandidate{ID: r.ID, Name: r.Name})
		for _, a := range r.AltNames {
			cands = append(cands, domain.AltCandidate{ID: r.ID, Name: r.Name, Alt: a})
		}
	}
	return domain.SimilarAltNames(name, id, cands, maxSimilarAlt), nil
}

// ArticleAltNames es lo que queda tras guardar: la lista y la versión nueva de la ficha. El trigger de
// la 0009 mueve updated_at en todo update del artículo, así que la ficha que guardó sigue editando con
// ésta (como tras la densidad o el costo); otra tableta con la ficha abierta recibe STALE y recarga.
type ArticleAltNames struct {
	AltNames  []string  `json:"altNames"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// SetArticleAltNames reemplaza los alternativos de un artículo del almacén. Deja el cambio en el
// evento de edición de la ficha, en la misma transacción.
//
// expected es la versión de la ficha que vio la pantalla: con otra, STALE. Sin esto, guardar
// alternativos sobre una ficha que otra tableta renombró le daba a la pantalla la versión nueva, y su
// siguiente autoguardado (que manda la ficha completa) deshacía el renombre sin aviso.
func (s *BackofficeService) SetArticleAltNames(ctx context.Context, id int64, names []string, expected time.Time) (ArticleAltNames, error) {
	var out ArticleAltNames
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		var err error
		out, err = setArticleAltNamesIn(ctx, q, id, func([]string) []string { return names }, &expected)
		return err
	})
	return out, err
}

// setArticleAltNamesIn guarda los otros nombres de un artículo dentro de una transacción ajena, bajo
// el candado de nombres y con la regla de choque de siempre. names recibe la lista de hoy y devuelve
// la nueva; expected nil no compara versión (agregar un nombre desde «No lo encuentro» parte de la
// lista leída bajo el candado, así que no pisa nada).
func setArticleAltNamesIn(ctx context.Context, q *db.Queries, id int64, names func([]string) []string, expected *time.Time) (ArticleAltNames, error) {
	var out ArticleAltNames
	err := func() error {
		if err := q.LockIngredientNames(ctx); err != nil {
			return err
		}
		art, err := q.GetArticleForAltNames(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if expected != nil && !art.UpdatedAt.Equal(*expected) {
			return domain.ErrArticleStale
		}
		clean, err := domain.CleanAltNames(art.Name, names(art.AltNames))
		if err != nil {
			return err
		}
		if len(clean) > 0 {
			switch c, err := q.FirstArticleNameClash(ctx, db.FirstArticleNameClashParams{Names: clean, ID: id}); {
			case err == nil:
				return domain.AltNameTaken{Name: c.Name, OwnerID: c.OwnerID, OwnerName: c.OwnerName, OwnerArchived: !c.OwnerActive, OwnerAlt: c.ViaAlt, Article: true}
			case !errors.Is(err, pgx.ErrNoRows):
				return err
			}
		}
		updatedAt, err := q.SetArticleAltNames(ctx, db.SetArticleAltNamesParams{Names: clean, ID: id})
		if err != nil {
			return err
		}
		out = ArticleAltNames{AltNames: clean, UpdatedAt: updatedAt}
		before := art.AltNames
		if before == nil {
			before = []string{}
		}
		if strings.Join(before, "\x00") == strings.Join(clean, "\x00") {
			return nil
		}
		return recordAudit(ctx, q, domain.AuditEvent{
			Action: domain.AuditIngredientUpdated, EntityType: "ingredient", EntityID: id,
			Before: map[string]any{"altNames": before}, After: map[string]any{"altNames": clean},
		})
	}()
	return out, err
}

// SimilarArticleAltNames son los artículos con nombre o alternativo parecido a `name`, sin `id` ni
// los orígenes de una unión.
func (s *BackofficeService) SimilarArticleAltNames(ctx context.Context, id int64, name string) ([]domain.SimilarAlt, error) {
	if err := validSimilarQuery(name); err != nil {
		return nil, err
	}
	rows, err := s.store.QC(ctx).ArticleAltNameCandidates(ctx)
	if err != nil {
		return nil, err
	}
	cands := []domain.AltCandidate{}
	for _, r := range rows {
		cands = append(cands, domain.AltCandidate{ID: r.ID, Name: r.Name})
		for _, a := range r.AltNames {
			cands = append(cands, domain.AltCandidate{ID: r.ID, Name: r.Name, Alt: a})
		}
	}
	return domain.SimilarAltNames(name, id, cands, maxSimilarAlt), nil
}

func validSimilarQuery(name string) error {
	if n := utf8.RuneCountInString(strings.TrimSpace(name)); n == 0 || n > domain.MaxAltName || !domain.PrintableName(name) {
		return fmt.Errorf("%w: el nombre va de 1 a %d letras", domain.ErrValidation, domain.MaxAltName)
	}
	return nil
}
