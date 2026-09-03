package domain

import (
	"errors"
	"strings"
	"testing"
)

// UN ESPACIO NO ES UN MOTIVO.
//
// El recorte vivía en un solo camino: la pantalla de reembolso hacía `trim` y la de cancelación solo
// miraba que la cadena no fuera vacía. Un espacio pasaba los dos lados y llegaba a la base, donde el
// `check` de la migración 0007 lo da por bueno. El histórico se quedaba con una cancelación sin
// motivo — que es exactamente lo que ese campo existe para impedir.
func TestUnMotivoEnBlancoSeRechaza(t *testing.T) {
	for _, v := range []string{"", " ", "   ", "\t", "\n", " \t\n "} {
		if _, err := MotivoValido(v); !errors.Is(err, ErrValidation) {
			t.Fatalf("motivo %q: err = %v, quiere ErrValidation", v, err)
		}
	}
}

func TestElMotivoSeGuardaRecortado(t *testing.T) {
	got, err := MotivoValido("  el cliente se arrepintió  ")
	if err != nil {
		t.Fatalf("motivo válido: %v", err)
	}
	if got != "el cliente se arrepintió" {
		t.Fatalf("motivo = %q, quiere sin espacios de sobra", got)
	}
}

// Sin cota, el único tope era el megabyte del cuerpo entero: caben ~un millón de caracteres en un
// campo que se lee en una tableta de siete pulgadas.
func TestUnMotivoDesmedidoSeRechaza(t *testing.T) {
	if _, err := MotivoValido(strings.Repeat("a", MaxMotivo+1)); !errors.Is(err, ErrValidation) {
		t.Fatalf("un motivo de %d caracteres debe rechazarse, fue %v", MaxMotivo+1, err)
	}
	// El tope exacto pasa: la cota es un límite, no un margen.
	if _, err := MotivoValido(strings.Repeat("a", MaxMotivo)); err != nil {
		t.Fatalf("el motivo del tamaño del tope debe pasar, fue %v", err)
	}
	// Y se cuenta en CARACTERES, no en bytes: con acentos, un tope en bytes rechaza un motivo que
	// en pantalla cabe de sobra.
	if _, err := MotivoValido(strings.Repeat("á", MaxMotivo)); err != nil {
		t.Fatalf("un motivo con acentos del tamaño del tope debe pasar, fue %v", err)
	}
}
