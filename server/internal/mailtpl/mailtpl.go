// Package mailtpl arma los correos del servidor con las plantillas compiladas desde MJML (spec 084).
//
// La fuente vive en emails/ y se compila en el build (cd web && bun run compilar-correos): aquí solo
// llega el HTML resultante, embebido en el binario y llenado con html/template, que escapa lo que
// escribieron las personas. El servidor no depende de MJML.
package mailtpl

import (
	"bytes"
	"embed"
	"html/template"
)

//go:embed templates/*.html.tmpl
var templates embed.FS

var stock = template.Must(template.ParseFS(templates, "templates/stock.html.tmpl"))

// StockEmail es el correo de avisos de inventario: el de cada bloque de 5 minutos y el de la mañana.
type StockEmail struct {
	BusinessName string
	Period       string
	Title        string
	Subtitle     string
	LogoURL      string
	OpenURL      string
	Footer       string
	Groups       []Group
}

// Group es un tipo de aviso con sus artículos.
type Group struct {
	Title string
	Items []Item
}

// Item es un aviso: qué, qué pasó y, si se sabe, qué comprar y por qué.
type Item struct {
	Kind     string
	Name     string
	Detail   string
	Buy      string
	Supplier string
	Why      string
}

// ColorFor es el color del borde de cada tarjeta, por tipo de aviso (los de la marca y del panel).
func (StockEmail) ColorFor(kind string) template.CSS {
	switch kind {
	case "stock_out":
		return "#c62b21"
	case "stock_low":
		return "#c2410c"
	case "not_found":
		return "#7c3aed"
	}
	return "#655d5a"
}

// RenderStock llena la plantilla.
func RenderStock(e StockEmail) (string, error) {
	var b bytes.Buffer
	if err := stock.Execute(&b, e); err != nil {
		return "", err
	}
	return b.String(), nil
}
