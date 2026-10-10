package domain

import (
	"errors"
	"strings"
	"testing"
)

// LO QUE SE PEGA DEL TABLERO DE UN TERCERO LLEGA SUCIO. El caso real es copiar con el renglón o con
// espacios; el que cuesta caro es pegar el secret en el field del id (o los dos en uno): se guardaría,
// la plataforma lo rechazaría y el mensaje diría «revisa tus credenciales» sin decir cuál.
func TestNormalizeAppCredentials(t *testing.T) {
	const id, secret = "Ab3dE-fGhIjKlMnOpQrStUvWxYz012345", "s3cr3t-0123456789abcdefghij"
	got, err := NormalizeAppCredentials("  "+id+"\n", "\t"+secret+"\r\n")
	if err != nil || got.ClientID != id || got.ClientSecret != secret {
		t.Fatalf("limpieza al pegar: %+v, %v", got, err)
	}

	for _, c := range []struct{ name, id, secret, field string }{
		{"id vacío", "", secret, "Client ID"},
		{"id de 7, un dedazo", "abcdefg", secret, "Client ID"},
		{"id con espacio adentro", "abcd efgh ijkl", secret, "Client ID"},
		{"secret vacío", id, "", "Client Secret"},
		{"secret de 15", id, "0123456789abcde", "Client Secret"},
		{"secret de más de 512", id, strings.Repeat("a", 513), "Client Secret"},
		{"secret con byte nulo", id, "0123456789\x00abcdefgh", "Client Secret"},
		{"secret con salto adentro", id, "0123456789\nabcdefgh", "Client Secret"},
		// Los dos iguales: se pegó el mismo valor dos veces.
		{"id y secret iguales", secret, secret, "Client Secret"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := NormalizeAppCredentials(c.id, c.secret)
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("esperaba ErrValidation, fue %v", err)
			}
			// El mensaje nombra el field: es lo único que le dice a quien opera qué volver a pegar.
			if !strings.Contains(err.Error(), c.field) {
				t.Fatalf("el mensaje %q no dice cuál field corregir (%s)", err, c.field)
			}
		})
	}
}

// El secret NUNCA aparece en el mensaje de error: el mensaje viaja en la respuesta HTTP y puede
// terminar en un log o en una captura de pantalla.
func TestTheCaptureErrorNeverRepeatsTheSecret(t *testing.T) {
	const secret = "0123456789\x00abcdefgh-un-secret"
	_, err := NormalizeAppCredentials("un-client-id-bueno", secret)
	if err == nil || strings.Contains(err.Error(), "un-secret") {
		t.Fatalf("el error no debe repetir el secret: %v", err)
	}
}

// El contexto que ata un cifrado a su dueño. Cambiarle el formato vuelve ilegible TODO lo que ya se
// guardó, así que este test lo congela a propósito.
func TestCredentialAADIsStable(t *testing.T) {
	if got := CredentialAAD(2, 6, SecretClientSecret); got != "credential|2|6|client_secret" {
		t.Fatalf("formato cambiado: %q — todo lo ya cifrado dejaría de descifrar", got)
	}
	if got := CredentialAAD(2, 6, SecretSigningKey); got != "credential|2|6|signing_key" {
		t.Fatalf("formato cambiado: %q", got)
	}
}
