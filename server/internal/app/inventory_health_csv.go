package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// csvHeader son las columnas de la exportación (contracts/api.md). Las secciones van con su código
// estable, el mismo de la query string, para que un archivo de hoy se pueda filtrar igual mañana.
var csvHeader = []string{"seccion", "tipo", "id", "nombre", "grupo", "estado", "canal", "piezas", "costo_estimado", "existencia", "sucursal", "problema"}

// safeCell protege un texto que una hoja de cálculo leería como fórmula (=, +, -, @, y un tabulador o
// retorno de carro al inicio, que las hojas recortan al importar antes de evaluar): un nombre de
// producto como «=HYPERLINK(...)» se ejecutaría al abrir el archivo. Solo en texto; los números van
// tal cual, o «-200» dejaría de ser un número.
func safeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// CSV es la exportación de «Salud del inventario»: el mismo cálculo que la pantalla (SC-005), sin
// paginar y con la lista completa de insumos sin compras (FR-006). Devuelve el archivo con BOM —sin
// él, las hojas de cálculo comunes leen mal los acentos— y su nombre.
func (s *InventoryHealthService) CSV(ctx context.Context, f domain.HealthFilter) ([]byte, string, error) {
	if err := f.Validate(); err != nil {
		return nil, "", err
	}
	f.Section, f.Offset, f.Limit = "", 0, allRows
	v, err := s.report(ctx, f)
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	buf.WriteString("\uFEFF")
	w := csv.NewWriter(&buf)
	write := func(cells ...string) { _ = w.Write(cells) }
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	num := func(d decimal.Decimal) string { return d.String() }
	write(csvHeader...)

	for _, it := range v.UnsoldDeduction.Items {
		tipo := "producto"
		if it.Kind == "extra" {
			tipo = "extra"
		}
		for _, u := range it.Units {
			// El costo se reparte por canal con el costo unitario de hoy: sumar la columna da el total.
			// Sin costo capturado, la celda va vacía: un 0 se sumaría como «no se pierde nada».
			cost := ""
			if it.EstimatedCost != nil {
				share := decimal.Zero
				if it.Pieces.IsPositive() {
					share = domain.Round2(it.EstimatedCost.Mul(u.Units).Div(it.Pieces))
				}
				cost = num(share)
			}
			write(string(domain.SectionUnsold), tipo, id(it.ID), safeCell(it.Name), safeCell(it.Group), it.Status,
				safeCell(u.Channel), num(u.Units), cost, "", "", "")
		}
	}
	for _, c := range v.ComboChoices.Items {
		write(string(domain.SectionUnsold), "combo", id(c.ProductID), safeCell(c.Name), safeCell(c.Slot), "", "", "", "", "", "", "varias_opciones")
	}
	for _, n := range v.NegativeStock.Items {
		branch := ""
		if n.Branch != nil {
			branch = *n.Branch
		}
		// La existencia va como número solo (una hoja la suma) y la unidad en «grupo»: pegadas, «-12 kg»
		// sería texto que empieza con «-».
		write(string(domain.SectionNegative), "insumo", id(n.IngredientID), safeCell(n.Name), safeCell(n.Unit), "", "", "", "",
			num(n.OnHand), safeCell(branch), "")
	}
	for _, n := range v.NoPurchases.Items {
		write(string(domain.SectionNoPurchases), "insumo", id(n.IngredientID), safeCell(n.Name), "", "", "", "", "", "", "", "sin_compras")
	}
	for _, m := range v.MalformedRecipes.Items {
		write(string(domain.SectionMalformed), "renglon_receta", id(m.OwnerID), safeCell(m.OwnerName), safeCell(m.Ingredient), m.OwnerKind,
			"", "", "", "", "", m.Problem)
	}
	pending := v.PendingRecipes
	for _, p := range []struct {
		problem string
		n       int
	}{
		{"undecided_products", pending.Undecided.Products}, {"undecided_extras", pending.Undecided.Extras},
		{"review_products", pending.Review.Products}, {"review_extras", pending.Review.Extras},
	} {
		write(string(domain.SectionPending), "resumen", "", "", "", "", "", strconv.Itoa(p.n), "", "", "", p.problem)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "salud-del-inventario-" + v.From + "-" + v.To + ".csv", nil
}
