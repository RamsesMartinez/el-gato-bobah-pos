package app

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/realtime"
)

// NotificationListener pasa al Broker los avisos que abre o cierra la base (spec 084, R3). El trigger
// de existencias no alcanza al Broker, que vive en memoria de la API: avisa con NOTIFY, que Postgres
// entrega solo si la transacción confirma. Cada API (azul y verde, durante una subida) escucha y
// avisa a sus propios clientes.
//
// La conexión es propia y fuera del pool: LISTEN la ocupa todo el tiempo, y en el pool le quitaría
// una a las peticiones. No lee tablas, así que no necesita empresa.
type NotificationListener struct {
	url    string
	broker *realtime.Broker
	ready  chan struct{}
	once   sync.Once
}

// NewNotificationListener arma el listener contra la base de la aplicación.
func NewNotificationListener(databaseURL string, broker *realtime.Broker) *NotificationListener {
	return &NotificationListener{url: databaseURL, broker: broker, ready: make(chan struct{})}
}

// WaitReady espera a que la primera conexión esté escuchando (para pruebas).
func (l *NotificationListener) WaitReady(d time.Duration) bool {
	select {
	case <-l.ready:
		return true
	case <-time.After(d):
		return false
	}
}

// Run escucha hasta que se cancele ctx. Si la conexión se cae, reintenta con espera creciente: sin
// conexión, la campana sigue con su relectura de cada minuto.
func (l *NotificationListener) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		if err := l.listen(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("notification_listener_down", "err", err.Error(), "retry_in", backoff.String())
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			backoff = min(backoff*2, time.Minute)
			continue
		}
		backoff = time.Second
	}
}

func (l *NotificationListener) listen(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, l.url)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	if _, err := conn.Exec(ctx, "listen notifications"); err != nil {
		return err
	}
	l.once.Do(func() { close(l.ready) })
	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		company, err := strconv.ParseInt(n.Payload, 10, 64)
		if err != nil {
			continue
		}
		l.broker.Publish(company, realtime.Event{Type: "notifications.changed"})
	}
}
