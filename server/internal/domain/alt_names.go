package domain

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Nombres alternativos (spec 063): otros nombres con los que se busca un producto o un artículo del
// almacén. Solo sirven para encontrar; lo que se imprime y se reporta lee el nombre principal.

const (
	// MaxAltNames es cuántos alternativos lleva un producto o artículo (decisión del dueño).
	MaxAltNames = 10
	// MaxAltName es el largo de uno, el del nombre de un artículo: cabe en el renglón de cualquier
	// buscador.
	MaxAltName = 60
)

var catalogAccents = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")

// CatalogNameKey es la forma con que dos nombres del catálogo son el mismo: sin mayúsculas, acentos
// ni espacios de más. Gemela de `catalog_name_key` en la base (0107); si cambia una, cambia la otra.
// strings.Fields y no `\s` de regexp: el de Go no ve el espacio duro.
func CatalogNameKey(s string) string {
	return strings.Join(strings.Fields(catalogAccents.Replace(strings.ToLower(s))), " ")
}

// CleanAltNames deja la lista como se guarda: cada nombre sin espacios de más, sin los vacíos y en
// el orden capturado. Rechaza lo que no cabe, un repetido y uno igual al nombre propio. Nunca
// devuelve nil: un nil viaja a Postgres como NULL y sale en JSON como `null`.
func CleanAltNames(own string, in []string) ([]string, error) {
	out := []string{}
	ownKey := CatalogNameKey(own)
	seen := map[string]bool{}
	for _, raw := range in {
		name := strings.Join(strings.Fields(raw), " ")
		if name == "" {
			continue
		}
		if !PrintableName(name) {
			return nil, fmt.Errorf("%w: «%s» lleva caracteres que no se ven", ErrValidation, name)
		}
		if utf8.RuneCountInString(name) > MaxAltName {
			return nil, fmt.Errorf("%w: cada nombre lleva hasta %d letras", ErrValidation, MaxAltName)
		}
		key := CatalogNameKey(name)
		if key == ownKey {
			return nil, fmt.Errorf("%w: «%s» ya es el nombre de este", ErrValidation, name)
		}
		if seen[key] {
			return nil, fmt.Errorf("%w: «%s» está repetido", ErrValidation, name)
		}
		seen[key] = true
		out = append(out, name)
	}
	if len(out) > MaxAltNames {
		return nil, fmt.Errorf("%w: hasta %d nombres", ErrValidation, MaxAltNames)
	}
	return out, nil
}

// PrintableName dice que un nombre solo trae letras que se ven: ni UTF-8 inválido ni NUL (Postgres
// los rechaza con 22021 y salía un 500), ni controles, ni caracteres de formato como el espacio de
// ancho cero, que dejarían guardar «Café​frío» junto a «Café frío» viéndose iguales (revisión).
func PrintableName(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

// ErrAltNameTaken: el nombre ya es de otro producto o artículo de la empresa.
var ErrAltNameTaken = fmt.Errorf("%w: ese nombre ya lo usa otro", ErrConflict)

// AltNameTaken dice qué nombre chocó y con quién. Lleva el dueño para que la pantalla lo nombre; RLS
// garantiza que nunca es de otra empresa.
type AltNameTaken struct {
	Name          string
	OwnerID       int64
	OwnerName     string
	OwnerArchived bool
	// OwnerAlt: chocó con un alternativo del dueño, no con su nombre.
	OwnerAlt bool
	// Article: el dueño es un artículo; si no, un producto. Cambia cómo se dice «archivado».
	Article bool
}

func (e AltNameTaken) Error() string {
	what := "el nombre"
	if e.OwnerAlt {
		what = "otro nombre"
	}
	msg := fmt.Sprintf("«%s» ya es %s de «%s»", e.Name, what, e.OwnerName)
	if e.OwnerArchived {
		if e.Article {
			msg += " (archivado)"
		} else {
			msg += " (inactivo)"
		}
	}
	return msg
}

func (e AltNameTaken) Unwrap() error { return ErrAltNameTaken }

// AltCandidate es un nombre de otro producto o artículo contra el que se busca un parecido: el
// principal (Alt vacío) o uno de sus alternativos.
type AltCandidate struct {
	ID   int64
	Name string
	Alt  string
}

// SimilarAlt es un parecido para el aviso: con quién y, si fue por su alternativo, cuál.
type SimilarAlt struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	ViaAlt string `json:"viaAlt,omitempty"`
}

// SimilarAltNames son hasta `limit` productos o artículos con un nombre parecido a `name`, sin el
// propio (`self`) ni los idénticos (ése es el choque, no un aviso). Uno por producto, por su nombre
// más parecido. «Parecido» es la regla de SimilarNames (spec 056). Nunca nil.
func SimilarAltNames(name string, self int64, cands []AltCandidate, limit int) []SimilarAlt {
	key := CatalogNameKey(name)
	named := make([]NamedItem, 0, len(cands))
	for i, c := range cands {
		text := c.Name
		if c.Alt != "" {
			text = c.Alt
		}
		if c.ID == self || CatalogNameKey(text) == key {
			continue
		}
		named = append(named, NamedItem{ID: int64(i), Name: text})
	}
	hits := SimilarNames(name, named, len(named))
	out := []SimilarAlt{}
	seen := map[int64]bool{}
	for _, h := range hits {
		c := cands[h.ID]
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		out = append(out, SimilarAlt{ID: c.ID, Name: c.Name, ViaAlt: c.Alt})
		if len(out) == limit {
			break
		}
	}
	return out
}
