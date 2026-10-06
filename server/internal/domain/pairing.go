package domain

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Emparejamiento de la tienda conectada (spec 026): en qué grupo va cada renglón, cómo se ordenan
// los candidatos al buscar, y qué precios escribe una lectura del menú.

var (
	// ErrSeveralStoresSamePlatform: la empresa tiene más de una tienda de la misma plataforma y el
	// precio por plataforma del POS es UNO por producto. Sincronizar haría que la última tienda leída
	// pisara a la otra; se rechaza hasta que el precio sea por sucursal (principio VIII).
	ErrSeveralStoresSamePlatform = errors.New("hay más de una tienda de esta plataforma: los precios no se pueden copiar todavía")
	// ErrPlatformPriceManaged: se quiso capturar a mano un precio que pone la plataforma conectada.
	// Lo rechaza el servidor y no solo la pantalla: una tableta con la pantalla vieja lo intentaría
	// igual, y el siguiente pedido se cobraría con un precio que la plataforma no publica.
	ErrPlatformPriceManaged = errors.New("ese precio lo pone la plataforma: cámbialo en la plataforma")
	// ErrCapturePriceRequired: el producto ya tiene otra pareja en la tienda y no se dijo cuál da el
	// precio de la captura a mano. Se decide al configurar para que nadie lo decida al operar.
	ErrCapturePriceRequired = errors.New("ese producto ya tiene otro platillo de la plataforma: elige cuál da el precio")
	// ErrLinkKindMismatch: un platillo se quiso ligar a una opción, o una opción a un producto.
	ErrLinkKindMismatch = errors.New("un platillo se liga a un producto y una opción a una opción")
)

// PairingGroup es el grupo en que la pantalla muestra un renglón de la tienda.
type PairingGroup string

const (
	GroupDone     PairingGroup = "done"
	GroupToReview PairingGroup = "toReview"
	GroupUnpaired PairingGroup = "unpaired"
	GroupExcluded PairingGroup = "excluded"
)

// GroupOf decide el grupo. La pareja confirmada gana a todo; la decisión «solo existe en la
// plataforma» gana a la propuesta, porque alguien ya dijo que no tiene pareja.
func GroupOf(confirmed, proposal, excluded bool) PairingGroup {
	switch {
	case confirmed:
		return GroupDone
	case excluded:
		return GroupExcluded
	case proposal:
		return GroupToReview
	default:
		return GroupUnpaired
	}
}

// GroupCounts son los conteos de la pantalla.
type GroupCounts struct {
	Unpaired int `json:"unpaired"`
	ToReview int `json:"toReview"`
	Done     int `json:"done"`
	Excluded int `json:"excluded"`
}

// CountGroups cuenta sobre los MISMOS grupos que pinta la lista: si los conteos salieran de otro
// cálculo, el encabezado y la lista podrían decir cosas distintas (principio III).
func CountGroups(groups []PairingGroup) GroupCounts {
	var c GroupCounts
	for _, g := range groups {
		switch g {
		case GroupDone:
			c.Done++
		case GroupToReview:
			c.ToReview++
		case GroupUnpaired:
			c.Unpaired++
		case GroupExcluded:
			c.Excluded++
		}
	}
	return c
}

// RankCandidates ordena los candidatos del POS por cuántas palabras comparten con el nombre del
// renglón. Solo ordena: no propone ni descarta, porque «se parece» no es «es el mismo».
func RankCandidates(name string, cands []ProductoLocal) []ProductoLocal {
	target := tokenSet(name)
	score := func(p ProductoLocal) int {
		n := 0
		for tok := range tokenSet(p.Nombre) {
			if target[tok] {
				n++
			}
		}
		return n
	}
	out := append([]ProductoLocal(nil), cands...)
	scores := make(map[int64]int, len(out))
	for _, p := range out {
		scores[p.ID] = score(p)
	}
	sort.SliceStable(out, func(i, j int) bool { return scores[out[i].ID] > scores[out[j].ID] })
	return out
}

// stopWords son conectores que harían «parecidos» a dos nombres que no lo son.
var stopWords = map[string]bool{"de": true, "con": true, "y": true, "la": true, "el": true, "en": true}

func tokenSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, tok := range strings.Fields(normalizarNombre(s)) {
		if !stopWords[tok] {
			set[tok] = true
		}
	}
	return set
}

// PriceTarget es a qué se le escribe un precio: un producto o una opción del POS.
type PriceTarget struct {
	Kind ClaseLocal
	ID   int64
}

// PairingLink es una pareja con lo que la sincronización necesita saber de ella.
type PairingLink struct {
	ExternalID   string
	Target       PriceTarget
	Confirmed    bool
	CapturePrice bool
	CreatedAt    time.Time
}

// CurrentPrice es el precio por plataforma que el POS tiene hoy para un destino.
type CurrentPrice struct {
	Price        decimal.Decimal
	FromPlatform bool
}

// PriceWrite es un precio que la lectura escribe. Changed distingue el cambio real (lo que el aviso
// cuenta) de solo pasar a «lo pone la plataforma» un precio que ya era igual.
type PriceWrite struct {
	Target     PriceTarget
	ExternalID string
	Name       string
	Old        *decimal.Decimal
	Price      decimal.Decimal
	Changed    bool
	SyncedAt   time.Time
}

// PriceSync calcula qué precios escribe una lectura buena del menú.
//
// Solo las parejas confirmadas que dan el precio de captura, solo si el platillo sigue en la lectura,
// y nunca un precio de cero o fuera de rango: la columna exige mayor que cero, y un platillo de
// regalo no dice cuánto cuesta el producto. Lo que no se escribe conserva el precio que tenía.
func PriceSync(items []ItemDePlataforma, links []PairingLink, current map[PriceTarget]CurrentPrice,
	storesOfPlatform int, now time.Time) ([]PriceWrite, error) {
	if storesOfPlatform > 1 {
		return nil, ErrSeveralStoresSamePlatform
	}
	byID := make(map[string]ItemDePlataforma, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	var writes []PriceWrite
	for _, l := range links {
		if !l.Confirmed || !l.CapturePrice {
			continue
		}
		it, ok := byID[l.ExternalID]
		if !ok || it.Centavos <= 0 {
			continue
		}
		p, err := PesosDeCentavos(it.Centavos)
		if err != nil || !ValidMoney(p, false) {
			continue
		}
		w := PriceWrite{Target: l.Target, ExternalID: l.ExternalID, Name: it.Nombre, Price: p, Changed: true, SyncedAt: now}
		if cur, ok := current[l.Target]; ok {
			old := cur.Price
			w.Old = &old
			w.Changed = !old.Equal(p)
			if !w.Changed && cur.FromPlatform {
				continue // ya lo pone la plataforma y no cambió: nada que escribir
			}
		}
		writes = append(writes, w)
	}
	sort.Slice(writes, func(i, j int) bool { return writes[i].ExternalID < writes[j].ExternalID })
	return writes, nil
}

// PlatformLineOption es una opción que la plataforma mandó dentro de un renglón del pedido.
type PlatformLineOption struct {
	Name      string
	Quantity  decimal.Decimal
	UnitPrice decimal.Decimal
}

// PlatformLineNotes escribe en una línea lo que el cliente eligió, para la nota del renglón: es lo
// que cocina lee cuando el platillo no tiene pareja en el POS y no hay otra forma de saberlo.
func PlatformLineNotes(opts []PlatformLineOption) string {
	parts := make([]string, 0, len(opts))
	for _, o := range opts {
		p := o.Name
		if o.Quantity.GreaterThan(decimal.NewFromInt(1)) {
			p = o.Quantity.String() + "× " + p
		}
		if o.UnitPrice.IsPositive() {
			p += " (+$" + o.UnitPrice.StringFixed(2) + ")"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " · ")
}

// CapturePriceAfterUnlink dice a qué pareja pasa el precio de captura cuando se quita la que lo
// daba: a la más reciente. Sin preguntar, porque quitar una pareja no debe dejar a la captura a mano
// sin precio.
func CapturePriceAfterUnlink(remaining []PairingLink) (string, bool) {
	if len(remaining) == 0 {
		return "", false
	}
	best := remaining[0]
	for _, l := range remaining[1:] {
		if l.CreatedAt.After(best.CreatedAt) || (l.CreatedAt.Equal(best.CreatedAt) && l.ExternalID > best.ExternalID) {
			best = l
		}
	}
	return best.ExternalID, true
}
