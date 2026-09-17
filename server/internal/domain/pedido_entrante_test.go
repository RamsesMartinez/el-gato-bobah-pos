package domain

import (
	"os"
	"strings"
	"testing"
)

// EL ESPEJO DE LAS CLASES DE FALLO DE UN PEDIDO, igual que el de la 0071 y por la misma razón: si
// la lista de Go y el `check` de la migración se separan, el síntoma en producción es un 23514 al
// guardar un aviso fallido — o sea, el pedido falla Y el registro de por qué falló también.
func TestLasClasesDeFalloDePedidoCoincidenConElCheck(t *testing.T) {
	sql, err := os.ReadFile("../../migrations/0072_pedidos_de_plataforma.sql")
	if err != nil {
		t.Fatalf("leer la migración: %v", err)
	}
	texto := string(sql)
	for _, c := range ClasesDeFalloDePedido() {
		if !strings.Contains(texto, "'"+string(c)+"'") {
			t.Errorf("la clase %q existe en Go y NO en el check de la 0072", c)
		}
	}
}

// El detalle real de un pedido, recortado a lo que se usa y con datos inventados. Trae DE MÁS a
// propósito —el teléfono del cliente, la dirección, el desglose de impuestos— porque lo que este
// caso fija es que nada de eso salga del intérprete: se copia campo por campo.
const detalleDePrueba = `{
  "id": "ped-abc-123",
  "display_id": "K4T2",
  "placed_at": "2026-09-17T18:04:00Z",
  "type": "DELIVERY",
  "eater": {"first_name": "Ana", "phone": "+52...", "phone_code": "1234"},
  "delivery": {"location": {"street_address": "Calle Falsa 123"}},
  "payment": {"charges": {"total": {"amount": 34200, "currency_code": "MXN"},
                          "tax": {"amount": 4700}}},
  "cart": {"items": [
    {"id": "crepa-nutella", "title": "Crepa de Nutella", "quantity": 2,
     "price": {"unit_price": {"amount": 12900}},
     "selected_modifier_groups_items": [
       {"id": "extra-fresa", "title": "Extra fresa", "quantity": 1,
        "price": {"unit_price": {"amount": 2000}}}
     ]},
    {"id": "cafe", "title": "Café americano", "quantity": 1,
     "price": {"unit_price": {"amount": 4500}}}
  ]}
}`

func TestLeerPedidoDePlataforma(t *testing.T) {
	p, err := LeerPedidoDePlataforma([]byte(detalleDePrueba))
	if err != nil {
		t.Fatalf("interpretar el pedido: %v", err)
	}
	if p.ID != "ped-abc-123" || p.FolioCorto != "K4T2" {
		t.Fatalf("identificadores mal leídos: %+v", p)
	}
	// LOS PRECIOS VIENEN EN CENTAVOS ENTEROS. Si esto se lee como pesos, el POS registra $34,200
	// por un pedido de $342 y el corte del día queda destruido sin que nada falle.
	if got := p.Total.String(); got != "342" {
		t.Fatalf("total = %s, se esperaba 342 (34200 centavos)", got)
	}
	if len(p.Renglones) != 2 {
		t.Fatalf("renglones = %d, se esperaban 2", len(p.Renglones))
	}
	r := p.Renglones[0]
	if r.Nombre != "Crepa de Nutella" || r.Cantidad.String() != "2" || r.PrecioUnitario.String() != "129" {
		t.Fatalf("el primer renglón salió mal: %+v", r)
	}
	if len(r.Opciones) != 1 || r.Opciones[0].Nombre != "Extra fresa" || r.Opciones[0].PrecioUnitario.String() != "20" {
		t.Fatalf("las opciones no colgaron de su renglón: %+v", r.Opciones)
	}
	if p.Cliente != "Ana" {
		t.Fatalf("cliente = %q", p.Cliente)
	}
	// Nada del teléfono ni de la dirección: solo lo que hace falta para preparar y entregar.
	if strings.Contains(p.Cliente, "+52") {
		t.Fatal("se coló el teléfono del cliente")
	}
}

// UN PEDIDO PARA RECOGER, que hasta la 0072 no cabía en la tabla de pedidos. Es también el camino
// más barato para probar la integración de punta a punta: no entra repartidor.
func TestUnPedidoParaRecogerSeLeeComoParaLlevar(t *testing.T) {
	crudo := strings.Replace(detalleDePrueba, `"type": "DELIVERY"`, `"type": "PICK_UP"`, 1)
	p, err := LeerPedidoDePlataforma([]byte(crudo))
	if err != nil {
		t.Fatalf("interpretar el pedido: %v", err)
	}
	if p.TipoDeServicio != "para_llevar" {
		t.Fatalf("tipo = %q, se esperaba para_llevar", p.TipoDeServicio)
	}
}

// Los bordes que hacen que un pedido NO entre, y los que no deben impedirlo.
func TestLeerPedidoDePlataformaBordes(t *testing.T) {
	casos := []struct {
		nombre string
		crudo  string
		falla  bool
	}{
		{"no es JSON", `{`, true},
		{"sin identificador", `{"cart":{"items":[{"id":"a","title":"b","quantity":1}]}}`, true},
		// Un pedido sin renglones NO es un pedido vacío: es un detalle que no sirve. Aceptarlo
		// mandaría a la cocina un ticket en blanco.
		{"sin renglones", `{"id":"x","cart":{"items":[]}}`, true},
		{"renglones sin nombre se descartan y el pedido queda sin ninguno",
			`{"id":"x","cart":{"items":[{"id":"a","quantity":1}]}}`, true},
		// Una fecha ilegible NO tumba el pedido: perderlo costaría mucho más que no saber la hora
		// exacta, que además viene también en el aviso.
		{"fecha ilegible", `{"id":"x","placed_at":"ayer","cart":{"items":[{"id":"a","title":"A","quantity":1}]}}`, false},
		// Cantidad cero o ausente se trata como uno: la plataforma no manda pedidos de cero
		// unidades, y rechazar el pedido por eso sería peor que preparar una.
		{"cantidad ausente", `{"id":"x","cart":{"items":[{"id":"a","title":"A"}]}}`, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, err := LeerPedidoDePlataforma([]byte(c.crudo))
			if c.falla && err == nil {
				t.Fatal("se esperaba un error y entró")
			}
			if !c.falla && err != nil {
				t.Fatalf("no debía fallar: %v", err)
			}
		})
	}
}
