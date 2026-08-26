// Command parse-doc extrae un documento de compra (ticket, factura o pedido) y lo imprime.
//
// Es la forma de verificar la extracción contra papeles reales sin levantar el API ni tocar
// la base: cuando llegue un proveedor nuevo, se corre esto con su documento y se ve si el
// schema lo cubre antes de escribir una línea de UI.
//
//	make parse-doc f=docs/tickets/ticket.pdf
//	go run ./cmd/parse-doc -json ../docs/tickets/*.pdf
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

func main() {
	raw := flag.Bool("json", false, "imprime el JSON crudo del modelo en vez del resumen")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "uso: parse-doc [-json] archivo.pdf [archivo2.jpg ...]")
		os.Exit(2)
	}

	// Solo se leen las dos variables de la extracción: esta herramienta no toca la base, así
	// que exigirle la config completa del API (DATABASE_URL, JWT_SECRET…) la volvería
	// inutilizable para lo único que hace.
	config.LoadEnvFile()
	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = config.DefaultAnthropicModel
	}
	svc := app.NewPurchaseDocService(os.Getenv("ANTHROPIC_API_KEY"), model)
	if !svc.Enabled() {
		fatal("%v — agrégala a deploy/.env", app.ErrDocExtractDisabled)
	}
	fmt.Printf("modelo: %s\n", model)

	failed := 0
	for _, path := range flag.Args() {
		if err := run(svc, path, *raw); err != nil {
			fmt.Printf("\n✗ %s\n  %v\n", filepath.Base(path), err)
			failed++
		}
	}
	if failed > 0 {
		os.Exit(1)
	}
}

func run(svc *app.PurchaseDocService, path string, rawOut bool) error {
	data, err := os.ReadFile(path) //nolint:gosec // ruta que da el operador en la línea de comandos
	if err != nil {
		return err
	}
	doc, rawJSON, err := svc.Extract(context.Background(), filepath.Base(path), data)
	if err != nil {
		return err
	}
	if rawOut {
		pretty, _ := json.MarshalIndent(rawJSON, "", "  ")
		fmt.Printf("\n=== %s ===\n%s\n", filepath.Base(path), pretty)
		return nil
	}
	report(filepath.Base(path), doc)
	return nil
}

// report imprime lo que un operador necesita para decidir si la extracción sirve: qué se
// leyó, qué no cuadra y qué renglones quedaron dudosos.
func report(name string, d domain.PurchaseDoc) {
	fmt.Printf("\n=== %s ===\n", name)
	fmt.Printf("tipo=%s  proveedor=%q  folio=%q  fecha=%q  moneda=%s\n",
		or(d.Kind, "?"), d.Supplier, d.Folio, or(d.IssuedOn, "(sin fecha)"), or(d.Currency, "?"))

	fmt.Printf("\n%-38s %8s %-6s %10s %10s  %s\n", "ARTÍCULO", "CANT", "UNID", "UNITARIO", "IMPORTE", "ESTADO")
	for _, l := range d.Lines {
		pack := ""
		if l.PackQty != "" {
			pack = fmt.Sprintf("  [%s %s]", l.PackQty, l.PackUnit)
		}
		code := ""
		if l.RawCode != "" {
			code = l.RawCode + " "
		}
		fmt.Printf("%-38s %8s %-6s %10s %10s  %s%s\n",
			trunc(code+l.RawName, 38), l.Qty, l.Unit, l.UnitPrice, l.Amount, l.Status, pack)
	}
	for _, c := range d.Charges {
		kind := "(desglose, ya incluido)"
		if c.AffectsTotal {
			kind = "(cargo)"
		}
		fmt.Printf("%-38s %8s %-6s %10s %10s  %s\n", trunc(c.Label, 38), "", "", "", c.Amount, kind)
	}

	r := d.Reconcile()
	fmt.Printf("\nlíneas=%s  cargos=%s  total=%s  diferencia=%s",
		r.LinesSum.StringFixed(2), r.ChargesSum.StringFixed(2), r.Total.StringFixed(2), r.Diff.StringFixed(2))
	if r.Balanced() {
		fmt.Print("  ✓ cuadra\n")
	} else {
		fmt.Print("  ⚠ NO cuadra — revisar\n")
	}
	if !r.BreakdownSum.IsZero() {
		fmt.Printf("desglose informativo (no suma): %s\n", r.BreakdownSum.StringFixed(2))
	}
	if r.HasSubtotal {
		fmt.Printf("subtotal impreso=%s vs líneas=%s", r.Subtotal.StringFixed(2), r.LinesSum.StringFixed(2))
		if r.LinesMatchSubtotal() {
			fmt.Print("  ✓\n")
		} else {
			fmt.Printf("  ⚠ difieren por %s — probable columna de precio equivocada\n", r.SubtotalDiff.StringFixed(2))
		}
	}

	if len(d.Payments) > 0 {
		fmt.Print("pagos: ")
		for i, p := range d.Payments {
			if i > 0 {
				fmt.Print(" + ")
			}
			fmt.Printf("%s %s", p.Method, p.Amount)
			if p.Reference != "" {
				fmt.Printf(" (%s)", p.Reference)
			}
		}
		if r.PaymentsMatchTotal() {
			fmt.Print("  ✓ cubren el total\n")
		} else {
			fmt.Printf("  ⚠ suman %s vs total %s\n", r.PaymentsSum, r.Total)
		}
	}

	for _, l := range d.Lines {
		if l.SuggestedName != "" {
			fmt.Printf("  alta sugerida: %-34s ← %s\n", l.SuggestedName, l.RawName)
		}
	}
	for _, u := range r.Unreadable {
		fmt.Printf("  ⚠ ilegible: %s\n", u)
	}
	for _, w := range d.Warnings {
		fmt.Printf("  ⚠ %s\n", w)
	}
	for _, e := range d.Extra {
		fmt.Printf("  · %s: %s\n", e.Key, e.Value)
	}
}

func or(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
