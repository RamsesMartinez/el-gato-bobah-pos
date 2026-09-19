package domain

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// CentavosPorPeso es la unidad menor con la que las plataformas de reparto entregan sus precios:
// Uber manda 13000 para $130.00. No es configurable porque no cambia — es la definición del peso.
const CentavosPorPeso = 100

// PesosDeCentavos convierte el precio que entrega una plataforma —entero en unidades menores— al
// peso con dos decimales que usa el resto del sistema.
//
// ES LA ÚNICA CONVERSIÓN DE CENTAVOS A PESOS DEL SISTEMA, a propósito. Hacerla también en el
// paquete que habla con la plataforma permitiría que a cada lado le tocara un redondeo distinto, y
// el síntoma sería una comparación que reporta diferencias de un centavo que no existen.
//
// Rechaza con ErrValidation lo que no cabe en una columna de dinero en vez de devolver un número
// raro: un precio absurdo de una API ajena tiene que rebotar como 422 en la frontera, no reventar
// después contra el `numeric` (principio V).
func PesosDeCentavos(centavos int64) (decimal.Decimal, error) {
	if centavos < 0 {
		return decimal.Zero, fmt.Errorf("%w: precio negativo de la plataforma (%d centavos)", ErrValidation, centavos)
	}
	pesos := Round2(decimal.NewFromInt(centavos).Div(decimal.NewFromInt(CentavosPorPeso)))
	if !ValidMoney(pesos, true) {
		return decimal.Zero, fmt.Errorf("%w: precio fuera de rango de la plataforma (%d centavos)", ErrValidation, centavos)
	}
	return pesos, nil
}
