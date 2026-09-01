package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

var (
	// ErrPinCorto: el PIN no llega al largo que exige el modo del negocio.
	ErrPinCorto = fmt.Errorf("%w: el PIN es demasiado corto", ErrValidation)
	// ErrPinDebil: todo iguales o una secuencia; se adivina en tres intentos.
	ErrPinDebil = fmt.Errorf("%w: ese PIN es demasiado fácil de adivinar", ErrValidation)
	// ErrPinNoNumerico: el PIN se teclea en un teclado numérico.
	ErrPinNoNumerico = fmt.Errorf("%w: el PIN solo lleva números", ErrValidation)
)

const (
	// Con el nombre elegido aparte, el PIN solo PRUEBA quién eres: cuatro alcanzan, y el lockout
	// per-usuario es lo que frena la fuerza bruta.
	minPin = 4
	// Con solo-PIN el PIN ES la identidad. Con cuatro dígitos y diez personas, un dedazo cae en el
	// PIN de otro 9 veces de cada diez mil; con seis, 9 de cada millón. Ocho no compra nada sobre
	// seis y sí garantiza el papelito pegado a la tableta, que es peor que un PIN corto.
	minPinSolo = 6
)

// ValidarPin rechaza un PIN que no sirve para el modo en que está el negocio.
func ValidarPin(pin string, soloPin bool) error {
	minimo := minPin
	if soloPin {
		minimo = minPinSolo
	}
	if len(pin) < minimo {
		return fmt.Errorf("%w: necesita al menos %d dígitos", ErrPinCorto, minimo)
	}
	if strings.IndexFunc(pin, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return ErrPinNoNumerico
	}
	if pinDebil(pin) {
		return ErrPinDebil
	}
	return nil
}

// pinDebil: todo iguales, o una secuencia corrida hacia arriba o hacia abajo.
func pinDebil(pin string) bool {
	iguales, sube, baja := true, true, true
	for i := 1; i < len(pin); i++ {
		if pin[i] != pin[0] {
			iguales = false
		}
		if pin[i] != pin[i-1]+1 {
			sube = false
		}
		if pin[i] != pin[i-1]-1 {
			baja = false
		}
	}
	return iguales || sube || baja
}

// ErrPinRepetido: con el modo de solo-PIN, ese PIN ya lo usa otra persona.
//
// El mensaje NO dice de quién, y no es cortesía: si lo dijera, el formulario de cambiar PIN sería
// un oráculo para averiguar el de un compañero probando números hasta acertar.
var ErrPinRepetido = fmt.Errorf("%w: ese PIN ya lo usa otra persona; elige otro", ErrValidation)

// ErrPinsNoAptos: no se puede encender el modo de solo-PIN con los PINs actuales.
//
// Nombra a QUIÉNES hay que corregir. Sin la lista, el dueño tendría que revisar ficha por ficha
// para adivinar cuáles fallan.
var ErrPinsNoAptos = fmt.Errorf("%w: hay PINs que no sirven para ese modo", ErrValidation)

// ErrSinPepper: falta el secreto que hace posible comparar PINs por igualdad.
//
// Sin él no se puede garantizar que dos personas no compartan PIN, así que el modo de solo-PIN no
// se enciende. Fail-closed: nunca se activa un modo cuya única protección no se puede aplicar.
var ErrSinPepper = fmt.Errorf("%w: falta configurar el secreto de PIN en el servidor", ErrConflict)

// PinLookup es la huella determinista de un PIN: dos PINs iguales dan el mismo valor, y sin el
// secreto del servidor no se puede recorrer el espacio de seis dígitos para invertirla.
//
// NO sustituye a bcrypt: quien verifica al desbloquear sigue siendo bcrypt. Esto solo sirve para
// comparar por igualdad y para encontrar de quién es un PIN, que es lo que bcrypt no puede hacer
// porque saliniza.
func PinLookup(pin, pepper string) string {
	mac := hmac.New(sha256.New, []byte(pepper))
	mac.Write([]byte(pin))
	return hex.EncodeToString(mac.Sum(nil))
}
