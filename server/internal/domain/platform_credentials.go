package domain

import (
	"errors"
	"fmt"
	"strings"
)

// AppCredentials son las de la aplicación que la empresa registró en el tablero de la
// plataforma. Se capturan en pantalla, por empresa: una sola pareja en el entorno haría que la
// empresa B aceptara pedidos con la aplicación de la A.
type AppCredentials struct {
	ClientID     string
	ClientSecret string
}

// SecretKind dice qué es lo que se cifró. Entra en la AAD: un client secret no descifra como
// llave de firma aunque sea de la misma empresa y plataforma.
type SecretKind string

const (
	SecretClientSecret SecretKind = "client_secret"
	// SecretSigningKey es el MISMO para la primaria y la secundaria, a propósito: rotar mueve el
	// cifrado de una columna a la otra sin volver a cifrar, y con la ranura en la AAD dejaría de
	// descifrar al moverse.
	SecretSigningKey SecretKind = "signing_key"
)

// CredentialAAD es la AAD de un secreto de plataforma. Su formato NO se cambia: todo lo ya
// guardado se cifró con él.
//
// Hoy el id de plataforma ya basta para distinguir empresas —`delivery_platforms.id` es global y
// cada fila es de una sola—, y por eso `TestCompanyBDoesNotUseCompanyAsApp` sigue en verde aunque se
// quite la empresa de aquí (medido). La empresa va igual para que la barrera no dependa de esa
// propiedad del esquema: el día que las plataformas se vuelvan un catálogo compartido, seguiría en pie.
func CredentialAAD(company int64, platform int16, kind SecretKind) string {
	return fmt.Sprintf("credential|%d|%d|%s", company, platform, kind)
}

// Sentinels de la captura y el uso de credenciales. El mapeo a HTTP vive solo en httpapi.Error.
var (
	// ErrCredentialsRejected: la plataforma no reconoce la pareja. Casi siempre es una copia
	// incompleta o la app del otro ambiente (pruebas contra producción).
	ErrCredentialsRejected = errors.New("la plataforma no reconoce esas credenciales")
	// ErrCredentialsMissingScopes: la pareja es buena pero a la app le faltan permisos.
	ErrCredentialsMissingScopes = errors.New("la app de la plataforma no tiene los permisos que se necesitan")
	// ErrPlatformUnavailable: no se pudo comprobar. No se guarda nada: guardar sin comprobar es
	// justo lo que la comprobación viene a evitar.
	ErrPlatformUnavailable = errors.New("la plataforma no respondió")
	// ErrKeyServiceUnavailable: el servicio que cifra (Cloud KMS) no respondió. No se guardó nada y
	// reintentar es lo correcto; distinto de un error interno para que la pantalla lo diga así.
	ErrKeyServiceUnavailable = errors.New("el servicio de seguridad no respondió")
	// ErrCredentialsUnreadable: hay credenciales guardadas pero este ambiente no las puede
	// leer — un respaldo de otro ambiente restaurado aquí. No es una falla: es la protección
	// funcionando, y la pantalla lo dice como «vuelve a capturarlas», no como un 500.
	ErrCredentialsUnreadable = errors.New("las credenciales guardadas no se pueden leer en este ambiente: hay que capturarlas otra vez")
)

// NormalizeAppCredentials limpia lo que se pega del tablero y rechaza lo que no puede ser una
// credencial. El mensaje nombra el campo —es lo único que le dice a quien opera qué volver a pegar—
// y nunca repite el valor.
func NormalizeAppCredentials(clientID, clientSecret string) (AppCredentials, error) {
	id, err := normalizeCredentialPart("Client ID", clientID, 8, 256)
	if err != nil {
		return AppCredentials{}, err
	}
	secret, err := normalizeCredentialPart("Client Secret", clientSecret, 16, 512)
	if err != nil {
		return AppCredentials{}, err
	}
	if id == secret {
		return AppCredentials{}, fmt.Errorf("%w: el Client Secret es igual al Client ID; copia el secret de su propio campo", ErrValidation)
	}
	return AppCredentials{ClientID: id, ClientSecret: secret}, nil
}

// normalizeCredentialPart aplica la misma regla que la llave de firma: sin espacios alrededor, solo ASCII
// imprimible adentro. El byte nulo, que Postgres rechaza con un 500, rebota aquí como 400.
func normalizeCredentialPart(field, v string, min, max int) (string, error) {
	v = strings.TrimSpace(v)
	if len(v) < min || len(v) > max {
		return "", fmt.Errorf("%w: el %s va entre %d y %d caracteres", ErrValidation, field, min, max)
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x21 || v[i] > 0x7e {
			return "", fmt.Errorf("%w: el %s tiene espacios o caracteres que no son de una credencial", ErrValidation, field)
		}
	}
	return v, nil
}
