package uber

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// maxBytesDeDetalle acota el detalle de UN pedido. Un pedido con cincuenta renglones y sus opciones
// no llega a 100 KB; el techo es holgado y su razón de ser es que una respuesta degradada no se
// decodifique entera en memoria, igual que el del menú.
const maxBytesDeDetalle = 1 << 20

// TraerDetalleDePedido pide el pedido completo SIGUIENDO la liga que entrega el aviso.
//
// La liga NO se arma a mano, y eso no es estilo: el aviso trae un `resource_href` y ése es el
// contrato. Adivinar la ruta es lo que nos dio 404 en siete formas distintas buscando el detalle de
// un pedido, y el día que la plataforma cambie la forma de esa URL, seguir la liga sigue
// funcionando.
//
// LA LIGA VIENE DENTRO DEL CUERPO QUE MANDA QUIEN LLAMA, así que se verifica antes de seguirla.
// Seguirla a ciegas convierte esta API en un cliente que pide lo que le digan: quien consiga que se
// procese un aviso apunta la liga a una dirección interna y usa el servidor como sonda de la red.
// Por eso se exige el host de la plataforma, y por eso la comparación es por host exacto.
//
// Devuelve el cuerpo CRUDO. Interpretarlo es del dominio, y guardarlo tal cual es lo único desde lo
// cual se puede corregir un mapeo equivocado días después, cuando la plataforma ya no entregue ese
// pedido.
func (c *Client) TraerDetalleDePedido(ctx context.Context, liga string) ([]byte, error) {
	destino, err := c.ligaDeLaPlataforma(liga)
	if err != nil {
		return nil, err
	}
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, destino, nil)
	if err != nil {
		return nil, fmt.Errorf("armar la lectura del pedido: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: no se pudo traer el pedido de %s", ErrRespuesta, NombreDeLaPlataforma)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w: %s rechazó el token al traer un pedido", ErrAuth, NombreDeLaPlataforma)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: %s respondió HTTP %d al detalle del pedido", ErrRespuesta, NombreDeLaPlataforma, resp.StatusCode)
	}

	crudo, err := io.ReadAll(io.LimitReader(resp.Body, maxBytesDeDetalle))
	if err != nil {
		return nil, fmt.Errorf("%w: no se pudo leer el detalle del pedido", ErrRespuesta)
	}
	if !json.Valid(crudo) {
		// Puede ser un cuerpo truncado por el techo o una respuesta degradada. En los dos casos, lo
		// que NO se hace es guardarlo como si fuera un pedido: un detalle a medias mapea renglones
		// de menos y el cliente recibe comida incompleta.
		return nil, fmt.Errorf("%w: el detalle del pedido no es JSON completo", ErrRespuesta)
	}
	return crudo, nil
}

// AceptarPedido le confirma a la plataforma que el pedido se va a preparar.
//
// Es UNA de las dos únicas escrituras de todo este paquete, y pasa por la lista blanca del
// transporte. `pickupTime` es cuándo estará listo, si se sabe; vacío deja que la plataforma use su
// estimación.
func (c *Client) AceptarPedido(ctx context.Context, pedidoID, referenciaPropia string) error {
	cuerpo := map[string]any{"reason": "accepted"}
	if referenciaPropia != "" {
		// El folio del POS viaja a la plataforma para poder conciliar después sin adivinar.
		cuerpo["external_reference_id"] = referenciaPropia
	}
	return c.decidirPedido(ctx, pedidoID, "accept_pos_order", cuerpo)
}

// RechazarPedido le dice a la plataforma que no se va a preparar, y por qué.
//
// El motivo va contra la lista cerrada del dominio ANTES de salir: mandar un código inventado hace
// que la plataforma lo rechace o, peor, que lo interprete como otra cosa y el cliente reciba una
// explicación falsa de por qué no le hicieron su pedido.
func (c *Client) RechazarPedido(ctx context.Context, pedidoID, motivo, explicacion string) error {
	if !domain.MotivoDeRechazoValido(motivo) {
		return fmt.Errorf("%w: %q", domain.ErrMotivoDesconocido, motivo)
	}
	return c.decidirPedido(ctx, pedidoID, "deny_pos_order", map[string]any{
		"reason": map[string]any{"code": motivo, "explanation": explicacion},
	})
}

func (c *Client) decidirPedido(ctx context.Context, pedidoID, accion string, cuerpo map[string]any) error {
	if strings.TrimSpace(pedidoID) == "" {
		return fmt.Errorf("%w: sin id de pedido", ErrRespuesta)
	}
	tok, err := c.token(ctx)
	if err != nil {
		return err
	}
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return fmt.Errorf("armar la decisión del pedido: %w", err)
	}
	destino := fmt.Sprintf("%s/v1/eats/orders/%s/%s", c.apiBase, url.PathEscape(pedidoID), accion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, destino, bytes.NewReader(datos))
	if err != nil {
		return fmt.Errorf("armar la decisión del pedido: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: no se pudo avisar la decisión a %s", ErrRespuesta, NombreDeLaPlataforma)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %s rechazó el token al decidir un pedido", ErrAuth, NombreDeLaPlataforma)
	case resp.StatusCode >= 300:
		return fmt.Errorf("%w: %s respondió HTTP %d a la decisión del pedido", ErrRespuesta, NombreDeLaPlataforma, resp.StatusCode)
	}
	return nil
}

// ligaDeLaPlataforma acepta solo URLs absolutas del host de la API. Cualquier otra cosa —una IP
// interna, localhost, otro dominio, un esquema que no sea http(s)— se rechaza sin pedirla.
func (c *Client) ligaDeLaPlataforma(liga string) (string, error) {
	u, err := url.Parse(liga)
	if err != nil || !u.IsAbs() {
		return "", fmt.Errorf("%w: la liga del pedido no es una dirección completa", ErrRespuesta)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("%w: la liga del pedido usa un esquema que no se sigue", ErrRespuesta)
	}
	base, err := url.Parse(c.apiBase)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(u.Host, base.Host) {
		return "", fmt.Errorf("%w: la liga del pedido apunta a %s y no a %s", ErrRespuesta, u.Host, base.Host)
	}
	return u.String(), nil
}
