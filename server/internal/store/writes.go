package store

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

// WriteTracker anota si, durante una operación, algo pudo haber quedado escrito en la base: una
// transacción que intentó confirmar o una sentencia que no es de lectura fuera de transacción (ésas se
// confirman solas). Lo usa el MCP (#335) para saber si una escritura que falló es segura de reintentar:
// sin nada anotado, la base no cambió.
type WriteTracker struct{ wrote atomic.Bool }

// MayHaveWritten dice si algo pudo haberse confirmado. Ante la duda dice que sí: una llave que se queda
// en «unknown» de más cuesta una lectura; una en «failed» de más, una escritura doble.
func (w *WriteTracker) MayHaveWritten() bool { return w.wrote.Load() }

type writeTrackerKey struct{}

// TrackWrites devuelve un contexto que anota en el rastreador lo que se escriba con él.
func TrackWrites(ctx context.Context) (context.Context, *WriteTracker) {
	w := &WriteTracker{}
	return context.WithValue(ctx, writeTrackerKey{}, w), w
}

func noteWrite(ctx context.Context) {
	if w, ok := ctx.Value(writeTrackerKey{}).(*WriteTracker); ok {
		w.wrote.Store(true)
	}
}

// noteStatement anota una sentencia fuera de transacción que no es una lectura. Solo «select» cuenta
// como lectura: un «with» puede llevar un insert adentro y se toma como escritura.
func (t translating) noteStatement(ctx context.Context, sql string) {
	if _, inTx := t.conn.(pgx.Tx); inTx {
		return
	}
	if !isPlainSelect(sql) {
		noteWrite(ctx)
	}
}

func isPlainSelect(sql string) bool {
	s := strings.TrimSpace(sql)
	for strings.HasPrefix(s, "--") {
		_, rest, ok := strings.Cut(s, "\n")
		if !ok {
			return false
		}
		s = strings.TrimSpace(rest)
	}
	return len(s) >= 6 && strings.EqualFold(s[:6], "select")
}
