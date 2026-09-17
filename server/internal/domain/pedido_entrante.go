package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// PedidoDePlataforma es un pedido tal como lo entrega la plataforma, ya interpretado y en el
// vocabulario del negocio. NO es un pedido del POS: se convierte en uno solo cuando alguien lo
// acepta.
type PedidoDePlataforma struct {
	ID             string
	FolioCorto     string
	Colocado       time.Time
	Cliente        string
	TipoDeServicio string
	// Total es LO QUE COBRÓ LA PLATAFORMA, y esa es la verdad. El precio del catálogo no manda
	// aquí: la dirección de la verdad en precio es de arriba hacia abajo.
	Total     decimal.Decimal
	Renglones []RenglonDePlataforma
}

// RenglonDePlataforma es un platillo pedido, con sus opciones colgando. Una opción es un renglón
// igual que su padre, porque así lo modela la plataforma: allá un modificador ES un item.
type RenglonDePlataforma struct {
	ItemID string
	Nombre string
	// Cantidad y PrecioUnitario son SNAPSHOT de lo que mandó la plataforma. Editar el catálogo no
	// puede reescribir lo que ya se cobró.
	Cantidad       decimal.Decimal
	PrecioUnitario decimal.Decimal
	Opciones       []RenglonDePlataforma
}

// ErrPedidoIlegible: el detalle llegó y no se pudo interpretar.
var ErrPedidoIlegible = errors.New("el detalle del pedido no se pudo interpretar")

// La forma que entrega la plataforma. Se declara aparte del tipo del dominio a propósito: lo que la
// plataforma agregue mañana no llega solo a nuestras tablas ni a la pantalla — hay que copiarlo
// campo por campo, que es exactamente la barrera que se quiere.
type detalleCrudo struct {
	ID        string `json:"id"`
	DisplayID string `json:"display_id"`
	PlacedAt  string `json:"placed_at"`
	Type      string `json:"type"`
	Eater     struct {
		FirstName string `json:"first_name"`
	} `json:"eater"`
	Payment struct {
		Charges struct {
			Total struct {
				Amount       int64  `json:"amount"`
				CurrencyCode string `json:"currency_code"`
			} `json:"total"`
		} `json:"charges"`
	} `json:"payment"`
	Cart struct {
		Items []itemCrudo `json:"items"`
	} `json:"cart"`
}

type itemCrudo struct {
	ID               string      `json:"id"`
	InstanceID       string      `json:"instance_id"`
	Title            string      `json:"title"`
	Quantity         int64       `json:"quantity"`
	Price            priceCrudo  `json:"price"`
	SelectedModifier []itemCrudo `json:"selected_modifier_groups_items"`
}

type priceCrudo struct {
	UnitPrice struct {
		Amount int64 `json:"amount"`
	} `json:"unit_price"`
}

// LeerPedidoDePlataforma interpreta el detalle crudo.
//
// LOS PRECIOS VIENEN EN CENTAVOS ENTEROS y aquí se convierten a pesos, redondeando en la frontera:
// es el principio III — dinero validado en cada frontera, y una sola vez. Dividir entre 100 en cada
// uso repartiría el redondeo por todo el código.
//
// EL TIPO DE SERVICIO PUEDE SER «PARA RECOGER», y hasta la 0072 eso no cabía en la tabla de
// pedidos. Es también el camino más barato para probar la integración de punta a punta, porque no
// entra repartidor.
func LeerPedidoDePlataforma(crudo []byte) (PedidoDePlataforma, error) {
	var d detalleCrudo
	if err := json.Unmarshal(crudo, &d); err != nil {
		return PedidoDePlataforma{}, fmt.Errorf("%w: %v", ErrPedidoIlegible, err)
	}
	if d.ID == "" {
		return PedidoDePlataforma{}, fmt.Errorf("%w: sin identificador de pedido", ErrPedidoIlegible)
	}

	p := PedidoDePlataforma{
		ID:             d.ID,
		FolioCorto:     d.DisplayID,
		Cliente:        d.Eater.FirstName,
		TipoDeServicio: tipoDeServicio(d.Type),
		Total:          deCentavos(d.Payment.Charges.Total.Amount),
	}
	if d.PlacedAt != "" {
		// Una fecha que no se entiende NO tumba el pedido: se deja vacía y el pedido entra igual.
		// Perder un pedido por un formato de fecha sería carísimo comparado con no saber la hora
		// exacta en que lo pidieron, que además viene también en el aviso.
		if t, err := time.Parse(time.RFC3339, d.PlacedAt); err == nil {
			p.Colocado = t
		}
	}
	p.Renglones = leerRenglones(d.Cart.Items)
	if len(p.Renglones) == 0 {
		// Un pedido sin renglones no es un pedido vacío: es un detalle que no sirve. Aceptarlo
		// mandaría a la cocina un ticket en blanco.
		return PedidoDePlataforma{}, fmt.Errorf("%w: el pedido no trae renglones", ErrPedidoIlegible)
	}
	return p, nil
}

func leerRenglones(items []itemCrudo) []RenglonDePlataforma {
	if len(items) == 0 {
		return nil
	}
	out := make([]RenglonDePlataforma, 0, len(items))
	for _, it := range items {
		id := it.ID
		if id == "" {
			id = it.InstanceID
		}
		if id == "" || it.Title == "" {
			continue
		}
		cantidad := it.Quantity
		if cantidad <= 0 {
			cantidad = 1
		}
		out = append(out, RenglonDePlataforma{
			ItemID:         id,
			Nombre:         it.Title,
			Cantidad:       decimal.NewFromInt(cantidad),
			PrecioUnitario: deCentavos(it.Price.UnitPrice.Amount),
			Opciones:       leerRenglones(it.SelectedModifier),
		})
	}
	return out
}

// tipoDeServicio traduce lo que dice la plataforma al vocabulario del POS.
//
// El default es `domicilio` porque es lo que era TODO pedido de plataforma hasta ahora, y porque un
// tipo desconocido con entrega a domicilio se atiende igual; lo que no puede pasar es que el pedido
// se rechace por una palabra nueva.
func tipoDeServicio(t string) string {
	switch t {
	case "PICK_UP", "PICKUP", "pick_up":
		return "para_llevar"
	default:
		return "domicilio"
	}
}

// deCentavos convierte el entero que manda la plataforma a pesos, con el redondeo hecho UNA vez.
func deCentavos(c int64) decimal.Decimal {
	return decimal.NewFromInt(c).Div(decimal.NewFromInt(100)).Round(2)
}
