package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

// MaxArticleName es el largo de un artículo del almacén, en letras: cabe en el renglón de la lista.
const MaxArticleName = 60

// CleanArticleName es el nombre como se guarda: sin espacios en las orillas ni repetidos adentro.
func CleanArticleName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

// ArticleInput es lo que se captura de un artículo al crearlo o editarlo. Los punteros nulos son
// campos que no se tocan.
type ArticleInput struct {
	Name         string
	MinStock     *decimal.Decimal
	Cost         *decimal.Decimal
	InitialStock *decimal.Decimal
}

// ValidateArticle rechaza como 400 lo que no cabe en la ficha. El nombre repetido no se decide aquí:
// depende de los demás artículos de la empresa, y lo responde la base con su candado.
func ValidateArticle(in ArticleInput) error {
	if n := utf8.RuneCountInString(CleanArticleName(in.Name)); n == 0 || n > MaxArticleName {
		return fmt.Errorf("%w: el nombre lleva de 1 a %d letras", ErrValidation, MaxArticleName)
	}
	if in.MinStock != nil && !ValidStockQtyOrZero(*in.MinStock) {
		return fmt.Errorf("%w: el mínimo no puede ser negativo", ErrValidation)
	}
	if in.InitialStock != nil && !ValidStockQtyOrZero(*in.InitialStock) {
		return fmt.Errorf("%w: la existencia inicial no puede ser negativa", ErrValidation)
	}
	if in.Cost != nil && !ValidMoney(*in.Cost, true) {
		return fmt.Errorf("%w: costo inválido", ErrValidation)
	}
	return nil
}

// UnitPath es cómo se cambia la unidad base de un artículo.
type UnitPath string

const (
	// UnitPathDirect convierte el mismo artículo: solo tiene ajustes.
	UnitPathDirect UnitPath = "direct"
	// UnitPathReplace crea otro en la unidad nueva y archiva este: ya tiene compras, ventas o mermas.
	UnitPathReplace UnitPath = "replace"
)

// UnitChangePath decide el camino por los tipos de movimiento del artículo. Solo un ajuste (conteo,
// existencia inicial, cambio de modo) deja convertir en el mismo artículo: los demás registran algo
// que pasó con alguien —un proveedor, un cliente— en la unidad vieja, y no se reescriben (decisión
// del dueño, 2026-10-19). Un tipo que no se conoce cuenta como de los que bloquean.
func UnitChangePath(movementTypes []string) UnitPath {
	for _, t := range movementTypes {
		if t != "ajuste" {
			return UnitPathReplace
		}
	}
	return UnitPathDirect
}

// ValidUnitChange acepta un cambio de clase de unidad con una equivalencia (cuánto de la unidad base
// nueva vale una de la vieja). Entre masa y volumen no se ofrece: haría falta la densidad, y 1 g = 1 ml
// es cierto para el agua y falso para el aceite.
func ValidUnitChange(fromKind, toKind string, factor decimal.Decimal) error {
	if fromKind == toKind {
		return fmt.Errorf("%w: el artículo ya se mide así", ErrValidation)
	}
	if fromKind != "pieza" && toKind != "pieza" {
		return fmt.Errorf("%w: no se puede pasar de %s a %s sin la densidad del producto", ErrValidation, fromKind, toKind)
	}
	if !ValidQty(factor, MaxStockQty, false) {
		return fmt.Errorf("%w: la equivalencia debe ser mayor que cero", ErrValidation)
	}
	return nil
}

// ConvertQty pasa una cantidad a la unidad nueva, a 4 decimales como la columna.
func ConvertQty(qty, factor decimal.Decimal) decimal.Decimal {
	return Round4(qty.Mul(factor))
}

// ConvertCost pasa un costo por unidad a la unidad nueva: si una pieza son 5,000 g, el costo de la
// pieza es 5,000 veces el del gramo. 6 decimales, la escala de los costos por unidad.
func ConvertCost(cost, factor decimal.Decimal) decimal.Decimal {
	return cost.Div(factor).Round(6)
}

// ErrArticleNameTaken: ya hay un artículo con ese nombre, sin distinguir mayúsculas, acentos ni
// espacios. Es un conflicto (409) con código propio para que la pantalla ofrezca el que existe.
var ErrArticleNameTaken = fmt.Errorf("%w: ya hay un artículo que se llama así", ErrConflict)

// ArticleNameTaken lleva el artículo que ya existe y si está archivado: la pantalla ofrece abrirlo o
// reactivarlo en vez de crear otro.
type ArticleNameTaken struct {
	ID       int64
	Archived bool
}

func (e ArticleNameTaken) Error() string { return ErrArticleNameTaken.Error() }
func (e ArticleNameTaken) Unwrap() error { return ErrArticleNameTaken }

// ErrArticleStale: la ficha cambió mientras se veía. Es un conflicto (409 STALE): la pantalla vuelve a
// cargar en vez de pisar lo que guardó otra persona.
var ErrArticleStale = fmt.Errorf("%w: el artículo cambió mientras lo veías; vuelve a abrirlo", ErrConflict)

// ErrPrepCost: el costo de un preparado lo calcula su receta, no se captura a mano (409 PREP_COST).
var ErrPrepCost = fmt.Errorf("%w: el costo de un preparado sale de su receta", ErrConflict)

// TrendPct es cuánto cambió el costo contra una referencia, en por ciento con un decimal. Sin
// referencia positiva no hay tendencia.
func TrendPct(current, reference decimal.Decimal) *decimal.Decimal {
	if !reference.IsPositive() {
		return nil
	}
	pct := current.Sub(reference).Div(reference).Mul(decimal.NewFromInt(100)).Round(1)
	return &pct
}

// ErrArticleInUse: el artículo no se archiva mientras una receta activa lo reste o un conteo abierto
// lo tenga capturado (409 IN_USE): la pantalla dice cuáles.
var ErrArticleInUse = fmt.Errorf("%w: el artículo todavía se usa", ErrConflict)
