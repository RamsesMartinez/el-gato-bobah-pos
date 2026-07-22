// Package hibp verifica contraseñas contra la base de brechas de Have I Been Pwned usando el
// modelo k-anonymity: se envía SOLO los primeros 5 hex del SHA-1 de la contraseña; la API
// devuelve todos los sufijos con ese prefijo y se compara localmente. La contraseña en claro
// NUNCA sale del proceso. Fail-open lo decide quien llama (config): si HIBP no responde, no se
// bloquea el alta de usuarios (un POS puede estar sin internet), pero se registra el evento.
package hibp

import (
	"context"
	"crypto/sha1" //nolint:gosec // G505: SHA-1 lo EXIGE el protocolo k-anonymity de HIBP (no es uso criptográfico)
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const apiRange = "https://api.pwnedpasswords.com/range/"

// Client consulta HIBP. Reutilizable; usa el http.Client dado (con timeout) o el default.
type Client struct {
	http *http.Client
}

func New(hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{http: hc}
}

// Pwned reporta si la contraseña aparece en alguna brecha conocida. El error NO es "está
// filtrada": es un fallo de red/servicio (el llamador decide fail-open). Un nil error con
// pwned=false significa "verificada y limpia".
func (c *Client) Pwned(ctx context.Context, password string) (bool, error) {
	sum := sha1.Sum([]byte(password)) //nolint:gosec // SHA-1 es lo que exige el protocolo k-anonymity de HIBP, no es uso criptográfico
	h := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := h[:5], h[5:]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiRange+prefix, nil)
	if err != nil {
		return false, err
	}
	// Add-Padding: HIBP rellena la respuesta con sufijos falsos (count 0) para que el tamaño no
	// filtre cuántos matches hubo. Los ignoramos al parsear (count 0).
	req.Header.Set("Add-Padding", "true")
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("hibp: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(body), "\n") {
		sfx, count, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		if strings.EqualFold(sfx, suffix) && strings.TrimSpace(count) != "0" {
			return true, nil
		}
	}
	return false, nil
}
