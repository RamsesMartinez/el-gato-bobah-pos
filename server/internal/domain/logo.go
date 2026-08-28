package domain

import (
	"bytes"
	"fmt"
	"image"
	"net/http"

	// Registran los decoders que usa image.DecodeConfig. Son los DOS formatos de la lista blanca:
	// aceptar uno que la stdlib no sepa leer (WebP, por ejemplo) sería aceptar un archivo cuyas
	// dimensiones no podemos acotar, y sumar golang.org/x/image solo para leer un header.
	_ "image/jpeg"
	_ "image/png"
)

// Topes del logo del ticket. Viven aquí y no en el handler porque el mismo límite se replica como
// check en la base: son dos candados sobre el mismo número.
const (
	MaxLogoBytes = 256 << 10 // 256 KB
	MaxLogoSide  = 1024      // px por lado
)

// Errores de la subida del logo. Envuelven ErrValidation a propósito: httpapi.Error ya lo mapea a
// 400 con el mensaje real, así que un rechazo nunca puede salir como 500 opaco.
var (
	ErrLogoTooLarge   = fmt.Errorf("%w: la imagen excede %d KB", ErrValidation, MaxLogoBytes>>10)
	ErrLogoType       = fmt.Errorf("%w: el archivo debe ser PNG o JPEG", ErrValidation)
	ErrLogoDimensions = fmt.Errorf("%w: la imagen excede %d px por lado", ErrValidation, MaxLogoSide)
)

// ValidateLogo acepta o rechaza una imagen para el encabezado del ticket y devuelve el mime REAL,
// deducido del contenido. El Content-Type que declara quien sube no se mira siquiera: lo escribe el
// cliente y el contenido no miente.
func ValidateLogo(data []byte) (string, error) {
	if len(data) == 0 {
		return "", fmt.Errorf("%w: no se recibió ninguna imagen", ErrValidation)
	}
	if len(data) > MaxLogoBytes {
		return "", ErrLogoTooLarge
	}

	// DetectContentType mira los primeros 512 bytes; es lo que descarta un .txt renombrado y, de
	// paso, el SVG (que es XML con <script> adentro y sería XSS servido desde nuestro origen).
	switch mime := http.DetectContentType(data); mime {
	case "image/png", "image/jpeg":
		// DecodeConfig lee solo el header: da las dimensiones sin descomprimir la imagen entera,
		// así que una "bomba" de 20000x20000 se rechaza sin reventar la memoria del proceso.
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return "", ErrLogoType
		}
		if cfg.Width > MaxLogoSide || cfg.Height > MaxLogoSide {
			return "", ErrLogoDimensions
		}
		return mime, nil
	default:
		return "", ErrLogoType
	}
}
