# Contrato de UI — 089

## Ficha / renglón de cuenta

- Nombre de la cuenta (folio o cliente).
- **Lo que le falta**, con palabras de mostrador (research R-1). Color: rojo si falta cobrar, azul si
  solo falta entregar, gris si no se ha enviado.
- Acciones (solo pedidos, nunca cuentas en captura): **Entregar** si `entregar`; **Cobrar** si
  `cobrar`. Cada una ≥ 44 px. En la columna y en la hoja «Todas las cuentas» el botón aparece solo cuando le falta UNA cosa (revisión de código: «Entregar» pegado a «Cobrar» invitaba a entregar sin cobrar); con las dos se abre la cuenta y su ticket ofrece «Entregar» en la franja «Falta …» y «Cobrar» en el pie.
- `aria-label`: «{nombre} · le falta {texto}».

## Endpoints usados (existentes)

- `GET /pos/accounts` — la fila.
- `POST /orders/:id/deliver` — entregar todo.
- `GET /orders/:id` + hoja de cobro — cobrar.
