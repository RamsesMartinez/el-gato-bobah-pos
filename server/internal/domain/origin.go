package domain

import (
	"encoding/json"
	"net/netip"
	"regexp"
)

// RequestOrigin es de dónde vino una acción: la red, la descripción del aparato y la clave que el
// propio navegador genera. Es una pista de auditoría, no una prueba: la clave se renueva si se borran
// los datos del navegador y el aparato la puede falsificar (decisión del dueño, 2026-10-10).
type RequestOrigin struct {
	IP        *netip.Addr
	Device    json.RawMessage
	DeviceKey *string
}

var deviceKeyRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// maxDeviceBytes acota lo que el navegador puede meter en cada evento de la bitácora.
const maxDeviceBytes = 300

// deviceFields son las únicas llaves de la descripción que se guardan; cualquier otra que mande el
// navegador se descarta.
var deviceFields = []string{"platform", "platformVersion", "model", "browser", "screen"}

// NormalizeOrigin limpia lo que llega en los headers del request. Nunca falla: un header mal
// formado deja ese dato vacío, porque rechazar una venta por una pista de auditoría sería peor que
// perder la pista.
func NormalizeOrigin(ip, deviceKey, deviceInfo string) RequestOrigin {
	var o RequestOrigin
	if a, err := netip.ParseAddr(ip); err == nil {
		o.IP = &a
	}
	if deviceKeyRe.MatchString(deviceKey) {
		o.DeviceKey = &deviceKey
	}
	o.Device = normalizeDevice(deviceInfo)
	return o
}

func normalizeDevice(raw string) json.RawMessage {
	var in map[string]any
	if raw == "" || json.Unmarshal([]byte(raw), &in) != nil || in == nil {
		return nil
	}
	// Cada valor se recorta por igual hasta que el total cabe: así un campo enorme no se come a los
	// demás.
	for limit := 120; limit >= 8; limit -= 16 {
		out := map[string]string{}
		for _, k := range deviceFields {
			if s, ok := in[k].(string); ok && s != "" {
				if len(s) > limit {
					s = s[:limit]
				}
				out[k] = s
			}
		}
		if len(out) == 0 {
			return nil
		}
		if b, err := json.Marshal(out); err == nil && len(b) <= maxDeviceBytes {
			return b
		}
	}
	return nil
}
