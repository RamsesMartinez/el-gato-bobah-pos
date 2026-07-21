package domain

// Currency es un código ISO-4217. El sistema es currency-aware desde ya (cada
// transacción lleva su moneda), aunque por ahora solo se opera en una a la vez.
type Currency string

const (
	MXN Currency = "MXN"
	USD Currency = "USD"

	// DefaultCurrency es la moneda del local si no se especifica otra.
	DefaultCurrency = MXN
)

// Valid reporta si la moneda está soportada. Amplía este set (y Decimals si el nuevo
// exponente no es 2) al habilitar más monedas.
func (c Currency) Valid() bool {
	return c == MXN || c == USD
}

// Decimals son los dígitos de la unidad menor (centavos). MXN y USD = 2.
func (c Currency) Decimals() int32 { return 2 }
