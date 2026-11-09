package main

import (
	"testing"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/config"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/mailer"
)

// Sin destinatario no hay notifier: los avisos quedan apagados y el resto de la API no lo nota
// (un *Notifier nil es válido en Report y Close).
func TestServerAlertsAreOffWithoutARecipient(t *testing.T) {
	cfg := config.Config{SMTPHost: "localhost"}
	if n := serverAlerts(cfg, mailer.New("localhost", 1025, "", "", "a@b.c"), nil); n != nil {
		t.Fatal("sin ALERT_EMAIL_TO se armó un notifier")
	}
}
