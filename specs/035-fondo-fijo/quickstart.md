# Quickstart: validar el fondo fijo

Prerrequisitos: `make start` (API como `gatobobah_app`), usuario gerente.

1. Ajustes › Caja: capturar fondo fijo $1,000 en la caja principal. Recargar → sigue $1,000.
2. Abrir turno, vender $4,300 en efectivo, ir a cerrar: el campo «Se queda de fondo» trae $1,000.
3. Contar $5,300, elegir quién se lleva el retiro, cerrar → corte muestra retiro $4,300 con esa persona, diferencia del cajón $0.
4. Siguiente turno: cerrar cambiando el fondo a $800 → el corte marca «distinto del fijo ($1,000)»; Ajustes › Caja sigue en $1,000.
5. Un cierre anterior a la subida se ve con su fondo tecleado y sin retiro.

Pruebas automáticas:

```bash
cd server && go build ./... && go test ./...      # dominio + integración (appRoleStore)
cd ../web && bun run vitest run && bun run lint
```

Casos de integración clave: retiro no altera la diferencia (SC-004), doble cierre = un retiro, `inTheThreeCases` sobre el fondo fijo, cierres previos intactos tras migrar con dos empresas.
