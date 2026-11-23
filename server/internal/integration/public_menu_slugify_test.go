//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// LA BASE Y GO NORMALIZAN IGUAL (spec 097). El relleno y CreateCompany nombran con
// public_menu_slugify; la pantalla y el PATCH, con domain.SlugifyMenuName. Si divergen, la vista
// previa de Ajustes promete una dirección y la base guarda otra.
func TestMenuNameSlugifyIsTheSameInSQLAndGo(t *testing.T) {
	st := newTestStore(t)
	cases := []string{
		"El Gato Bobah", "Doña Lupe", "  Fonda   «La Güera»  ", "ÁÉÍÓÚ àèìòù âêîôû äëïöü ãõ ç Ñ",
		"Tacos #1 & Más", "--lupe--fonda--", "🍕", "", "Ñ", "İstanbul Kebab", "Ølstue", "Café_Crème",
		strings.Repeat("a", 39) + " b", strings.Repeat("a", 41), strings.Repeat("ab-", 14),
	}
	for _, in := range cases {
		var sql string
		if err := st.Pool.QueryRow(context.Background(), `select public_menu_slugify($1)`, in).Scan(&sql); err != nil {
			t.Fatal(err)
		}
		if goes := domain.SlugifyMenuName(in); goes != sql {
			t.Errorf("%q: Go %q, SQL %q", in, goes, sql)
		}
	}
}
