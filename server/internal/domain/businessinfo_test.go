package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestBusinessInfoValidate(t *testing.T) {
	ok := BusinessInfo{Name: "El Gato Bobah", Address: "Av. Siempre Viva 742", Phone: "55 1234 5678"}

	tests := []struct {
		name    string
		info    BusinessInfo
		wantErr bool
	}{
		{name: "completo", info: ok},
		{name: "solo el nombre", info: BusinessInfo{Name: "X"}},
		{name: "nombre vacío", info: BusinessInfo{}, wantErr: true},
		// El ticket mide 32 caracteres de ancho: un nombre largo no "se ve feo", rompe el layout
		// de todos los tickets que salgan después.
		{name: "nombre solo de espacios", info: BusinessInfo{Name: "   "}, wantErr: true},
		{name: "nombre en el tope", info: BusinessInfo{Name: strings.Repeat("a", 60)}},
		{name: "nombre pasado del tope", info: BusinessInfo{Name: strings.Repeat("a", 61)}, wantErr: true},
		{name: "dirección en el tope", info: BusinessInfo{Name: "X", Address: strings.Repeat("a", 120)}},
		{name: "dirección pasada", info: BusinessInfo{Name: "X", Address: strings.Repeat("a", 121)}, wantErr: true},
		{name: "teléfono en el tope", info: BusinessInfo{Name: "X", Phone: strings.Repeat("5", 30)}},
		{name: "teléfono pasado", info: BusinessInfo{Name: "X", Phone: strings.Repeat("5", 31)}, wantErr: true},
		// Los textos del ticket son BLOQUES de varios renglones (el aviso de "sin valor fiscal"
		// con los datos de facturación), no una línea suelta como la dirección.
		{name: "texto superior en el tope", info: BusinessInfo{Name: "X", HeaderNote: strings.Repeat("a", MaxTicketNote)}},
		{name: "texto superior pasado", info: BusinessInfo{Name: "X", HeaderNote: strings.Repeat("a", MaxTicketNote+1)}, wantErr: true},
		{name: "texto inferior en el tope", info: BusinessInfo{Name: "X", FooterNote: strings.Repeat("a", MaxTicketNote)}},
		{name: "texto inferior pasado", info: BusinessInfo{Name: "X", FooterNote: strings.Repeat("a", MaxTicketNote+1)}, wantErr: true},
		{name: "texto inferior de varios renglones", info: BusinessInfo{Name: "X", FooterNote: "=====\nTICKET SIN VALOR FISCAL\n=====\nfacturacion@elgatobobah.com"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.info.Validate()
			if tc.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("err = %v, want algo que envuelva ErrValidation (si no, sale 500)", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
		})
	}
}

// Los largos se miden en caracteres, no en bytes: "ñ" ocupa dos bytes y un nombre de 60 letras con
// acentos se rechazaría por una razón que no tiene nada que ver con cómo se ve en el papel.
func TestBusinessInfoLargoEnCaracteres(t *testing.T) {
	info := BusinessInfo{Name: strings.Repeat("ñ", 60)}
	if err := info.Validate(); err != nil {
		t.Fatalf("60 caracteres con acentos rechazados: %v", err)
	}
}
