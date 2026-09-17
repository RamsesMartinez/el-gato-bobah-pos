package httpapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ramthedev/el-gato-bobah-pos/server/internal/app"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/domain"
	"github.com/ramthedev/el-gato-bobah-pos/server/internal/logging"
)

// maxBytesDeAviso acota el cuerpo ANTES de leerlo. Un aviso de la plataforma son unos cientos de
// bytes: cuatro campos y una liga. El techo es holgado y su razón de ser es que nadie pueda hacer
// que este proceso lea un cuerpo arbitrariamente grande en una ruta que no pide sesión.
const maxBytesDeAviso = 64 << 10

// WebhookDePlataforma recibe los avisos de una plataforma de reparto.
//
// **ES LA PRIMERA RUTA DEL NEGOCIO SIN SESIÓN DE USUARIO.** Quien llama es la plataforma, no una
// persona: no hay JWT, no hay empresa en el contexto y no puede haberla, porque la empresa es el
// resultado de autenticar el cuerpo. La autenticación entera es la firma.
//
// LO QUE NUNCA SALE DE AQUÍ, y es la mitad del diseño: el mismo 401 para firma ausente, mal formada,
// incorrecta y para una tienda que no es de nadie nuestro. Distinguirlos le diría a quien prueba a
// ciegas cuándo va acertando el identificador de una tienda real — y esos identificadores son lo
// único que hace falta saber para intentarlo.
func (h *Handlers) WebhookDePlataforma(w http.ResponseWriter, r *http.Request) {
	plataforma := plataformaDeRuta(chi.URLParam(r, "plataforma"))
	if plataforma == "" {
		Error(w, domain.ErrNotFound)
		return
	}

	// El techo va ANTES de leer, no después: leer entero para luego medir es justo lo que se quiere
	// evitar. MaxBytesReader corta el cuerpo y hace que el Read devuelva error.
	r.Body = http.MaxBytesReader(w, r.Body, maxBytesDeAviso)
	crudo, err := io.ReadAll(r.Body)
	if err != nil {
		JSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "cuerpo demasiado grande"})
		return
	}

	err = h.pedidosPlataforma.RecibirAviso(r.Context(), plataforma, app.AvisoEntrante{
		Crudo: crudo,
		// El nombre de la cabecera lo fija la plataforma.
		Firma:    r.Header.Get("X-Uber-Signature"),
		Ambiente: r.Header.Get("X-Environment"),
	})

	switch {
	case err == nil:
		// 200 CON CUERPO VACÍO, que es lo que la plataforma espera. Cualquier otra cosa la hace
		// reintentar un aviso que ya procesamos.
		w.WriteHeader(http.StatusOK)

	case errors.Is(err, domain.ErrFirmaInvalida):
		// El evento de seguridad lleva clave estable e identificadores, NUNCA el cuerpo (que trae
		// datos del cliente) ni la llave. Es lo que permite notar un barrido sin guardar el barrido.
		logging.SecurityEvent(r.Context(), "webhook_no_autenticado",
			"platform", plataforma, "ip", clientIP(r), "bytes", len(crudo))
		JSON(w, http.StatusUnauthorized, map[string]string{"error": "no autorizado"})

	case errors.Is(err, domain.ErrAmbienteEquivocado):
		logging.SecurityEvent(r.Context(), "webhook_de_otro_ambiente",
			"platform", plataforma, "ip", clientIP(r))
		JSON(w, http.StatusBadRequest, map[string]string{"error": "solicitud inválida"})

	case errors.Is(err, domain.ErrValidation):
		JSON(w, http.StatusBadRequest, map[string]string{"error": "solicitud inválida"})

	default:
		// 5xx A PROPÓSITO: es lo que hace que la plataforma reintente. Contestar 200 a un aviso que
		// falló lo pierde en silencio, y el cliente se queda esperando comida que nadie prepara.
		// El detalle del error ya quedó en el registro del aviso, con su clase y sin el mensaje
		// crudo de la API ajena.
		JSON(w, http.StatusBadGateway, map[string]string{"error": "no se pudo procesar"})
	}
}

// plataformaDeRuta traduce el segmento de la URL al nombre de la plataforma, contra una lista
// cerrada. No se interpola lo que venga: el valor viaja a una consulta y a los mensajes de error, y
// un segmento sin acotar termina en los registros de seguridad — el mismo defecto que ya se cerró
// para el nombre de usuario.
func plataformaDeRuta(s string) string {
	switch s {
	case "uber-eats":
		return "Uber Eats"
	default:
		return ""
	}
}
