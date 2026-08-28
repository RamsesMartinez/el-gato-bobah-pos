package domain

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// pngOf y jpegOf generan imágenes reales del tamaño pedido: validar por contenido exige contenido
// de verdad, no un string que parezca una imagen.
func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.Black)
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

func jpegOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatalf("jpeg.Encode: %v", err)
	}
	return buf.Bytes()
}

func TestValidateLogo(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		wantMime string
		wantErr  error
	}{
		{name: "png válido", data: pngOf(t, 64, 64), wantMime: "image/png"},
		{name: "jpeg válido", data: jpegOf(t, 64, 64), wantMime: "image/jpeg"},
		{name: "vacío", data: nil, wantErr: ErrValidation},
		{name: "no es imagen", data: []byte("esto es un .txt renombrado a .png"), wantErr: ErrLogoType},
		{
			// El vector es XML con <script> adentro: servido desde nuestro origen sería XSS con
			// acceso al token. Que no esté en la lista blanca es la única razón por la que no entra.
			name:    "svg con script",
			data:    []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
			wantErr: ErrLogoType,
		},
		{name: "más ancho que el tope", data: pngOf(t, 1025, 10), wantErr: ErrLogoDimensions},
		{name: "más alto que el tope", data: pngOf(t, 10, 1025), wantErr: ErrLogoDimensions},
		{name: "en el tope exacto", data: pngOf(t, 1024, 1024), wantMime: "image/png"},
		{name: "excede 256 KB", data: bytes.Repeat([]byte{0x89}, MaxLogoBytes+1), wantErr: ErrLogoTooLarge},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mime, err := ValidateLogo(tc.data)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				// Todo rechazo tiene que ser 400 y no 500: el mapeo cuelga de ErrValidation.
				if !errors.Is(err, ErrValidation) {
					t.Errorf("%v no envuelve ErrValidation → saldría como 500", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if mime != tc.wantMime {
				t.Errorf("mime = %q, want %q", mime, tc.wantMime)
			}
		})
	}
}

// El tipo sale del CONTENIDO, no de lo que diga quien sube: el header del multipart lo escribe el
// atacante y el contenido no miente.
func TestValidateLogoClasificaPorContenido(t *testing.T) {
	mime, err := ValidateLogo(jpegOf(t, 32, 32))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if mime != "image/jpeg" {
		t.Errorf("un JPEG se clasificó como %q; si se guarda ese mime, el navegador recibe una promesa falsa", mime)
	}
}
