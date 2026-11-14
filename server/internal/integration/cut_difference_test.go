//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/store"
)

// LA MISMA DIFERENCIA EN LAS TRES PANTALLAS (#290).
//
// En noviembre de 2026 la Caja B cerró con el cajón corto: el Histórico decía faltante, el diálogo
// «Caja cerrada» decía «Efectivo: dif. $0» y quien cerraba no supo que algo faltaba. Las tres salen
// ahora de domain.CutDifference; este test las compara entre sí, no contra una cifra escrita a mano,
// porque el defecto era que discrepaban.
func TestTheThreeScreensShowTheSameDifference(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	settings := app.NewSettingsService(st, "pepper-de-prueba")
	cajero := makeUser(t, st, "cajero_tres_pantallas", "cajero")
	principal, _, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)

	// $785 contra $835: faltan $50 en el cajón.
	cerrada, err := backoffice.CloseSession(ctx, principal, cajero, app.CierreCmd{
		Piezas: piezasDe(t, st, "500", 1, "100", 2, "20", 4, "5", 1),
	})
	if err != nil {
		t.Fatalf("cerrar con faltante: %v", err)
	}
	quiero := domain.CutDiff{Drawer: dp("-50"), OtherMethods: decimal.Zero, Total: decimal.RequireFromString("-50"), DrawerCovered: true}

	mismo := func(donde string, got *domain.CutDiff) {
		t.Helper()
		if got == nil {
			t.Fatalf("%s no trae la diferencia del corte", donde)
		}
		if got.Drawer == nil || !got.Drawer.Equal(*quiero.Drawer) || !got.OtherMethods.Equal(quiero.OtherMethods) ||
			!got.Total.Equal(quiero.Total) || got.DrawerCovered != quiero.DrawerCovered {
			t.Fatalf("%s dice cajón %v, otros %s, total %s (cubierto %v); el corte tiene un faltante de $50 "+
				"solo en el cajón — las tres pantallas tienen que decir lo mismo",
				donde, got.Drawer, got.OtherMethods, got.Total, got.DrawerCovered)
		}
	}
	mismo("el diálogo «Caja cerrada»", cerrada.Difference)

	detalle, err := backoffice.SessionDetail(ctx, cerrada.ID)
	if err != nil {
		t.Fatalf("SessionDetail: %v", err)
	}
	mismo("el detalle del corte", detalle.Difference)

	fila := filaDelHistorico(t, ctx, backoffice, cerrada.ID, true)
	mismo("la lista del Histórico", fila.Difference)
	if fila.TotalDifference == nil || !fila.TotalDifference.Equal(fila.Difference.Total) {
		t.Fatalf("totalDifference (%v) dejó de ser igual a difference.total (%s): los clientes que aún lo leen verían otra cifra",
			fila.TotalDifference, fila.Difference.Total)
	}

	// CON ARQUEO CIEGO, quien no configura la caja no ve la diferencia del último cierre en
	// ninguna de las dos lecturas que puede pedir: lo contado es el esperado de la apertura
	// siguiente (spec 035, R8).
	prenderCiego(t, ctx, settings)
	oculta := filaDelHistorico(t, ctx, backoffice, cerrada.ID, false)
	if oculta.Difference != nil || oculta.TotalDifference != nil {
		t.Fatalf("con arqueo ciego la lista mandó la diferencia del último cierre: %+v", oculta.Difference)
	}
	ciego, err := backoffice.SessionDetailFor(ctx, cerrada.ID, false)
	if err != nil {
		t.Fatalf("SessionDetailFor ciego: %v", err)
	}
	if ciego.Difference != nil {
		t.Fatalf("con arqueo ciego el detalle mandó la diferencia del último cierre: %+v", ciego.Difference)
	}
}

// Un turno abierto no tiene diferencia: nada se ha contado todavía. Un «Cuadra» ahí afirmaría algo
// que nadie verificó.
func TestAnOpenShiftHasNoDifference(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	backoffice := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_turno_abierto_dif", "cajero")
	principal, _, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)
	abierta, err := backoffice.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatalf("CurrentByRegister: %v", err)
	}
	if abierta.Difference != nil {
		t.Fatalf("el turno abierto trae diferencia %+v sin haber contado", abierta.Difference)
	}
	if fila := filaDelHistorico(t, ctx, backoffice, abierta.ID, true); fila.Difference != nil {
		t.Fatalf("la fila del turno abierto trae diferencia %+v", fila.Difference)
	}
}

func filaDelHistorico(t *testing.T, ctx context.Context, b *app.BackofficeService, id int64, canSee bool) app.SessionHistoryRow {
	t.Helper()
	filas, _, err := b.SessionHistoryFor(ctx, domain.SessionHistoryFilter{Limit: 50}, canSee)
	if err != nil {
		t.Fatalf("SessionHistoryFor: %v", err)
	}
	for _, f := range filas {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("el corte %d no está en el Histórico", id)
	return app.SessionHistoryRow{}
}

func dp(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

// EL MOTIVO DE LA APERTURA EXPLICA EL CORTE ANTERIOR DE SU CAJA (#290).
//
// El cajero de la Caja B abrió el turno siguiente con «el cierre se contó mal» y explicó qué pasó, y
// esa nota no aparecía en ninguna pantalla. Ahora viaja en el turno que la escribió y como
// `explainedByNext` en el corte que explica — el de SU caja, nunca el de otra.
func TestTheOpeningReasonExplainsThePreviousCut(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_motivo_enlaza", "cajero")
	principal := registerID(t, st, "Caja principal")
	fuerte := registerID(t, st, "Caja fuerte")
	quinientos, cuatrocientos := dec("500"), dec("400")

	// Caja principal: un corte sin nada que explicar.
	if _, err := back.OpenSession(ctx, principal, aperturaAMano(quinientos), cajero); err != nil {
		t.Fatal(err)
	}
	cortePrincipal, err := back.CloseSession(ctx, principal, cajero, app.CierreCmd{Total: &quinientos, Motivo: "prueba"})
	if err != nil {
		t.Fatal(err)
	}
	// Caja fuerte: abre con 500 y cierra contando 400 (faltan 100); la apertura siguiente cuenta 500
	// y explica que el cierre se contó mal.
	if _, err := back.OpenSession(ctx, fuerte, aperturaAMano(quinientos), cajero); err != nil {
		t.Fatal(err)
	}
	corteFuerte, err := back.CloseSession(ctx, fuerte, cajero, app.CierreCmd{Total: &cuatrocientos, Motivo: "prueba"})
	if err != nil {
		t.Fatal(err)
	}
	const nota = "El gasto salió de esta caja y se contó como si fuera de la otra"
	siguiente, err := back.OpenSession(ctx, fuerte, app.AperturaCmd{Total: &quinientos, Motivo: "prueba",
		Reason: "last_count_wrong", ReasonNote: nota}, cajero)
	if err != nil {
		t.Fatal(err)
	}

	nuevo := filaDelHistorico(t, ctx, back, siguiente.ID, true)
	if nuevo.OpeningReason == nil || *nuevo.OpeningReason != "last_count_wrong" || nuevo.OpeningReasonNote == nil || *nuevo.OpeningReasonNote != nota {
		t.Fatalf("el turno que abrió con motivo no lo trae: %v / %v", nuevo.OpeningReason, nuevo.OpeningReasonNote)
	}
	explicado := filaDelHistorico(t, ctx, back, corteFuerte.ID, true)
	if explicado.ExplainedByNext == nil || explicado.ExplainedByNext.SessionID != siguiente.ID || explicado.ExplainedByNext.ReasonNote == nil || *explicado.ExplainedByNext.ReasonNote != nota {
		t.Fatalf("el corte de la Caja fuerte no dice qué turno lo explicó: %+v", explicado.ExplainedByNext)
	}
	if otro := filaDelHistorico(t, ctx, back, cortePrincipal.ID, true); otro.ExplainedByNext != nil {
		t.Fatalf("el corte de la Caja principal tomó la explicación de otra caja: %+v", otro.ExplainedByNext)
	}

	// EL BORDE DE LA PÁGINA: el corte explicado cae en una página y su turno siguiente en otra. Una
	// ventana sobre la página lo perdería; la relación se busca fuera del filtro.
	for off := int32(0); off < 3; off++ {
		filas, _, err := back.SessionHistoryFor(ctx, domain.SessionHistoryFilter{Limit: 1, Offset: off}, true)
		if err != nil {
			t.Fatal(err)
		}
		if len(filas) == 1 && filas[0].ID == corteFuerte.ID && filas[0].ExplainedByNext == nil {
			t.Fatalf("en una página de un renglón el corte %d perdió su explicación", corteFuerte.ID)
		}
	}

	detalle, err := back.SessionDetail(ctx, corteFuerte.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detalle.ExplainedByNext == nil || detalle.ExplainedByNext.SessionID != siguiente.ID {
		t.Fatalf("el detalle del corte no trae su explicación: %+v", detalle.ExplainedByNext)
	}
	detSig, err := back.SessionDetail(ctx, siguiente.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detSig.PreviousSessionID == nil || *detSig.PreviousSessionID != corteFuerte.ID || detSig.OpeningReason == nil {
		t.Fatalf("el detalle del turno nuevo no dice contra qué corte comparó ni su motivo: %v %v", detSig.PreviousSessionID, detSig.OpeningReason)
	}

	// AISLAMIENTO: otra empresa no ve ni el motivo ni el enlace.
	other := makeCompany(t, st, "otra-motivo-apertura")
	inTheThreeCases(t, defaultCompanyID, other, func(t *testing.T, s *store.Store, c context.Context) {
		filas, _, err := app.NewBackofficeService(s, clock).SessionHistoryFor(c, domain.SessionHistoryFilter{Limit: 50}, true)
		if err != nil {
			return
		}
		for _, f := range filas {
			if f.ID == corteFuerte.ID || f.ID == siguiente.ID {
				t.Fatalf("otra empresa leyó el corte %d con su motivo", f.ID)
			}
		}
	})
}

// COMPATIBILIDAD (#290, revisión): la PWA anterior y el MCP leen `totalDifference` de TODO turno, y
// antes de esta feature un turno abierto lo traía como número (la suma parcial). Mandarlo en null
// durante la subida dejaba al cliente viejo formateando un null.
func TestAnOpenShiftKeepsANumericTotalDifferenceForOldClients(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_compat_total", "cajero")
	principal, _, _, _ := turnoConEfectivoDeMostradorYDeApp(t, ctx, st, cajero)
	abierta, err := back.CurrentByRegister(ctx, principal)
	if err != nil {
		t.Fatal(err)
	}
	if fila := filaDelHistorico(t, ctx, back, abierta.ID, true); fila.TotalDifference == nil {
		t.Fatal("el turno abierto mandó totalDifference en null: un cliente anterior lo formateaba como número")
	}
}

// Un corte que CUADRA no tiene nada que explicar, aunque el turno siguiente haya abierto con motivo
// (p. ej. «cambié el fondo»): decir «diferencia explicada» junto a «Cuadra» afirma algo que no pasó.
func TestACutThatBalancesIsNotExplainedByTheNextOpening(t *testing.T) {
	t.Parallel()
	st := newTestStore(t)
	ctx := context.Background()
	back := app.NewBackofficeService(st, clock)
	cajero := makeUser(t, st, "cajero_cuadra_explica", "cajero")
	fuerte := registerID(t, st, "Caja fuerte")
	quinientos, seiscientos := dec("500"), dec("600")
	if _, err := back.OpenSession(ctx, fuerte, aperturaAMano(quinientos), cajero); err != nil {
		t.Fatal(err)
	}
	corte, err := back.CloseSession(ctx, fuerte, cajero, app.CierreCmd{Total: &quinientos, Motivo: "prueba"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := back.OpenSession(ctx, fuerte, app.AperturaCmd{Total: &seiscientos, Motivo: "prueba", Reason: "float_changed"}, cajero); err != nil {
		t.Fatal(err)
	}
	if f := filaDelHistorico(t, ctx, back, corte.ID, true); f.ExplainedByNext != nil {
		t.Fatalf("un corte que cuadra salió «explicado al abrir»: %+v (diferencia %+v)", f.ExplainedByNext, f.Difference)
	}
	det, err := back.SessionDetail(ctx, corte.ID)
	if err != nil {
		t.Fatal(err)
	}
	if det.ExplainedByNext != nil {
		t.Fatalf("el detalle de un corte que cuadra dice que se explicó al abrir: %+v", det.ExplainedByNext)
	}
}
