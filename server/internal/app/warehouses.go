package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// Warehouse es un lugar de la sucursal donde se guarda mercancía (spec 094).
type Warehouse struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	IsSales        bool   `json:"isSales"`
	IsActive       bool   `json:"isActive"`
	ItemsWithStock int64  `json:"itemsWithStock"`
}

// ListWarehouses son los almacenes de la sucursal, el de venta primero. Siempre un arreglo.
func (s *BackofficeService) ListWarehouses(ctx context.Context, includeArchived bool) ([]Warehouse, error) {
	rows, err := s.store.QC(ctx).ListWarehouses(ctx, includeArchived)
	if err != nil {
		return nil, err
	}
	out := make([]Warehouse, 0, len(rows))
	for _, r := range rows {
		out = append(out, Warehouse{ID: r.ID, Name: r.Name, IsSales: r.IsSales, IsActive: r.IsActive, ItemsWithStock: r.ItemsWithStock})
	}
	return out, nil
}

// WarehouseName es un almacén activo como lo ve quien solo elige dónde registrar.
type WarehouseName struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	IsSales bool   `json:"isSales"`
}

// WarehouseNames lista los almacenes activos de la sucursal sin contar existencias.
func (s *BackofficeService) WarehouseNames(ctx context.Context) ([]WarehouseName, error) {
	rows, err := s.store.QC(ctx).ListWarehouseNames(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]WarehouseName, 0, len(rows))
	for _, r := range rows {
		out = append(out, WarehouseName{ID: r.ID, Name: r.Name, IsSales: r.IsSales})
	}
	return out, nil
}

// CreateWarehouse da de alta un almacén vacío en la sucursal de la sesión.
func (s *BackofficeService) CreateWarehouse(ctx context.Context, name string) (int64, error) {
	if err := domain.ValidWarehouseName(name); err != nil {
		return 0, err
	}
	name = domain.CleanWarehouseName(name)
	var id int64
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockWarehouses(ctx); err != nil {
			return err
		}
		if err := warehouseNameFree(ctx, q, name, 0); err != nil {
			return err
		}
		var err error
		if id, err = q.CreateWarehouse(ctx, name); err != nil {
			return warehouseNameConflict(err)
		}
		return auditWarehouse(ctx, q, domain.AuditWarehouseCreated, id, nil, map[string]any{"name": name})
	})
	return id, err
}

// RenameWarehouse cambia el nombre de un almacén activo de la sucursal.
func (s *BackofficeService) RenameWarehouse(ctx context.Context, id int64, name string) error {
	return s.UpdateWarehouse(ctx, id, &name, false)
}

func renameWarehouseTx(ctx context.Context, q *db.Queries, id int64, name string) error {
	if err := q.LockWarehouses(ctx); err != nil {
		return err
	}
	w, err := q.LockWarehouse(ctx, id)
	if err != nil {
		return warehouseNotFound(err)
	}
	if !w.IsActive {
		return domain.ErrWarehouseNotFound
	}
	if w.Name == name {
		return nil
	}
	if err := warehouseNameFree(ctx, q, name, id); err != nil {
		return err
	}
	if err := q.RenameWarehouse(ctx, db.RenameWarehouseParams{ID: id, Name: name}); err != nil {
		return warehouseNameConflict(err)
	}
	return auditWarehouse(ctx, q, domain.AuditWarehouseUpdated, id, map[string]any{"name": w.Name}, map[string]any{"name": name})
}

// SetSalesWarehouse hace de este almacén el de venta de la sucursal: de él salen las ventas desde ese
// momento. Lo ya vendido se queda en el almacén del que salió, y una cancelación repone ahí.
func (s *BackofficeService) SetSalesWarehouse(ctx context.Context, id int64) error {
	return s.UpdateWarehouse(ctx, id, nil, true)
}

func setSalesWarehouseTx(ctx context.Context, q *db.Queries, id int64) error {
	if err := q.LockWarehouses(ctx); err != nil {
		return err
	}
	w, err := q.LockWarehouse(ctx, id)
	if err != nil {
		return warehouseNotFound(err)
	}
	if !w.IsActive {
		return domain.ErrWarehouseNotFound
	}
	if w.IsSales {
		return nil
	}
	before, err := q.SalesWarehouse(ctx)
	if err != nil {
		return err
	}
	if err := q.ClearSalesWarehouse(ctx); err != nil {
		return err
	}
	if err := q.MarkSalesWarehouse(ctx, id); err != nil {
		return err
	}
	return auditWarehouse(ctx, q, domain.AuditWarehouseUpdated, id,
		map[string]any{"salesWarehouse": before.Name}, map[string]any{"salesWarehouse": w.Name})
}

// ArchiveWarehouse archiva un almacén vacío, que no es el de venta y sin conteo abierto. Ya
// archivado no hace nada: un reintento de la tableta no es un error.
func (s *BackofficeService) ArchiveWarehouse(ctx context.Context, id int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		w, err := q.LockWarehouse(ctx, id)
		if err != nil {
			return warehouseNotFound(err)
		}
		if !w.IsActive {
			return nil
		}
		items, err := q.WarehouseItemsWithStock(ctx, id)
		if err != nil {
			return err
		}
		open, err := q.WarehouseHasOpenCount(ctx, id)
		if err != nil {
			return err
		}
		pending, err := q.WarehouseHasPendingPurchases(ctx, &id)
		if err != nil {
			return err
		}
		if err := domain.CanArchiveWarehouse(w.IsSales, items, open, pending); err != nil {
			return err
		}
		if err := q.SetWarehouseActive(ctx, db.SetWarehouseActiveParams{ID: id, IsActive: false}); err != nil {
			return err
		}
		return auditWarehouse(ctx, q, domain.AuditWarehouseArchived, id, nil, map[string]any{"name": w.Name})
	})
}

// RestoreWarehouse reactiva un almacén archivado. Si mientras tanto otro activo tomó su nombre, se
// rechaza: dos almacenes con el mismo nombre en la pantalla no se distinguen.
func (s *BackofficeService) RestoreWarehouse(ctx context.Context, id int64) error {
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if err := q.LockWarehouses(ctx); err != nil {
			return err
		}
		w, err := q.LockWarehouse(ctx, id)
		if err != nil {
			return warehouseNotFound(err)
		}
		if w.IsActive {
			return nil
		}
		if err := warehouseNameFree(ctx, q, w.Name, id); err != nil {
			return err
		}
		if err := q.SetWarehouseActive(ctx, db.SetWarehouseActiveParams{ID: id, IsActive: true}); err != nil {
			return warehouseNameConflict(err)
		}
		return auditWarehouse(ctx, q, domain.AuditWarehouseRestored, id, nil, map[string]any{"name": w.Name})
	})
}

// ResolveWarehouseID convierte lo que pidió una petición en el id de un almacén activo de la
// sucursal; sin almacén, el de venta. Uno ajeno, de otra sucursal o archivado es «no existe».
func ResolveWarehouseID(ctx context.Context, q *db.Queries, sel domain.WarehouseSel) (int64, error) {
	w, err := resolveWarehouse(ctx, q, sel)
	return w.ID, err
}

// resolvedWarehouse es el almacén resuelto con su nombre, para lo que la respuesta tiene que decir.
type resolvedWarehouse struct {
	ID      int64
	Name    string
	IsSales bool
}

// resolveWarehouse es ResolveWarehouseID con el nombre.
func resolveWarehouse(ctx context.Context, q *db.Queries, sel domain.WarehouseSel) (resolvedWarehouse, error) {
	if sel.Sales() {
		w, err := q.SalesWarehouse(ctx)
		if err != nil {
			return resolvedWarehouse{}, warehouseNotFound(err)
		}
		return resolvedWarehouse{ID: w.ID, Name: w.Name, IsSales: w.IsSales}, nil
	}
	w, err := q.ResolveWarehouse(ctx, sel.ID)
	if err != nil {
		return resolvedWarehouse{}, warehouseNotFound(err)
	}
	return resolvedWarehouse{ID: w.ID, Name: w.Name, IsSales: w.IsSales}, nil
}

func warehouseNameFree(ctx context.Context, q *db.Queries, name string, self int64) error {
	existing, err := q.WarehouseByName(ctx, name)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	case err != nil:
		return err
	case existing != self:
		return domain.ErrWarehouseNameTaken
	}
	return nil
}

// warehouseNameConflict traduce el índice único del nombre: con el candado no debería pasar, pero
// un 500 por un nombre repetido sería peor que un 409.
func warehouseNameConflict(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrWarehouseNameTaken
	}
	return err
}

func warehouseNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrWarehouseNotFound
	}
	return err
}

func auditWarehouse(ctx context.Context, q *db.Queries, action domain.AuditAction, id int64, before, after any) error {
	branch, err := q.CurrentBranchID(ctx)
	if err != nil {
		return err
	}
	return auditEntity(ctx, q, action, "warehouse", id, &branch, nil, before, after)
}

// UpdateWarehouse cambia el nombre y/o hace de venta a un almacén, en una sola transacción: si lo
// segundo falla, lo primero no queda.
func (s *BackofficeService) UpdateWarehouse(ctx context.Context, id int64, name *string, makeSales bool) error {
	if name != nil {
		if err := domain.ValidWarehouseName(*name); err != nil {
			return err
		}
		clean := domain.CleanWarehouseName(*name)
		name = &clean
	}
	return s.store.WithTx(ctx, func(q *db.Queries) error {
		if name != nil {
			if err := renameWarehouseTx(ctx, q, id, *name); err != nil {
				return err
			}
		}
		if makeSales {
			return setSalesWarehouseTx(ctx, q, id)
		}
		return nil
	})
}
