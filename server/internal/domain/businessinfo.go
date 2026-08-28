package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Largos del encabezado del ticket, en CARACTERES. El ancho útil del papel de 80mm son ~32
// caracteres: los topes no son estéticos, son lo que evita que un texto largo desacomode todos los
// tickets que salgan después. Los mismos números están como check en la base.
const (
	MaxBusinessName  = 60
	MaxBusinessLine  = 120 // dirección: un renglón
	MaxBusinessPhone = 30
	// Los textos del ticket son BLOQUES, no renglones: ahí va el aviso de "sin valor fiscal" con
	// los datos de facturación, que son varias líneas. 400 caracteres son ~13 renglones de 32,
	// es decir unos 5 cm de papel por ticket — más que eso deja de ser un aviso y es un folleto.
	MaxTicketNote = 400
)

// BusinessInfo es la identidad editable que sale impresa en el ticket. Todos los campos salvo Name
// son opcionales: vacío significa "no imprimas ese renglón", no "imprime un hueco".
type BusinessInfo struct {
	Name       string
	Address    string
	Phone      string
	HeaderNote string
	FooterNote string
}

// Validate rechaza la información que no cabe en un ticket de 80mm.
func (b BusinessInfo) Validate() error {
	if strings.TrimSpace(b.Name) == "" {
		return fmt.Errorf("%w: el nombre del negocio no puede ir vacío", ErrValidation)
	}
	limits := []struct {
		label string
		value string
		max   int
	}{
		{"el nombre del negocio", b.Name, MaxBusinessName},
		{"la dirección", b.Address, MaxBusinessLine},
		{"el teléfono", b.Phone, MaxBusinessPhone},
		{"el texto superior", b.HeaderNote, MaxTicketNote},
		{"el texto inferior", b.FooterNote, MaxTicketNote},
	}
	for _, l := range limits {
		// RuneCount y no len: "ñ" son dos bytes, y rechazar un nombre por sus acentos no tiene
		// nada que ver con cómo se ve en el papel.
		if utf8.RuneCountInString(l.value) > l.max {
			return fmt.Errorf("%w: %s no puede pasar de %d caracteres", ErrValidation, l.label, l.max)
		}
	}
	return nil
}
