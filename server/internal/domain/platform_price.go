package domain

import "github.com/shopspring/decimal"

var cien = decimal.NewFromInt(100)

// PlatformPrice devuelve el precio de venta de un producto (o el delta de una opción) en una
// plataforma digital: el capturado a mano si existe, o el base más el margen de esa plataforma.
//
// Solo se guardan las EXCEPCIONES. Materializar los 502 productos × 3 plataformas serían 1,506
// filas que se vuelven obsoletas en el primer cambio de precio base, y nadie se entera; con el
// margen al vuelo, subir un precio base actualiza las tres listas solo.
//
// EL REDONDEO VA AQUÍ, en el precio UNITARIO, y no en el total de línea. `order_lines.unit_price`
// es numeric(10,2) y Postgres coacciona el valor al guardarlo, mientras el total de línea se
// calcula con el valor sin coaccionar: medido contra el catálogo real, 12 de 215 productos activos
// dan un tercer decimal al 35% (434.98 → 587.223), y el ticket que se pega a la bolsa sale con un
// centavo de diferencia contra lo que quedó en la base.
//
// `manual` en nil significa "sin excepción capturada", que es el caso de la mayoría del catálogo.
func PlatformPrice(base, markupPct decimal.Decimal, manual *decimal.Decimal) decimal.Decimal {
	if manual != nil {
		return Round2(*manual)
	}
	if markupPct.IsZero() {
		// Sin margen no se hace ninguna cuenta: el caso de todos los días (mostrador, y la
		// plataforma "Propio") no debe poder mover un centavo por un redondeo intermedio.
		return base
	}
	return Round2(base.Mul(cien.Add(markupPct)).Div(cien))
}
