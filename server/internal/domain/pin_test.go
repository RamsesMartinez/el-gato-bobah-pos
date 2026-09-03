package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidarPin(t *testing.T) {
	casos := []struct {
		nombre  string
		pin     string
		soloPin bool
		quiere  error
	}{
		// Modo por default: el nombre ya identifica y el PIN solo prueba, así que basta con 4.
		{"cuatro dígitos sin solo-PIN", "4827", false, nil},
		{"seis dígitos sin solo-PIN", "482715", false, nil},

		// Con solo-PIN el PIN ES la identidad. Con 4 dígitos y diez personas, un dedazo cae en el
		// PIN de otro 9 veces de cada diez mil; con 6, 9 de cada millón.
		{"cuatro dígitos con solo-PIN", "4827", true, ErrPinCorto},
		{"cinco dígitos con solo-PIN", "48271", true, ErrPinCorto},
		{"seis dígitos con solo-PIN", "482715", true, nil},

		// Las reglas de siempre siguen valiendo en los dos modos.
		{"todo iguales", "1111", false, ErrPinDebil},
		{"secuencia ascendente", "1234", false, ErrPinDebil},
		{"secuencia descendente", "4321", false, ErrPinDebil},
		{"secuencia larga con solo-PIN", "123456", true, ErrPinDebil},

		{"muy corto", "12", false, ErrPinCorto},
		{"vacío", "", false, ErrPinCorto},
		// Un PIN se teclea en un teclado numérico: una letra solo puede venir de un cliente que no
		// es la pantalla, y aceptarla haría imposible entrar desde la tableta.
		{"con letras", "48a7", false, ErrPinNoNumerico},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if err := ValidarPin(c.pin, c.soloPin); !errors.Is(err, c.quiere) {
				t.Fatalf("ValidarPin(%q, soloPin=%v) = %v, quiere %v", c.pin, c.soloPin, err, c.quiere)
			}
		})
	}
}

// La huella tiene que ser determinista —dos PINs iguales dan lo mismo, que es lo que deja
// detectarlos— y a la vez inútil sin el secreto.
func TestPinLookup(t *testing.T) {
	const pepper = "secreto-de-prueba"

	// Determinista: es lo que deja que el índice único de la base detecte dos PINs iguales.
	primera := PinLookup("482715", pepper)
	segunda := PinLookup("482715", pepper)
	if primera != segunda {
		t.Fatal("la huella no es determinista: el índice único no podría detectar repetidos")
	}
	if PinLookup("482715", pepper) == PinLookup("913572", pepper) {
		t.Fatal("dos PINs distintos dieron la misma huella")
	}
	// Con otro secreto la huella cambia: es lo que la vuelve inútil para quien se lleve la base
	// sin el secreto del servidor.
	if PinLookup("482715", pepper) == PinLookup("482715", "otro-secreto") {
		t.Fatal("la huella no depende del secreto: la base sería invertible sola")
	}
	// Y no contiene el PIN.
	if strings.Contains(PinLookup("482715", pepper), "482715") {
		t.Fatal("la huella lleva el PIN dentro")
	}
}
