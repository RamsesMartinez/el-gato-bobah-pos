package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Fotos del catálogo (spec 091). El navegador o la computadora del catálogo preparan dos WebP
// cuadrados; aquí solo se decide si lo que llegó es eso. El servidor no decodifica píxeles: con
// un solo formato aceptado, servido desde otro dominio con el Content-Type que fija la API, un
// archivo que pasa la cabecera y no decodifica se ve roto, no ejecuta nada.
const (
	MediaSmallSide     = 400
	MediaLargeSide     = 1200
	MaxMediaSmallBytes = 100 << 10
	MaxMediaLargeBytes = 400 << 10
	// MediaCacheControl va en cada objeto: el nombre lleva la huella del contenido, así que un
	// archivo publicado no cambia nunca y se puede guardar un año.
	MediaCacheControl = "public, max-age=31536000, immutable"
	MediaContentType  = "image/webp"
)

// Errores de la foto. No envuelven ErrValidation: cada uno tiene su código para que la pantalla y
// el MCP distingan «otro archivo» de «la misma foto, más chica».
var (
	ErrImageInvalid  = errors.New("foto no válida")
	ErrImageTooLarge = errors.New("foto demasiado pesada")
	// ErrMediaDisabled: el despliegue no tiene almacenamiento configurado. La venta sigue igual.
	ErrMediaDisabled = errors.New("las fotos no están configuradas en este servidor")
	// ErrMediaUnavailable: el almacenamiento no respondió; el registro queda como estaba.
	ErrMediaUnavailable = errors.New("no se pudo guardar la foto; vuelve a intentar")
)

// ValidateWebP acepta un WebP estático de exactamente side×side y a lo más maxBytes. Lee solo las
// cabeceras RIFF y del primer chunk.
func ValidateWebP(data []byte, side, maxBytes int) error {
	if len(data) == 0 {
		return fmt.Errorf("%w: la foto de %d no llegó", ErrImageInvalid, side)
	}
	if len(data) > maxBytes {
		return fmt.Errorf("%w: la foto de %d pesa %d KB; el máximo es %d KB", ErrImageTooLarge, side,
			(len(data)+1023)>>10, maxBytes>>10)
	}
	w, h, flags, ok := webpSize(data)
	if !ok {
		return fmt.Errorf("%w: solo se aceptan fotos WebP (la de %d no lo es)", ErrImageInvalid, side)
	}
	if flags&vp8xAnimation != 0 {
		return fmt.Errorf("%w: la foto no puede ser animada", ErrImageInvalid)
	}
	// La pantalla vuelve a codificar y la metadata se pierde; lo que sube el MCP llega tal cual, y un
	// EXIF con la ubicación del teléfono quedaría público un año en el dominio de medios.
	if flags&(vp8xEXIF|vp8xXMP) != 0 {
		return fmt.Errorf("%w: la foto de %d trae metadatos (EXIF o XMP); quítalos al convertirla (magick … -strip)", ErrImageInvalid, side)
	}
	if w != side || h != side {
		return fmt.Errorf("%w: la foto de %d mide %d×%d; debe medir %d×%d", ErrImageInvalid, side, w, h, side, side)
	}
	return nil
}

// Banderas del chunk VP8X.
const (
	vp8xAnimation = 0x02
	vp8xXMP       = 0x04
	vp8xEXIF      = 0x08
)

// webpSize lee ancho, alto y banderas del primer chunk. Un RIFF cuyo tamaño declarado no es el del
// archivo se rechaza: es un archivo truncado o con otra cosa pegada.
func webpSize(b []byte) (w, h int, flags byte, ok bool) {
	if len(b) < 30 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return 0, 0, 0, false
	}
	if int(binary.LittleEndian.Uint32(b[4:8])) != len(b)-8 {
		return 0, 0, 0, false
	}
	p := b[20:]
	switch string(b[12:16]) {
	case "VP8 ":
		// Cuadro clave (bit 0 en 0) con su código de arranque; solo él trae las dimensiones.
		if p[0]&0x01 != 0 || p[3] != 0x9d || p[4] != 0x01 || p[5] != 0x2a {
			return 0, 0, 0, false
		}
		return int(binary.LittleEndian.Uint16(p[6:8]) & 0x3fff), int(binary.LittleEndian.Uint16(p[8:10]) & 0x3fff), 0, true
	case "VP8L":
		if p[0] != 0x2f {
			return 0, 0, 0, false
		}
		bits := binary.LittleEndian.Uint32(p[1:5])
		return int(bits&0x3fff) + 1, int(bits>>14&0x3fff) + 1, 0, true
	case "VP8X":
		w = int(p[4]) | int(p[5])<<8 | int(p[6])<<16
		h = int(p[7]) | int(p[8])<<8 | int(p[9])<<16
		return w + 1, h + 1, p[0], true
	}
	return 0, 0, 0, false
}

// MediaHash es la huella con la que se nombra la foto: 32 hex del SHA-256 de la variante grande,
// calculada aquí y nunca la que mande el cliente.
func MediaHash(large []byte) string {
	sum := sha256.Sum256(large)
	return hex.EncodeToString(sum[:16])
}

// MediaKind es la carpeta de la entidad dueña de la foto.
type MediaKind string

const (
	MediaProducts   MediaKind = "products"
	MediaCategories MediaKind = "categories"
)

// MediaObjectKey es la llave del objeto dentro del bucket. La empresa va primero y sale de la
// sesión: así ninguna entrada del cliente decide dónde se escribe.
func MediaObjectKey(companyID int64, kind MediaKind, id int64, hash string, side int) string {
	return fmt.Sprintf("%d/%s/%d/%s-%d.webp", companyID, kind, id, hash, side)
}

// MediaURL es la dirección pública de un objeto.
func MediaURL(base, key string) string { return strings.TrimRight(base, "/") + "/" + key }

var mediaFileName = regexp.MustCompile(`^([0-9a-f]{32})-1200\.webp$`)

// MediaKeysFromURL devuelve las llaves de las dos variantes de una image_url, solo si la armó esta
// API para esa misma empresa y entidad. Cualquier otra cosa (otro dominio, otra empresa, otra
// entidad, una URL escrita a mano) se deja en paz: borrarla sería borrar un archivo ajeno.
func MediaKeysFromURL(base, url string, companyID int64, kind MediaKind, id int64) ([]string, bool) {
	prefix := strings.TrimRight(base, "/") + "/" + strconv.FormatInt(companyID, 10) + "/" + string(kind) + "/" +
		strconv.FormatInt(id, 10) + "/"
	if base == "" || !strings.HasPrefix(url, prefix) {
		return nil, false
	}
	m := mediaFileName.FindStringSubmatch(strings.TrimPrefix(url, prefix))
	if m == nil {
		return nil, false
	}
	return []string{
		MediaObjectKey(companyID, kind, id, m[1], MediaSmallSide),
		MediaObjectKey(companyID, kind, id, m[1], MediaLargeSide),
	}, true
}

// FoodFamily es la familia de comida de una categoría: decide la ilustración de lo que no tiene
// foto. Es del producto, no de la empresa; el enum food_family de la 0117 y el sprite del front
// tienen la misma lista.
type FoodFamily string

// FoodFamilies en el orden en que se ofrecen.
var FoodFamilies = []FoodFamily{
	"cold_drink", "coffee", "tea", "frappe", "soda", "soup", "snack", "wings", "dessert", "breakfast", "dish",
}

// ValidFoodFamily dice si s es una familia conocida.
func ValidFoodFamily(s string) bool {
	for _, f := range FoodFamilies {
		if string(f) == s {
			return true
		}
	}
	return false
}
