package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// CompanyService gestiona la empresa (tenant) del usuario autenticado. Bajo RLS (policy
// company_self) el store solo ve/edita la fila de la propia empresa: no puede tocar otra.
type CompanyService struct {
	store *store.Store
}

func NewCompanyService(s *store.Store) *CompanyService { return &CompanyService{store: s} }

func (s *CompanyService) Get(ctx context.Context, companyID int64) (domain.Company, error) {
	co, err := s.store.QC(ctx).GetCompany(ctx, companyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Company{}, domain.ErrNotFound
		}
		return domain.Company{}, err
	}
	return domain.Company{ID: co.ID, Slug: co.Slug, Name: co.Name, IsActive: co.IsActive}, nil
}

// Update cambia nombre y slug de la empresa. Valida el formato del slug y mapea el choque de
// unicidad (otra empresa ya lo usa) a ErrConflict. RLS with-check impide editar otra empresa.
func (s *CompanyService) Update(ctx context.Context, companyID int64, name, slug string) (domain.Company, error) {
	if name == "" || !domain.ValidSlug(slug) {
		return domain.Company{}, domain.ErrValidation
	}
	co, err := s.store.QC(ctx).UpdateCompany(ctx, db.UpdateCompanyParams{ID: companyID, Name: name, Slug: slug})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return domain.Company{}, domain.ErrConflict
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Company{}, domain.ErrNotFound
		}
		return domain.Company{}, err
	}
	return domain.Company{ID: co.ID, Slug: co.Slug, Name: co.Name, IsActive: co.IsActive}, nil
}
