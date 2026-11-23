package domain

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// FeaturePublicMenu es la clave estable del menú público (#333): el día que las funciones se activen
// por paquete se guarda como dato, y cambiarla dejaría sin menú a quien ya lo tenía.
const FeaturePublicMenu = "public_menu"

var (
	// ErrMenuNameUnavailable: el nombre está reservado o es (o fue) de otro negocio.
	ErrMenuNameUnavailable = errors.New("ese nombre no está disponible")
	// ErrMenuNameLimit: el negocio ya guarda MaxRetiredMenuNames nombres anteriores.
	ErrMenuNameLimit = errors.New("ya cambiaste el nombre muchas veces; puedes volver a uno anterior")
)

// MaxRetiredMenuNames topa los nombres anteriores de un negocio. Un nombre usado nunca se libera
// (un QR impreso o un enlace de WhatsApp viejo caería en el menú de otro), así que sin tope una
// empresa podría acaparar nombres globales rotándolos.
const MaxRetiredMenuNames = 20

// menuNameMaxLen y menuNameMinLen son el check de public_menu_names (0123).
const (
	menuNameMinLen = 3
	menuNameMaxLen = 40
)

// menuAccents es LA tabla de acentos. public_menu_slugify (0123) usa exactamente la misma en su
// translate(); una prueba de integración compara las dos sobre los mismos casos. Lo que no está aquí
// se vuelve guion en las dos.
const (
	menuAccentsFrom = "áàâäãéèêëíìîïóòôöõúùûüñçÁÀÂÄÃÉÈÊËÍÌÎÏÓÒÔÖÕÚÙÛÜÑÇ"
	menuAccentsTo   = "aaaaaeeeeiiiiooooouuuuncAAAAAEEEEIIIIOOOOOUUUUNC"
)

var menuAccentMap = func() map[rune]rune {
	from, to := []rune(menuAccentsFrom), []rune(menuAccentsTo)
	m := make(map[rune]rune, len(from))
	for i, r := range from {
		m[r] = to[i]
	}
	return m
}()

// SlugifyMenuName convierte lo que escribe la dueña en un nombre de dirección: acentos fuera, todo a
// minúsculas, lo que no sea letra o número a guion, sin guiones repetidos ni en las orillas, y a 40
// caracteres como máximo (sin dejar un guion en el corte). Puede quedar vacío o corto: quien llama
// decide qué hacer con eso.
func SlugifyMenuName(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range s {
		if t, ok := menuAccentMap[r]; ok {
			r = t
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > menuNameMaxLen {
		out = strings.TrimRight(out[:menuNameMaxLen], "-")
	}
	return out
}

var menuNameShape = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidMenuNameShape dice si n ya es un nombre de dirección válido (el mismo check que la tabla).
func ValidMenuNameShape(n string) bool {
	return len(n) >= menuNameMinLen && len(n) <= menuNameMaxLen && menuNameShape.MatchString(n)
}

// reservedMenuNames son rutas del propio menú y palabras que un cliente leería como del sistema. La
// misma lista vive en public_menu_free_name (0123).
var reservedMenuNames = []string{
	"q", "api", "admin", "www", "menu", "menus", "static", "assets", "login", "ayuda", "soporte",
	"app", "staff", "media", "cdn", "robots-txt", "favicon-ico", "sitemap-xml", "functions",
}

// IsReservedMenuName dice si n no se puede usar como nombre de menú.
func IsReservedMenuName(n string) bool { return slices.Contains(reservedMenuNames, n) }

// menuPalettes son los ids de PALETTES de la pantalla Interfaz (web/src/stores/ui.ts).
var menuPalettes = []string{"brand", "blue", "teal", "green", "cyan", "purple", "pink", "orange"}

// ValidPalette dice si p es una paleta que el menú sabe pintar.
func ValidPalette(p string) bool { return slices.Contains(menuPalettes, p) }

// MenuRenameAction es lo que hay que hacer en la base para cambiar el nombre.
type MenuRenameAction int

const (
	// MenuRenameNone: el pedido es el nombre vigente.
	MenuRenameNone MenuRenameAction = iota + 1
	// MenuRenameRevive: retirar el vigente y volver a uno anterior del mismo negocio.
	MenuRenameRevive
	// MenuRenameInsert: retirar el vigente y estrenar uno nuevo.
	MenuRenameInsert
)

// MenuRenameInput es lo que el servicio leyó YA con el menú del negocio bloqueado.
type MenuRenameInput struct {
	Requested      string   // ya normalizado con SlugifyMenuName
	Current        string   // el vigente
	Retired        []string // los anteriores de este negocio
	TakenElsewhere bool     // existe en otro negocio (vigente o retirado)
}

// PlanMenuRename decide cómo cambiar el nombre del menú. Un nombre que fue de otro negocio nunca se
// da (FR-003); uno propio anterior se revive aunque el tope esté lleno, porque no gasta otro nombre.
func PlanMenuRename(in MenuRenameInput) (MenuRenameAction, error) {
	if in.Requested == in.Current {
		return MenuRenameNone, nil
	}
	if !ValidMenuNameShape(in.Requested) {
		return 0, fmt.Errorf("%w: el nombre necesita de 3 a 40 letras o números", ErrValidation)
	}
	if IsReservedMenuName(in.Requested) || in.TakenElsewhere {
		return 0, ErrMenuNameUnavailable
	}
	if slices.Contains(in.Retired, in.Requested) {
		return MenuRenameRevive, nil
	}
	if len(in.Retired) >= MaxRetiredMenuNames {
		return 0, ErrMenuNameLimit
	}
	return MenuRenameInsert, nil
}
