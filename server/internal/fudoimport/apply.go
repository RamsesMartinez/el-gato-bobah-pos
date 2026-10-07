package fudoimport

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SQL a mano y no por sqlc, con la empresa EXPLÍCITA en cada consulta: la carga corre como owner,
// que salta RLS, y sqlc no conoce `company_id` en estas tablas (lo agregó la 0023 con DDL dinámico).
// Sin el filtro, el catálogo de otra empresa con nombres parecidos recibiría las recetas.

// LoadCatalog lee el catálogo de UNA empresa.
func LoadCatalog(ctx context.Context, q pgx.Tx, company int64) (Catalog, error) {
	cat := Catalog{Units: map[string]Unit{}}
	rows, err := q.Query(ctx, `select id, code, kind::text, to_base from units`)
	if err != nil {
		return cat, err
	}
	for rows.Next() {
		var u Unit
		var code string
		if err := rows.Scan(&u.ID, &code, &u.Kind, &u.ToBase); err != nil {
			rows.Close()
			return cat, err
		}
		cat.Units[code] = u
	}
	rows.Close()

	rows, err = q.Query(ctx, `
		select id, name::text, track_stock, type = 'combo',
		       recipe_id is not null or composition_status is not null
		  from products where company_id = $1 and system_kind is null`, company)
	if err != nil {
		return cat, err
	}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.Name, &p.TrackStock, &p.IsCombo, &p.HasComposition); err != nil {
			rows.Close()
			return cat, err
		}
		cat.Products = append(cat.Products, p)
	}
	rows.Close()

	rows, err = q.Query(ctx, `
		select id, name::text, recipe_id is not null or linked_product_id is not null or composition_status is not null
		  from modifier_options where company_id = $1`, company)
	if err != nil {
		return cat, err
	}
	for rows.Next() {
		var o Option
		if err := rows.Scan(&o.ID, &o.Name, &o.HasComposition); err != nil {
			rows.Close()
			return cat, err
		}
		cat.Options = append(cat.Options, o)
	}
	rows.Close()

	rows, err = q.Query(ctx, `
		select i.id, i.name::text, u.kind::text, i.is_prep or i.composition_status is not null
		  from ingredients i join units u on u.id = i.base_unit_id where i.company_id = $1`, company)
	if err != nil {
		return cat, err
	}
	for rows.Next() {
		var in Ingredient
		if err := rows.Scan(&in.ID, &in.Name, &in.BaseKind, &in.HasComposition); err != nil {
			rows.Close()
			return cat, err
		}
		cat.Ingredients = append(cat.Ingredients, in)
	}
	rows.Close()
	return cat, rows.Err()
}

// Applied cuenta lo que se escribió.
type Applied struct {
	Products, Options, Links, Preps int
}

// Apply escribe el plan como composición estimada. Cada update vuelve a exigir que la fila siga sin
// composición: si alguien capturó algo entre el plan y la escritura, gana lo capturado.
func Apply(ctx context.Context, tx pgx.Tx, company int64, p Plan) (Applied, error) {
	var a Applied
	recipe := func(lines []Line) (int64, error) {
		var id int64
		if err := tx.QueryRow(ctx, `insert into recipes (company_id) values ($1) returning id`, company).Scan(&id); err != nil {
			return 0, err
		}
		for i, l := range lines {
			if _, err := tx.Exec(ctx, `
				insert into recipe_items (company_id, recipe_id, ingredient_id, quantity, unit_id, position)
				values ($1, $2, $3, $4, $5, $6)`, company, id, l.IngredientID, l.Qty, l.UnitID, i); err != nil {
				return 0, err
			}
		}
		return id, nil
	}
	write := func(sql string, args ...any) (bool, error) {
		tag, err := tx.Exec(ctx, sql, args...)
		return err == nil && tag.RowsAffected() == 1, err
	}
	// drop borra la receta que no se usó porque la fila ya tenía composición.
	drop := func(rid int64) error {
		_, err := tx.Exec(ctx, `delete from recipes where id = $1 and company_id = $2`, rid, company)
		return err
	}

	for id, lines := range p.ProductRecipes {
		rid, err := recipe(lines)
		if err != nil {
			return a, fmt.Errorf("receta del producto %d: %w", id, err)
		}
		ok, err := write(`
			update products set recipe_id = $3, composition_status = 'estimated'
			 where id = $1 and company_id = $2 and recipe_id is null and composition_status is null
			   and not track_stock and type <> 'combo'`, id, company, rid)
		if err != nil {
			return a, err
		}
		if !ok {
			if err := drop(rid); err != nil {
				return a, err
			}
			continue
		}
		a.Products++
	}
	for id, lines := range p.OptionRecipes {
		rid, err := recipe(lines)
		if err != nil {
			return a, fmt.Errorf("receta del extra %d: %w", id, err)
		}
		ok, err := write(`
			update modifier_options set recipe_id = $3, composition_status = 'estimated'
			 where id = $1 and company_id = $2 and recipe_id is null and linked_product_id is null
			   and composition_status is null`, id, company, rid)
		if err != nil {
			return a, err
		}
		if !ok {
			if err := drop(rid); err != nil {
				return a, err
			}
			continue
		}
		a.Options++
	}
	for id, product := range p.OptionLinks {
		ok, err := write(`
			update modifier_options set linked_product_id = $3, composition_status = 'estimated'
			 where id = $1 and company_id = $2 and recipe_id is null and linked_product_id is null
			   and composition_status is null
			   and exists (select 1 from products where id = $3 and company_id = $2)`, id, company, product)
		if err != nil {
			return a, err
		}
		if ok {
			a.Links++
		}
	}
	for id, prep := range p.PrepIngredients {
		rid, err := recipe(prep.Lines)
		if err != nil {
			return a, fmt.Errorf("receta del insumo %d: %w", id, err)
		}
		ok, err := write(`
			update ingredients set is_prep = true, recipe_id = $3, yield_qty = $4, composition_status = 'estimated'
			 where id = $1 and company_id = $2 and not is_prep and composition_status is null`,
			id, company, rid, prep.Yield)
		if err != nil {
			return a, err
		}
		if !ok {
			if err := drop(rid); err != nil {
				return a, err
			}
			continue
		}
		a.Preps++
	}
	return a, nil
}
