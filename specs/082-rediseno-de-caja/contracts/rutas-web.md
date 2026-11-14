# Contrato de rutas del front (fase 2)

| Ruta | Muestra | Permiso |
| --- | --- | --- |
| `/caja` | Cajas (la principal seleccionada) | `cash.operate` |
| `/caja/:cajaId` | Esa caja: su turno (Operar) | `cash.operate` |
| `/caja/:cajaId/cerrar` | Asistente de cierre (fase 4) | `cash.operate` |
| `/caja/historico` | Histórico | `cash.operate` |
| `/caja/historico/:corteId` | Histórico con ese corte abierto | `cash.operate` |
| `/caja/administrar` | Cajas, conceptos, terminales, modo de arqueo, correos y tolerancia | `cash.configure` o `business.configure` (cada bloque el suyo) |

- Un `cajaId` o `corteId` inexistente o de otra empresa muestra «No encontrado» y vuelve a la lista.
- El bloque de ajustes de caja de `/negocio` se sustituye por un enlace a `/caja/administrar`.
- Los permisos se agregan a `app/routes.ts` (espejo de UX; la barrera sigue en el servidor).
