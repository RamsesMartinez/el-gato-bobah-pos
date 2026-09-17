package uber

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
)

// maxBytesDeTiendas: un comercio tiene sucursales, no miles. El techo evita que una respuesta
// degradada se decodifique entera en memoria, por lo mismo que el del menú.
const maxBytesDeTiendas = 1 << 20

type tiendasResponse struct {
	Stores []struct {
		StoreID  string `json:"store_id"`
		Name     string `json:"name"`
		Location struct {
			City string `json:"city"`
		} `json:"location"`
		PosData struct {
			IntegrationEnabled bool `json:"integration_enabled"`
		} `json:"pos_data"`
	} `json:"stores"`
}

// ListarTiendas devuelve las tiendas que esta credencial alcanza, para que alguien ELIJA la suya en
// vez de copiar un UUID.
//
// POR QUÉ IMPORTA MÁS DE LO QUE PARECE: el `store_id` es el único dato del alta que una persona no
// puede producir de memoria ni deducir. Pedirlo escrito es lo que hoy deja fuera a quien nunca ha
// usado el sistema.
//
// Y es el mismo endpoint que servirá el día que un comercio ajeno autorice la conexión: con un
// token de aplicación devuelve las tiendas de ESTA app, y con uno emitido tras el consentimiento
// del comerciante devuelve las de ESE comercio. La pantalla no cambia; cambia de dónde sale el
// token.
func (c *Client) ListarTiendas(ctx context.Context) ([]domain.TiendaDePlataforma, error) {
	tok, err := c.token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+"/v1/eats/stores", nil)
	if err != nil {
		return nil, fmt.Errorf("armar la lista de tiendas: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: no se pudieron listar las tiendas de %s", ErrRespuesta, NombreDeLaPlataforma)
	}
	defer func() { _ = resp.Body.Close() }()

	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w: %s rechazó el token al listar tiendas", ErrAuth, NombreDeLaPlataforma)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: %s respondió HTTP %d a las tiendas", ErrRespuesta, NombreDeLaPlataforma, resp.StatusCode)
	}

	crudo, err := io.ReadAll(io.LimitReader(resp.Body, maxBytesDeTiendas))
	if err != nil {
		return nil, fmt.Errorf("%w: no se pudo leer la lista de tiendas", ErrRespuesta)
	}
	var tr tiendasResponse
	if err := json.Unmarshal(crudo, &tr); err != nil {
		return nil, fmt.Errorf("%w: la lista de tiendas de %s no se pudo interpretar", ErrRespuesta, NombreDeLaPlataforma)
	}

	// Se copia campo por campo, no se reenvía la respuesta: así, un campo nuevo que la plataforma
	// agregue —otro dato de contacto, por ejemplo— no llega solo hasta la pantalla.
	out := make([]domain.TiendaDePlataforma, 0, len(tr.Stores))
	for _, s := range tr.Stores {
		out = append(out, domain.TiendaDePlataforma{
			ID: s.StoreID, Nombre: s.Name, Ciudad: s.Location.City,
			PDVConectado: s.PosData.IntegrationEnabled,
		})
	}
	return out, nil
}
