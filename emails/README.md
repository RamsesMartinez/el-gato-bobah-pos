# Plantillas de correo (spec 084)

Fuente MJML de los correos del servidor. **No se edita el HTML de `server/internal/mailtpl/templates/`
a mano**: sale de aquí con `cd web && bun run compilar-correos`, y CI falla si la salida versionada
no coincide con la fuente (`git diff --exit-code`). MJML vive solo en el build: el servidor no tiene
dependencias nuevas, embebe el HTML compilado con `go:embed` y lo llena con `html/template`.

- Los archivos que empiezan con `_` son piezas que se incluyen; cada otro `.mjml` es un correo.
- Lo variable va en marcas de Go (`{{.Title}}`) **dentro de `mj-text`**: MJML no sabe de ciclos ni
  condicionales, así que un `{{range}}` que envolviera secciones de MJML se rompería al compilar.
- Solo HTML (decisión del dueño, issue #291). Peso máximo 102 KB: Gmail recorta lo que pasa de ahí.
