# Contratos: la consola de plataforma

Todo cuelga de `/api/v1/platform` y **nada** se agrega al grupo del negocio.

## `POST /api/v1/platform/auth/login`

```
{ "username": "...", "password": "..." }
→ 200 { "accessToken": "...", "operator": { "name": "..." } }
```

- Firma con **`PLATFORM_JWT_SECRET`**, no con el del negocio. Un token emitido aquí **no valida**
  contra la API del negocio, y al revés tampoco: no es una comprobación, es que las firmas no
  coinciden.
- Busca en `platform_operators`. Una credencial de negocio no está ahí, así que se rechaza sin
  ninguna lógica especial (FR-004).
- Un operador con `is_active = false` se rechaza **igual que uno inexistente**: misma respuesta,
  misma latencia.
- Limitado por IP y por cuenta con el limitador que ya existe, y con el mismo lockout.
- Deja evento de seguridad en los dos casos (FR-012), con el usuario intentado y **nunca** la
  contraseña.

## `POST /api/v1/auth/login` — lo que cambia

Nada en el código: una credencial de plataforma simplemente **no está en `users`**. Lo que sí hay
que garantizar y probar es que la respuesta y **la latencia** sean las de una cuenta inexistente
(FR-003), que es lo que ya hace `auth.CheckDummySecret` para el resto de los casos.

## `GET /api/v1/platform/companies`

```
→ 200 { "items": [ { "id": 2, "slug": "gatobobah", "name": "El Gato Bobah",
                     "createdAt": "2026-08-29T..." } ],
        "schema": { "version": 67 } }
```

- Requiere sesión de plataforma. Con una sesión de negocio: **401**, no 403 — no se confirma que la
  ruta exista para quien no debe verla.
- **Sin una sola cifra de dinero** (FR-008).
- `schema` viaja **una vez, para la instalación**, y no como columna por empresa: la versión de
  migración es global a la base y repetirla por renglón aparentaría informar algo que no es por
  cliente (ver [data-model.md](../data-model.md)).
- «Última actividad» **no viaja en esta primera versión**: leerla exigiría permiso sobre tablas de
  operación, que es justo lo que FR-005 prohíbe. Llega con la spec 017, que escribe ese dato del
  lado de plataforma.

## Lo que NO existe, a propósito

- Ninguna ruta que **escriba** nada de una empresa. El rol de base tampoco podría (FR-009).
- Ninguna ruta de plataforma dentro del grupo del negocio.
- Ningún endpoint que devuelva pedidos, ventas, cortes, gastos, productos ni usuarios de un cliente.
  No es que no se hayan escrito: es que la conexión que atiende estas rutas **no tiene permiso** para
  leerlos.

## Configuración nueva

| Variable | Qué | Al arranque |
|---|---|---|
| `PLATFORM_JWT_SECRET` | Firma de la consola | La API **no arranca** si falta, si es débil o **si es igual a `JWT_SECRET`** |
| `PLATFORM_DATABASE_URL` | Conexión como `gatobobah_platform` | En producción es obligatoria; sin ella el aislamiento no existe |

Las dos se validan en `config.Validate`, junto a las que ya están. Dos secretos iguales son un
secreto, y el fallo sería silencioso.

**Y una comprobación funcional al arranque, no solo de configuración.** El binario ya trae
`assertRLSEnforced`, que aborta si el rol de servicio resulta ser superusuario o tiene `bypassrls`.
La consola necesita su gemela: comprobar que el rol de plataforma **no** es superusuario y que un
`select` canario sobre `orders` falla con `42501`, antes de servir tráfico.

Sin ella, un `PLATFORM_DATABASE_URL` copiado de `DATABASE_URL` por descuido serviría con bypass
total y nadie lo notaría hasta una auditoría a mano. Validar la forma de la cadena no basta: hay que
preguntarle a Postgres qué puede hacer de verdad ese rol.
