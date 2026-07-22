// Package mailer envía correo por SMTP con la stdlib (net/smtp), sin dependencias. Cubre dos
// modos con el MISMO código de construcción de mensaje:
//   - Local (Mailpit): host:1025 sin auth ni TLS.
//   - Prod (Zoho Mail): smtp.zoho.com:587 STARTTLS + auth, o :465 TLS implícito + auth.
//
// Es best-effort para correo transaccional de bajo volumen (recuperación de contraseña); no
// reintenta ni encola (YAGNI: si Zoho falla, el usuario reintenta o el admin resetea a mano).
package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type Mailer struct {
	host string
	port int
	user string
	pass string
	from string
}

func New(host string, port int, user, pass, from string) *Mailer {
	return &Mailer{host: host, port: port, user: user, pass: pass, from: from}
}

// Enabled reports whether a host is configured (si no, la recuperación por email no aplica).
func (m *Mailer) Enabled() bool { return m.host != "" }

// Send entrega un correo HTML. Toda la sesión SMTP está ACOTADA en tiempo (dial timeout +
// deadline de conexión), imprescindible porque el envío corre en una goroutine desacoplada del
// request (ver ResetService.Request): sin cota, un SMTP colgado filtraría goroutines (§4).
// Transporte por puerto/credenciales:
//   - port 465 → TLS implícito + AUTH (Zoho SSL).
//   - user!="" en otro puerto → STARTTLS si el server lo ofrece + AUTH (Zoho :587).
//   - user=="" → plano sin auth (Mailpit local).
func (m *Mailer) Send(to, subject, htmlBody string) error {
	if !m.Enabled() {
		return fmt.Errorf("mailer deshabilitado (SMTP_HOST vacío)")
	}
	addr := net.JoinHostPort(m.host, fmt.Sprintf("%d", m.port))
	msg := buildMessage(m.from, to, subject, htmlBody)
	tlsCfg := &tls.Config{ServerName: m.host}

	// Dial acotado por timeout (10s) + deadline de sesión (abajo). Background: Send es
	// fire-and-forget desde una goroutine sin ctx de request; el término lo dan los timeouts.
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	dialCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var conn net.Conn
	var err error
	if m.port == 465 {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: tlsCfg}).DialContext(dialCtx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(dialCtx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	// Deadline global de la sesión: garantiza que Send (y su goroutine) SIEMPRE termina.
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	c, err := smtp.NewClient(conn, m.host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = c.Close() }()

	if m.port != 465 && m.user != "" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				return err
			}
		}
	}
	if m.user != "" {
		if err := c.Auth(smtp.PlainAuth("", m.user, m.pass, m.host)); err != nil {
			return err
		}
	}
	if err := c.Mail(m.from); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	wc, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := wc.Write(msg); err != nil {
		return err
	}
	if err := wc.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func buildMessage(from, to, subject, htmlBody string) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + to + "\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(htmlBody)
	return []byte(b.String())
}
