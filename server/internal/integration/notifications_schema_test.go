//go:build integration

package integration

import (
	"context"
	"regexp"
	"sort"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// checkValues lee los valores de un check `x in ('a', 'b')` de la base.
func checkValues(t *testing.T, constraint string) []string {
	t.Helper()
	st := newTestStore(t)
	var def string
	if err := st.Pool.QueryRow(context.Background(),
		`select pg_get_constraintdef(oid) from pg_constraint where conname = $1`, constraint).Scan(&def); err != nil {
		t.Fatalf("%s: %v", constraint, err)
	}
	var out []string
	for _, m := range regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(def, -1) {
		out = append(out, m[1])
	}
	sort.Strings(out)
	return out
}

// Los motivos de cierre y los tipos de aviso viven en dos lugares (el check de la 0115 y domain): un
// motivo que solo conoce uno de los dos es un cierre que la pantalla no sabe decir, o un insert que la
// base rechaza.
func TestNotificationListsMatchTheChecks(t *testing.T) {
	var reasons, kinds []string
	for _, r := range domain.NotificationReasons() {
		reasons = append(reasons, string(r))
	}
	for _, k := range domain.NotificationKinds() {
		kinds = append(kinds, string(k))
	}
	sort.Strings(reasons)
	sort.Strings(kinds)
	if got := checkValues(t, "notifications_resolved_reason"); !equalStrings(got, reasons) {
		t.Errorf("motivos: la base dice %v y domain %v", got, reasons)
	}
	if got := checkValues(t, "notifications_kind"); !equalStrings(got, kinds) {
		t.Errorf("tipos: la base dice %v y domain %v", got, kinds)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
