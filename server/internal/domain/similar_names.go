package domain

import (
	"sort"
	"strings"
	"unicode"
)

// Nombres parecidos (spec 056): el aviso al dar de alta un insumo y los grupos de posibles duplicados.
// Va en Go y no con pg_trgm: con nombres cortos la similitud de trigramas deja fuera «Lehce entera»
// contra «Leche entera», y la regla de aquí se prueba sin base.

// NamedItem es un insumo por su nombre.
type NamedItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// DistinctPair es un par marcado «No son lo mismo», con Low < High.
type DistinctPair struct{ Low, High int64 }

// minPrefixWords: un nombre cuenta como el inicio de otro solo con dos palabras o más. Medido contra
// los insumos de una empresa real: con una palabra suelta, «Queso», «Crema» o «Bolsa» juntaban 24
// insumos distintos en un solo grupo por transitividad.
const minPrefixWords = 2

// similarKey es el nombre para comparar: sin mayúsculas, acentos, signos ni espacios de más.
func similarKey(name string) []string {
	n := NormalizeReasonName(name)
	return strings.FieldsFunc(n, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// prepared es un nombre listo para compararse muchas veces: se normaliza una sola vez por insumo.
type prepared struct {
	words  []string
	runes  []rune
	digits string
	hist   [128]int // letras y cifras ASCII; lo demás cae en la casilla 127
}

func prepare(name string) prepared {
	p := prepared{words: similarKey(name)}
	p.runes = []rune(strings.Join(p.words, " "))
	p.digits = digitsOf(string(p.runes))
	for _, r := range p.runes {
		if r < 127 {
			p.hist[r]++
		} else {
			p.hist[127]++
		}
	}
	return p
}

// similarity dice si dos nombres se parecen y qué tanto (0 es lo más parecido). Parecidos: uno es el
// inicio del otro, con dos palabras o más («Leche entera» y «Leche entera Lala 1L»; en medio no cuenta:
// «Frutos rojos» está en concentrados y tés distintos), o difieren en una letra (dos si es largo),
// contando dos letras invertidas como una. Las cifras tienen que ser las mismas: «Vaso 12 oz» y
// «Vaso 16 oz» son medidas distintas, no un dedazo.
//
// Antes de la distancia de edición van cotas que la acotan por abajo (largo y letras de más o de
// menos): con un catálogo grande casi todos los pares se descartan ahí. Medido sin ellas, 2,000
// nombres de 60 letras costaban 24 s de CPU por cada vez que se abría la hoja (revisión de seguridad).
func similarity(a, b *prepared) (int, bool) {
	if len(a.runes) == 0 || len(b.runes) == 0 {
		return 0, false
	}
	if string(a.runes) == string(b.runes) {
		return 0, true
	}
	short, long := a, b
	if len(a.runes) > len(b.runes) {
		short, long = b, a
	}
	if len(short.words) >= minPrefixWords && startsWith(long.words, short.words) {
		return 1, true
	}
	if a.digits != b.digits {
		return 0, false
	}
	limit := 1
	if max(len(a.runes), len(b.runes)) > 10 {
		limit = 2
	}
	if len(long.runes)-len(short.runes) > limit {
		return 0, false
	}
	// Cada operación agrega, quita o cambia una letra: la distancia nunca es menor que las letras que a
	// uno le sobran respecto del otro.
	plus, minus := 0, 0
	for i := range a.hist {
		if d := a.hist[i] - b.hist[i]; d > 0 {
			plus += d
		} else {
			minus -= d
		}
	}
	if max(plus, minus) > limit {
		return 0, false
	}
	d := boundedDistance(a.runes, b.runes, limit)
	return d, d <= limit
}

func startsWith(long, short []string) bool {
	if len(short) > len(long) {
		return false
	}
	for i := range short {
		if long[i] != short[i] {
			return false
		}
	}
	return true
}

func digitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// boundedDistance es la distancia de Damerau-Levenshtein restringida (dos letras invertidas cuentan
// uno), calculada solo en la franja |i-j| <= k y con salida temprana: si un renglón entero ya pasa de
// k, el resultado es k+1. Lo que importa es saber si es <= k, no cuánto vale más allá.
func boundedDistance(a, b []rune, k int) int {
	const inf = 1 << 30
	n, m := len(a), len(b)
	if n-m > k || m-n > k {
		return k + 1
	}
	prev2 := make([]int, m+1)
	prev := make([]int, m+1)
	cur := make([]int, m+1)
	for j := range prev {
		prev[j] = j
		if j > k {
			prev[j] = inf
		}
	}
	for j := range prev2 {
		prev2[j] = inf
	}
	for i := 1; i <= n; i++ {
		for j := range cur {
			cur[j] = inf
		}
		if i <= k {
			cur[0] = i
		}
		rowMin := cur[0]
		for j := max(1, i-k); j <= min(m, i+k); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				v = min(v, prev2[j-2]+1)
			}
			cur[j] = v
			rowMin = min(rowMin, v)
		}
		if rowMin > k {
			return k + 1
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return min(prev[m], k+1)
}

// SimilarNames son hasta `limit` candidatos con nombre parecido a `name`, los más parecidos primero.
// Nunca nil.
func SimilarNames(name string, candidates []NamedItem, limit int) []NamedItem {
	key := prepare(name)
	type hit struct {
		item  NamedItem
		score int
	}
	hits := []hit{}
	for _, c := range candidates {
		cp := prepare(c.Name)
		if score, ok := similarity(&key, &cp); ok {
			hits = append(hits, hit{c, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score < hits[j].score
		}
		return hits[i].item.Name < hits[j].item.Name
	})
	out := []NamedItem{}
	for _, h := range hits {
		if len(out) == limit {
			break
		}
		out = append(out, h.item)
	}
	return out
}

// DuplicateGroups junta los insumos con nombres parecidos, por transitividad (si A se parece a B y B a
// C, son un grupo), sin los pares marcados «No son lo mismo». Solo grupos de dos o más; cada uno
// ordenado por nombre y los grupos por su primer nombre. Nunca nil. stop se consulta en cada renglón:
// si dice que ya no, regresa false sin terminar (el request se canceló).
func DuplicateGroups(items []NamedItem, distinct []DistinctPair, stop func() bool) ([][]NamedItem, bool) {
	skip := make(map[DistinctPair]bool, len(distinct))
	for _, p := range distinct {
		skip[p] = true
	}
	parent := make([]int, len(items))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	keys := make([]prepared, len(items))
	for i, it := range items {
		keys[i] = prepare(it.Name)
	}
	for i := range items {
		if stop() {
			return nil, false
		}
		for j := i + 1; j < len(items); j++ {
			lo, hi := items[i].ID, items[j].ID
			if lo > hi {
				lo, hi = hi, lo
			}
			if skip[DistinctPair{lo, hi}] {
				continue
			}
			if _, ok := similarity(&keys[i], &keys[j]); ok {
				parent[find(i)] = find(j)
			}
		}
	}
	byRoot := map[int][]NamedItem{}
	for i, it := range items {
		r := find(i)
		byRoot[r] = append(byRoot[r], it)
	}
	out := [][]NamedItem{}
	for _, g := range byRoot {
		if len(g) < 2 {
			continue
		}
		sort.Slice(g, func(i, j int) bool { return g[i].Name < g[j].Name })
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0].Name < out[j][0].Name })
	return out, true
}
