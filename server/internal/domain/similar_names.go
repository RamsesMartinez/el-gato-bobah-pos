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

// similarity dice si dos nombres se parecen y qué tanto (0 es lo más parecido). Parecidos: uno es el
// inicio del otro, con dos palabras o más («Leche entera» y «Leche entera Lala 1L»; en medio no cuenta:
// «Frutos rojos» está en concentrados y tés distintos), o difieren en una letra (dos si es largo),
// contando dos letras invertidas como una. Las cifras tienen que ser las mismas: «Vaso 12 oz» y
// «Vaso 16 oz» son medidas distintas, no un dedazo.
func similarity(a, b []string) (int, bool) {
	sa, sb := strings.Join(a, " "), strings.Join(b, " ")
	if sa == "" || sb == "" {
		return 0, false
	}
	if sa == sb {
		return 0, true
	}
	short, long := a, b
	if len([]rune(sa)) > len([]rune(sb)) {
		short, long = b, a
	}
	if len(short) >= minPrefixWords && startsWith(long, short) {
		return 1, true
	}
	if digitsOf(sa) != digitsOf(sb) {
		return 0, false
	}
	limit := 1
	if max(len([]rune(sa)), len([]rune(sb))) > 10 {
		limit = 2
	}
	d := editDistance([]rune(sa), []rune(sb))
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

// editDistance es la distancia de Damerau-Levenshtein restringida: dos letras invertidas cuentan uno.
func editDistance(a, b []rune) int {
	d := make([][]int, len(a)+1)
	for i := range d {
		d[i] = make([]int, len(b)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(a)][len(b)]
}

// SimilarNames son hasta `limit` candidatos con nombre parecido a `name`, los más parecidos primero.
// Nunca nil.
func SimilarNames(name string, candidates []NamedItem, limit int) []NamedItem {
	key := similarKey(name)
	type hit struct {
		item  NamedItem
		score int
	}
	hits := []hit{}
	for _, c := range candidates {
		if score, ok := similarity(key, similarKey(c.Name)); ok {
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
// ordenado por nombre y los grupos por su primer nombre. Nunca nil.
func DuplicateGroups(items []NamedItem, distinct []DistinctPair) [][]NamedItem {
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
	keys := make([][]string, len(items))
	for i, it := range items {
		keys[i] = similarKey(it.Name)
	}
	for i := range items {
		for j := i + 1; j < len(items); j++ {
			lo, hi := items[i].ID, items[j].ID
			if lo > hi {
				lo, hi = hi, lo
			}
			if skip[DistinctPair{lo, hi}] {
				continue
			}
			if _, ok := similarity(keys[i], keys[j]); ok {
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
	return out
}
