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

// Alta y edición de categorías del menú (spec 099). Las reglas del árbol son de
// domain.CheckCategoryChange; aquí se leen los hechos con candado y se escribe. La llave foránea de
// parent_id es de una sola columna y salta RLS: el padre se lee bajo RLS y uno ajeno «no existe».

// CategoryRow es una categoría tal como queda después de crearla o editarla.
type CategoryRow struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ParentID *int64 `json:"parentId"`
	IsActive bool   `json:"isActive"`
}

// CategoryPatch es lo que se cambia de una categoría; nil = no se toca. SetParent dice que el padre
// sí se cambia: ParentID nil con SetParent la vuelve raíz.
type CategoryPatch struct {
	Name      *string
	Active    *bool
	ParentID  *int64
	SetParent bool
}

// CreateCategory da de alta una categoría activa, raíz o dentro de una raíz activa.
func (s *AdminService) CreateCategory(ctx context.Context, name string, parentID *int64, actor int64) (CategoryRow, error) {
	name, err := domain.NormalizeCategoryName(name)
	if err != nil {
		return CategoryRow{}, err
	}
	var out CategoryRow
	err = s.store.WithTx(ctx, func(q *db.Queries) error {
		var facts domain.CategoryFacts
		if parentID != nil {
			nodes, err := lockCategories(ctx, q, *parentID)
			if err != nil {
				return err
			}
			facts.Parent = nodes[*parentID]
		}
		if err := domain.CheckCategoryChange(facts, domain.CategoryNext{Name: name, ParentID: parentID, Active: true}); err != nil {
			return err
		}
		if err := checkSibling(ctx, q, name, parentID, 0); err != nil {
			return err
		}
		id, err := q.InsertCategory(ctx, db.InsertCategoryParams{Name: name, ParentID: parentID})
		if err != nil {
			return nameTaken(err)
		}
		out = CategoryRow{ID: id, Name: name, ParentID: parentID, IsActive: true}
		return auditEntity(ctx, q, domain.AuditCategoryCreated, "category", id, nil, &actor, nil,
			map[string]any{"name": name, "parentId": parentID, "isActive": true})
	})
	return out, err
}

// UpdateCategory cambia nombre, estado y padre de una categoría. Un cambio que no cambia nada no
// escribe ni deja bitácora.
func (s *AdminService) UpdateCategory(ctx context.Context, id int64, p CategoryPatch, actor int64) (CategoryRow, error) {
	var out CategoryRow
	err := s.store.WithTx(ctx, func(q *db.Queries) error {
		lockIDs := []int64{id}
		if p.SetParent && p.ParentID != nil {
			lockIDs = append(lockIDs, *p.ParentID)
		}
		nodes, err := lockCategories(ctx, q, lockIDs...)
		if err != nil {
			return err
		}
		cur := nodes[id]
		if cur == nil {
			return fmt.Errorf("%w: no existe la categoría %d", domain.ErrNotFound, id)
		}
		next := domain.CategoryNext{Name: cur.Name, ParentID: cur.ParentID, Active: cur.Active}
		if p.Name != nil {
			if next.Name, err = domain.NormalizeCategoryName(*p.Name); err != nil {
				return err
			}
		}
		if p.Active != nil {
			next.Active = *p.Active
		}
		if p.SetParent {
			next.ParentID = p.ParentID
		}
		out = CategoryRow{ID: id, Name: next.Name, ParentID: next.ParentID, IsActive: next.Active}
		before, after := categoryDiff(cur, next)
		if len(after) == 0 {
			// Nada cambia: ni se valida (un árbol heredado inválido no hace fallar un no-cambio) ni se
			// escribe ni deja bitácora.
			return nil
		}
		facts := domain.CategoryFacts{Target: cur}
		if next.ParentID != nil {
			if facts.Parent = nodes[*next.ParentID]; facts.Parent == nil && !p.SetParent {
				// El padre de siempre, que no se pidió cambiar: se bloquea después de la categoría.
				// No hay ciclo de candados: quien mueve bloquea los dos en orden de id, y aquí el
				// padre no cambia.
				more, err := lockCategories(ctx, q, *next.ParentID)
				if err != nil {
					return err
				}
				facts.Parent = more[*next.ParentID]
			}
		}
		children, err := q.CategoryChildren(ctx, id)
		if err != nil {
			return err
		}
		facts.HasChildren = len(children) > 0
		for _, c := range children {
			if c.IsActive {
				facts.ActiveChildren = append(facts.ActiveChildren, c.Name)
			}
		}
		if cur.Active && !next.Active {
			n, err := q.CategoryActiveProductCount(ctx, id)
			if err != nil {
				return err
			}
			facts.ActiveProducts = int(n)
		}
		if err := domain.CheckCategoryChange(facts, next); err != nil {
			return err
		}
		if _, renamed := after["name"]; renamed || !sameID(cur.ParentID, next.ParentID) {
			if err := checkSibling(ctx, q, next.Name, next.ParentID, id); err != nil {
				return err
			}
		}
		if err := q.UpdateCategoryTree(ctx, db.UpdateCategoryTreeParams{ID: id, Name: next.Name, ParentID: next.ParentID,
			IsActive: next.Active}); err != nil {
			return nameTaken(err)
		}
		return auditEntity(ctx, q, domain.AuditCategoryUpdated, "category", id, nil, &actor, before, after)
	})
	return out, err
}

// lockCategories bloquea las categorías en una sola sentencia y en orden de id (research R7). Las
// que no vuelven no existen en la empresa de la sesión.
func lockCategories(ctx context.Context, q *db.Queries, ids ...int64) (map[int64]*domain.CategoryNode, error) {
	rows, err := q.LockCategoriesForChange(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]*domain.CategoryNode, len(rows))
	for _, r := range rows {
		out[r.ID] = &domain.CategoryNode{ID: r.ID, Name: r.Name, ParentID: r.ParentID, Active: r.IsActive}
	}
	return out, nil
}

// checkSibling dice cuál categoría ya tiene ese nombre bajo el mismo padre, para que el agente use
// esa en vez de crear otra.
func checkSibling(ctx context.Context, q *db.Queries, name string, parentID *int64, self int64) error {
	sib, err := q.FindCategorySibling(ctx, db.FindCategorySiblingParams{ParentID: parentID, Name: name, SelfID: self})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	state := ""
	if !sib.IsActive {
		state = ", inactiva"
	}
	return fmt.Errorf("%w: «%s» (id %d%s)", domain.ErrCategoryNameTaken, name, sib.ID, state)
}

// nameTaken traduce la carrera entre dos altas con el mismo nombre. Solo el único de nombres: otro
// 23505 no es «nombre repetido». La transacción ya está abortada, así que se devuelve tal cual, sin
// escribir nada más en ella.
func nameTaken(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == "categories_name_scope" {
		return fmt.Errorf("%w: otra categoría con ese nombre se acaba de guardar", domain.ErrCategoryNameTaken)
	}
	return err
}

// categoryDiff devuelve solo los campos que cambian, antes y después, para la bitácora. El nombre
// se compara exacto: corregir mayúsculas es un cambio aunque citext los iguale.
func categoryDiff(cur *domain.CategoryNode, next domain.CategoryNext) (before, after map[string]any) {
	before, after = map[string]any{}, map[string]any{}
	if cur.Name != next.Name {
		before["name"], after["name"] = cur.Name, next.Name
	}
	if !sameID(cur.ParentID, next.ParentID) {
		before["parentId"], after["parentId"] = cur.ParentID, next.ParentID
	}
	if cur.Active != next.Active {
		before["isActive"], after["isActive"] = cur.Active, next.Active
	}
	return before, after
}

func sameID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
