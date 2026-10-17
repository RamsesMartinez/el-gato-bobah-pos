package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// CountPlace es un lugar del recorrido del conteo (spec 040). No es un almacén: solo ordena.
type CountPlace struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Position    int32  `json:"position"`
	Ingredients int32  `json:"ingredients"`
}

// ListCountPlaces son los lugares que se ofrecen, en el orden del recorrido.
func (s *BackofficeService) ListCountPlaces(ctx context.Context) ([]CountPlace, error) {
	rows, err := s.store.QC(ctx).ListCountPlaces(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CountPlace, 0, len(rows))
	for _, r := range rows {
		out = append(out, CountPlace{ID: r.ID, Name: r.Name, Position: r.Position, Ingredients: r.Ingredients})
	}
	return out, nil
}

// CreateCountPlace da de alta un lugar al final del recorrido. Si ya hay uno con ese nombre (sin
// distinguir mayúsculas, acentos ni espacios), responde cuál es para que la pantalla lo elija.
func (s *BackofficeService) CreateCountPlace(ctx context.Context, name string) (int64, error) {
	if err := domain.ValidCountPlaceName(name); err != nil {
		return 0, err
	}
	name = domain.CleanCountPlaceName(name)
	var id int64
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCountPlaces(ctx); err != nil {
			return err
		}
		if err := placeNameFree(ctx, q, name); err != nil {
			return err
		}
		var err error
		id, err = q.CreateCountPlace(ctx, name)
		return placeNameConflict(err)
	})
	return id, err
}

// RenameCountPlace cambia el nombre que muestran todos sus artículos.
func (s *BackofficeService) RenameCountPlace(ctx context.Context, id int64, name string) error {
	if err := domain.ValidCountPlaceName(name); err != nil {
		return err
	}
	name = domain.CleanCountPlaceName(name)
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCountPlaces(ctx); err != nil {
			return err
		}
		if existing, err := q.CountPlaceByName(ctx, name); err == nil && existing != id {
			return domain.CountPlaceExists{ID: existing}
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		n, err := q.RenameCountPlace(ctx, db.RenameCountPlaceParams{ID: id, Name: name})
		if err = placeNameConflict(err); err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// MoveCountPlace pone el lugar en la posición to del recorrido (0 = primero) y renumera los demás.
func (s *BackofficeService) MoveCountPlace(ctx context.Context, id int64, to int) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCountPlaces(ctx); err != nil {
			return err
		}
		order, err := q.ActiveCountPlaceIDs(ctx)
		if err != nil {
			return err
		}
		next, err := domain.MoveCountPlace(order, id, to)
		if err != nil {
			return err
		}
		return renumberPlaces(ctx, q, next)
	})
}

// ArchiveCountPlace archiva un lugar sin artículos. Con artículos responde cuántos tiene: hay que
// decir a dónde pasan con ArchiveCountPlaceMoving.
func (s *BackofficeService) ArchiveCountPlace(ctx context.Context, id int64) error {
	return s.archivePlace(ctx, id, nil, false)
}

// ArchiveCountPlaceMoving archiva el lugar y pasa sus artículos, activos o no, a dest (nil = sin
// lugar), en la misma transacción.
func (s *BackofficeService) ArchiveCountPlaceMoving(ctx context.Context, id int64, dest *int64) error {
	return s.archivePlace(ctx, id, dest, true)
}

func (s *BackofficeService) archivePlace(ctx context.Context, id int64, dest *int64, decided bool) error {
	if dest != nil && *dest == id {
		return fmt.Errorf("%w: los artículos no pueden pasar al lugar que se archiva", domain.ErrValidation)
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockCountPlaces(ctx); err != nil {
			return err
		}
		// for update: una asignación en curso (for share) termina antes, y una nueva espera y ve el
		// lugar archivado. Así ningún artículo queda en uno que ya no se ofrece.
		p, err := q.LockCountPlaceForUpdate(ctx, id)
		if err != nil {
			return notFound(err)
		}
		if p.ArchivedAt.Valid {
			return nil
		}
		if dest != nil {
			if err := activePlace(ctx, q, *dest); err != nil {
				return err
			}
		}
		n, err := q.CountPlaceIngredients(ctx, &id)
		if err != nil {
			return err
		}
		if n > 0 && !decided {
			return domain.CountPlaceHasIngredients{Ingredients: n}
		}
		if n > 0 {
			if err := q.MoveCountPlaceIngredients(ctx, db.MoveCountPlaceIngredientsParams{FromPlaceID: &id, ToPlaceID: dest}); err != nil {
				return err
			}
		}
		if err := q.ArchiveCountPlace(ctx, id); err != nil {
			return err
		}
		order, err := q.ActiveCountPlaceIDs(ctx)
		if err != nil {
			return err
		}
		return renumberPlaces(ctx, q, order)
	})
}

// SetCountPlace pone el artículo en un lugar de la lista, o sin lugar con nil.
func (s *BackofficeService) SetCountPlace(ctx context.Context, ingredientID int64, placeID *int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if placeID != nil {
			if err := activePlace(ctx, q, *placeID); err != nil {
				return err
			}
		}
		n, err := q.SetCountPlace(ctx, db.SetCountPlaceParams{ID: ingredientID, PlaceID: placeID})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// activePlace toma el lugar for share y exige que se siga ofreciendo.
func activePlace(ctx context.Context, q *db.Queries, id int64) error {
	p, err := q.LockCountPlaceForShare(ctx, id)
	if err != nil {
		return notFound(err)
	}
	if p.ArchivedAt.Valid {
		return fmt.Errorf("%w: ese lugar ya no se ofrece", domain.ErrConflict)
	}
	return nil
}

func placeNameFree(ctx context.Context, q *db.Queries, name string) error {
	existing, err := q.CountPlaceByName(ctx, name)
	if err == nil {
		return domain.CountPlaceExists{ID: existing}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

// placeNameConflict convierte el 23505 del índice en conflicto. Bajo el candado por empresa la
// búsqueda previa ya encontró el repetido; el índice queda como segunda barrera.
func placeNameConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: ese lugar ya existe", domain.ErrConflict)
	}
	return err
}

func renumberPlaces(ctx context.Context, q *db.Queries, order []int64) error {
	for i, id := range order {
		if err := q.SetCountPlacePosition(ctx, db.SetCountPlacePositionParams{ID: id, Position: int32(i)}); err != nil {
			return err
		}
	}
	return nil
}
