//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store/db"
)

// EL BLOQUEO DE PANTALLA QUEDA APAGADO EN TODAS LAS EMPRESAS, NO SOLO EN LA DEL GUC.
//
// Con DOS empresas a propósito. El `update` de la migración no lleva `where company_id`: corre como
// owner, así que RLS no aplica y alcanza a todas. Con una sola empresa en la base eso es
// indistinguible de un update que sí filtrara, y la migración pasaría verde para dejar a la segunda
// bloqueándose cada tres minutos.
func TestLaMigracionApagaElBloqueoEnTodasLasEmpresas(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	otra := makeCompany(t, st, "bloqueo-otra")
	if err := st.WithTenant(ctx, otra, func(q *db.Queries) error {
		return q.SeedBusinessSettings(ctx, "Otra")
	}); err != nil {
		t.Fatalf("sembrar ajustes: %v", err)
	}

	for _, empresa := range []int64{defaultCompanyID, otra} {
		var seg int
		if err := st.Pool.QueryRow(ctx,
			`select lock_after_seconds from business_settings where company_id = $1`, empresa).Scan(&seg); err != nil {
			t.Fatalf("empresa %d: %v", empresa, err)
		}
		if seg != 0 {
			t.Errorf("la empresa %d se bloquea a los %d s, quiere 0", empresa, seg)
		}
	}
}

// UNA EMPRESA NUEVA TAMPOCO NACE BLOQUEÁNDOSE.
//
// El default de la columna y `domain.DefaultIdentity` tienen que decir lo mismo: el seed no lista
// esa columna, así que quien decide es el DEFAULT de la base; pero un negocio sin fila de ajustes
// cae al de Go. Con los dos en desacuerdo, dos negocios idénticos se comportarían distinto según
// tengan fila o no.
func TestLaEmpresaNuevaNaceSinBloqueoYAsiLoDiceElServicio(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	nueva := makeCompany(t, st, "bloqueo-nueva")
	if err := st.WithTenant(ctx, nueva, func(q *db.Queries) error {
		return q.SeedBusinessSettings(ctx, "Nueva")
	}); err != nil {
		t.Fatalf("sembrar ajustes: %v", err)
	}

	var seg int
	if err := st.Pool.QueryRow(ctx,
		`select lock_after_seconds from business_settings where company_id = $1`, nueva).Scan(&seg); err != nil {
		t.Fatalf("leer los ajustes de la nueva: %v", err)
	}
	if seg != 0 {
		t.Errorf("la empresa nueva se bloquea a los %d s, quiere 0", seg)
	}
	leido := struct{ LockAfterSeconds int }{seg}
	if leido.LockAfterSeconds != domain.DefaultIdentity().LockAfterSeconds {
		t.Errorf("el default de la columna (%d) y el de Go (%d) no dicen lo mismo",
			leido.LockAfterSeconds, domain.DefaultIdentity().LockAfterSeconds)
	}
}
