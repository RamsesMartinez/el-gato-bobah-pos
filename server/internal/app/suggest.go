package app

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// suggest.go — defaults contextuales de modificadores.
//
// Algoritmo: popularidad contextual con decaimiento exponencial por recencia
// + kernel gaussiano por hora del día + factor por día de semana. No es un bandit
// ni collaborative-filtering (sobre-ingeniería para elegir el default de un grupo).
// La recencia exponencial captura *implícitamente* temporada/tendencia: la
// "temporada" es simplemente las semanas recientes, así que no se modela aparte.
//
// El score de cada opción es la suma de pesos de las veces que se eligió:
//
//	w = exp(-ln2 · Δdías / halfLife)         · recencia
//	  × exp(-(Δhora_circular)² / 2σ²)         · cercanía a la hora actual
//	  × (mismo día de semana ? factor : 1)    · patrón semanal
//
// El default de un grupo es el argmax de los scores. Sin umbral de soporte: el local
// es de bajo volumen, así que se emite aun con pocas compras y el POS refleja las
// elecciones recientes de inmediato en vez de quedarse mudo.

const (
	// ponytail: knobs calibrables (como calibrar un sensor). Ajustar según cómo
	// cambien los hábitos del local; no hay valores "de fábrica" universales.
	recencyHalfLifeDays = 21.0 // half-life del peso por antigüedad
	hourSigma           = 2.5  // ancho (horas) del kernel por hora del día
	sameDOWFactor       = 1.6  // multiplica el peso si fue el mismo día de semana
	maxRanked           = 4    // opciones rankeadas devueltas por grupo
)

// mxLocation: México abolió el horario de verano en 2022 → UTC-6 fijo todo el año.
// ponytail: zona fija sin depender de tzdata en el contenedor. Ceiling: si vuelve el
// DST o hay locales en otra zona, cargar time.LoadLocation por sucursal.
var mxLocation = time.FixedZone("America/Mexico_City", -6*3600)

type pick struct {
	productID int64
	groupID   int64
	optionID  int64
	at        time.Time
}

// RankedOption es una opción sugerida con su probabilidad: pct = % del peso del grupo
// que se llevó esa opción (share ponderado). El POS lo muestra como "72%".
type RankedOption struct {
	OptionID int64 `json:"id"`
	Pct      int   `json:"pct"`
}

type SuggestService struct {
	store *store.Store
	now   func() time.Time
	loc   *time.Location

	mu           sync.Mutex
	cached       map[int64]map[int64][]RankedOption
	cachedBucket time.Time // hora truncada del último cómputo
}

func NewSuggestService(s *store.Store, now func() time.Time) *SuggestService {
	if now == nil {
		now = time.Now
	}
	return &SuggestService{store: s, now: now, loc: mxLocation}
}

// Defaults devuelve producto→grupo→opciones rankeadas por probabilidad contextual.
// Memoiza por bucket de hora: recomputa a lo más una vez por hora (los hábitos no
// cambian entre requests, y la ventana de 90 días la evalúa la BD).
func (s *SuggestService) Defaults(ctx context.Context) (map[int64]map[int64][]RankedOption, error) {
	now := s.now().In(s.loc)
	bucket := now.Truncate(time.Hour)

	s.mu.Lock()
	if s.cached != nil && s.cachedBucket.Equal(bucket) {
		c := s.cached
		s.mu.Unlock()
		return c, nil
	}
	s.mu.Unlock()

	rows, err := s.store.Q.RecentModifierPicks(ctx)
	if err != nil {
		return nil, err
	}
	picks := make([]pick, len(rows))
	for i, r := range rows {
		picks[i] = pick{productID: r.ProductID, groupID: r.GroupID, optionID: r.OptionID, at: r.CreatedAt.In(s.loc)}
	}
	result := rankDefaults(picks, now)

	s.mu.Lock()
	s.cached = result
	s.cachedBucket = bucket
	s.mu.Unlock()
	return result, nil
}

// Invalidate descarta el memo para que el próximo Defaults() recompute. Se llama al
// crear un pedido: las elecciones nuevas afectan las recomendaciones de inmediato.
func (s *SuggestService) Invalidate() {
	s.mu.Lock()
	s.cached = nil
	s.cachedBucket = time.Time{}
	s.mu.Unlock()
}

// rankDefaults es la función pura del algoritmo (sin BD) → unit-testeable.
// Devuelve, por grupo, el top de opciones con su pct = share ponderado del grupo.
func rankDefaults(picks []pick, now time.Time) map[int64]map[int64][]RankedOption {
	lambda := math.Ln2 / recencyHalfLifeDays

	// scores[product][group][option] = suma de pesos
	scores := map[int64]map[int64]map[int64]float64{}
	for _, p := range picks {
		w := weight(p.at, now, lambda)
		if scores[p.productID] == nil {
			scores[p.productID] = map[int64]map[int64]float64{}
		}
		if scores[p.productID][p.groupID] == nil {
			scores[p.productID][p.groupID] = map[int64]float64{}
		}
		scores[p.productID][p.groupID][p.optionID] += w
	}

	out := map[int64]map[int64][]RankedOption{}
	for prod, groups := range scores {
		for grp, opts := range groups {
			var total float64
			for _, s := range opts {
				total += s
			}
			ranked := rankOptions(opts)
			if len(ranked) > maxRanked {
				ranked = ranked[:maxRanked]
			}
			list := make([]RankedOption, 0, len(ranked))
			for _, id := range ranked {
				pct := 0
				if total > 0 {
					pct = int(math.Round(opts[id] / total * 100))
				}
				list = append(list, RankedOption{OptionID: id, Pct: pct})
			}
			if out[prod] == nil {
				out[prod] = map[int64][]RankedOption{}
			}
			out[prod][grp] = list
		}
	}
	return out
}

// rankOptions ordena las opciones por score desc; desempata por optionID asc (determinista).
func rankOptions(opts map[int64]float64) []int64 {
	ids := make([]int64, 0, len(opts))
	for id := range opts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if opts[ids[i]] != opts[ids[j]] {
			return opts[ids[i]] > opts[ids[j]]
		}
		return ids[i] < ids[j]
	})
	return ids
}

func weight(at, now time.Time, lambda float64) float64 {
	ageDays := now.Sub(at).Hours() / 24
	if ageDays < 0 {
		ageDays = 0 // picks "del futuro" (reloj) no se penalizan ni premian de más
	}
	recency := math.Exp(-lambda * ageDays)

	// distancia circular entre horas del día (0..12): 23:00 y 01:00 distan 2h, no 22h.
	dh := math.Abs(hourOfDay(at) - hourOfDay(now))
	if dh > 12 {
		dh = 24 - dh
	}
	hourK := math.Exp(-(dh * dh) / (2 * hourSigma * hourSigma))

	dow := 1.0
	if at.Weekday() == now.Weekday() {
		dow = sameDOWFactor
	}
	return recency * hourK * dow
}

func hourOfDay(t time.Time) float64 {
	return float64(t.Hour()) + float64(t.Minute())/60
}
