package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/fudoimport"
)

// loadCompositions carga lo que lleva cada producto, extra e insumo desde FUDO, marcado como
// estimado, en una sola transacción. No borra ni pisa nada: solo llena lo que falta (spec 028).
func loadCompositions(ctx context.Context, pool *pgxpool.Pool, slug, csvDir string, dryRun bool) error {
	src, err := fudoimport.ReadSources(csvDir)
	if err != nil {
		return fmt.Errorf("leer los archivos de FUDO: %w", err)
	}
	var company int64
	if err := pool.QueryRow(ctx, `select id from companies where slug = $1`, slug).Scan(&company); err != nil {
		return fmt.Errorf("empresa %q: %w", slug, err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cat, err := fudoimport.LoadCatalog(ctx, tx, company)
	if err != nil {
		return fmt.Errorf("leer el catálogo: %w", err)
	}
	plan := fudoimport.PlanCompositions(src, cat)
	logReport(plan)
	if dryRun {
		log.Printf("prueba: se cargarían %d productos, %d extras con receta, %d extras ligados a producto y %d insumos compuestos",
			len(plan.ProductRecipes), len(plan.OptionRecipes), len(plan.OptionLinks), len(plan.PrepIngredients))
		return nil
	}
	applied, err := fudoimport.Apply(ctx, tx, company, plan)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Printf("cargado como estimado: %d productos, %d extras con receta, %d extras ligados a producto, %d insumos compuestos",
		applied.Products, applied.Options, applied.Links, applied.Preps)
	return nil
}

func logReport(p fudoimport.Plan) {
	r := p.Report
	log.Printf("ya capturados (no se tocan): %d", r.AlreadyCaptured)
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		log.Printf("%s (%d):\n  %s", title, len(items), strings.Join(items, "\n  "))
	}
	section("nombres que empatan con más de uno (no se adivinan)", r.Ambiguous)
	section("insumos que no existen en el POS (la receta no se cargó)", r.MissingIngredients)
	section("unidades de otro tipo (la receta no se cargó)", r.UnitMismatch)
	section("insumos que se llevarían a sí mismos, directo o por otro (no se cargaron)", r.Cycles)
	section("cantidades inválidas (la receta no se cargó)", r.InvalidQuantity)
	section("recetas de FUDO sin producto ni extra en el POS", r.NotInCatalog)
	section("paquetes de FUDO: se arman a mano en Catálogo › Recetas", r.Packages)
}
