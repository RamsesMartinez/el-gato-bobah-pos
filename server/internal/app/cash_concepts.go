package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// CashConceptsService administra los conceptos de salida de caja (spec 032, punto 3).
type CashConceptsService struct{ store *store.Store }

// NewCashConceptsService construye el servicio.
func NewCashConceptsService(s *store.Store) *CashConceptsService {
	return &CashConceptsService{store: s}
}

// CashConceptView es un concepto ofrecido al capturar una salida.
type CashConceptView struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	CategoryID   *int64  `json:"categoryId"`
	CategoryName *string `json:"categoryName"`
	SupplierID   *int64  `json:"supplierId"`
	SupplierName *string `json:"supplierName"`
}

// List devuelve los conceptos activos.
func (s *CashConceptsService) List(ctx context.Context) ([]CashConceptView, error) {
	rows, err := s.store.QC(ctx).ListCashConcepts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CashConceptView, 0, len(rows))
	for _, r := range rows {
		out = append(out, CashConceptView{ID: r.ID, Name: r.Name, CategoryID: r.ExpenseCategoryID, CategoryName: r.CategoryName,
			SupplierID: r.SupplierID, SupplierName: r.SupplierName})
	}
	return out, nil
}

// Create agrega un concepto desde la captura. Si ya existe uno activo con el mismo nombre (sin
// mayúsculas ni espacios extremos) devuelve ése: dos tabletas que escriben «hielo» a la vez no
// crean dos conceptos.
func (s *CashConceptsService) Create(ctx context.Context, name string) (CashConceptView, error) {
	n, err := domain.NormalizeConceptName(name)
	if err != nil {
		return CashConceptView{}, err
	}
	q := s.store.QC(ctx)
	if c, err := q.FindActiveCashConceptByName(ctx, n); err == nil {
		return CashConceptView{ID: c.ID, Name: c.Name}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return CashConceptView{}, err
	}
	c, err := q.InsertCashConcept(ctx, n)
	if isUniqueViolation(err) {
		if c2, err2 := q.FindActiveCashConceptByName(ctx, n); err2 == nil {
			return CashConceptView{ID: c2.ID, Name: c2.Name}, nil
		}
	}
	if err != nil {
		return CashConceptView{}, err
	}
	return CashConceptView{ID: c.ID, Name: c.Name}, nil
}

// ConceptUpdate es la edición desde Configuración.
type ConceptUpdate struct {
	Name       string `json:"name"`
	CategoryID *int64 `json:"categoryId"`
	SupplierID *int64 `json:"supplierId"`
}

// Update renombra un concepto y lo liga a una categoría y un proveedor recurrente.
func (s *CashConceptsService) Update(ctx context.Context, id int64, in ConceptUpdate) (CashConceptView, error) {
	n, err := domain.NormalizeConceptName(in.Name)
	if err != nil {
		return CashConceptView{}, err
	}
	c, err := s.store.QC(ctx).UpdateCashConcept(ctx, db.UpdateCashConceptParams{ID: id, Name: n,
		ExpenseCategoryID: in.CategoryID, SupplierID: in.SupplierID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return CashConceptView{}, domain.ErrNotFound
	case isUniqueViolation(err):
		return CashConceptView{}, fmt.Errorf("%w: ya hay un concepto con ese nombre", domain.ErrConflict)
	case isFKViolation(err):
		return CashConceptView{}, fmt.Errorf("%w: esa categoría o proveedor no existe", domain.ErrValidation)
	case err != nil:
		return CashConceptView{}, err
	}
	return CashConceptView{ID: c.ID, Name: c.Name}, nil
}

// Archive deja de ofrecer un concepto; sus salidas lo conservan.
func (s *CashConceptsService) Archive(ctx context.Context, id int64) error {
	n, err := s.store.QC(ctx).ArchiveCashConcept(ctx, id)
	if err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return err
}

// Merge junta un duplicado en otro concepto activo: sus salidas pasan al que queda y el duplicado
// se archiva. Solo hacia un concepto activo, así una cadena de fusiones no puede cerrar un ciclo.
func (s *CashConceptsService) Merge(ctx context.Context, fromID, intoID int64) error {
	if fromID == intoID {
		return fmt.Errorf("%w: un concepto no se junta consigo mismo", domain.ErrValidation)
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		into, err := q.GetCashConcept(ctx, intoID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && into.ArchivedAt.Valid) {
			return fmt.Errorf("%w: el concepto que queda debe estar activo", domain.ErrValidation)
		}
		if err != nil {
			return err
		}
		n, err := q.MergeCashConcept(ctx, db.MergeCashConceptParams{ID: fromID, IntoID: &intoID})
		if err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNotFound
		}
		return q.MoveCashOutsToConcept(ctx, db.MoveCashOutsToConceptParams{FromID: &fromID, IntoID: &intoID})
	})
}

func isFKViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23503"
}
