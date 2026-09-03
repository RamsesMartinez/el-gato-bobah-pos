# Contratos: bloqueo y cambio de operador por PIN

Solo lo que cambia o nace. Todo bajo `/api/v1`, autenticado salvo donde se diga.

## Cambia: `POST /auth/pin-switch`

Ya existe y **cambia de semántica**, que es el punto delicado de esta feature.

**Hoy** emite una sesión nueva: un refresh token con el plazo completo por delante. Eso hace que
cada desbloqueo reinicie el reloj del turno, y por eso hoy hay usuarios con 4 tokens vivos.

**Pasa a** cambiar quién está activo **conservando el vencimiento** de la sesión del dispositivo, y
revocando el refresh de quien estaba.

```
POST /auth/pin-switch
{ "userId": 12, "pin": "4827" }        // userId ausente cuando el negocio usa solo-PIN

200 { "accessToken": "...", "user": { ... } }
401 credenciales inválidas             // mismo cuerpo si falla el id o el PIN: no se distingue
429 demasiados intentos                // lockout per-usuario, ya existente
```

- **`userId` opcional**: con `pin_only_unlock` encendido, el servidor deduce la persona por el PIN.
  Con el ajuste apagado, un `userId` ausente se rechaza — no se cae al modo permisivo en silencio.
- **El refresh NO se renueva con plazo nuevo.** El `expires_at` de la sesión del dispositivo se
  conserva. Es la regla que hace que `session_hours` signifique algo.
- **La respuesta no dice si el PIN existía.** Igualar la latencia con el bcrypt de descarte ya está
  implementado; el contrato lo fija.

## Nace: `GET /auth/unlock-options`

Quiénes pueden desbloquear esta estación. Lo pide la pantalla de bloqueo para pintar la rejilla.

```
GET /auth/unlock-options
200 { "pinOnly": false, "users": [ { "id": 12, "name": "Ana" }, ... ] }
```

- **Devuelve solo id y nombre.** Nada de correo, rol ni teléfono: es una lista que se pinta en un
  mostrador a la vista del público.
- Solo personas **activas y con PIN configurado**. Quien no tiene PIN no aparece, porque tocarlo no
  la dejaría entrar (entra con sus credenciales completas, FR-012).
- Con `pinOnly` en `true`, `users` viaja **vacío**: la pantalla no debe listar a nadie, o el modo de
  solo-PIN perdería su única ventaja y expondría la plantilla sin necesidad.

## Cambia: `GET /business-settings`

Gana los tres ajustes de identificación, junto a los que ya trae.

```
200 {
  ...,
  "pinOnlyUnlock": false,
  "lockAfterSeconds": 180,
  "sessionHours": 8
}
```

Los lee el POS al arrancar, con los demás ajustes. La pantalla de bloqueo los necesita **antes** de
saber qué pedir.

## Cambia: `PUT /business-settings`

Acepta los tres. Solo admin/gerente, como el resto de los ajustes.

```
PUT /business-settings
{ "pinOnlyUnlock": true }

200 ajustes actualizados
422 { "error": "...", "usuarios": ["Luis", "Sofía"] }   // PINs que no cumplen
```

**Encender `pinOnlyUnlock` es una operación con compuerta**: el servidor verifica que todas las
personas activas tengan PIN de al menos 6 dígitos y distinto al de las demás. Si no, rechaza
**nombrando a quiénes** hay que corregir. Un 422 sin la lista mandaría al dueño a revisar ocho fichas
una por una.

Apagarlo no tiene compuerta: volver al modo seguro siempre se puede.

## Cambia: `POST /me/pin` y el alta/edición de empleado

Ganan la validación de unicidad y de largo **cuando el negocio tiene solo-PIN encendido**.

```
POST /me/pin
{ "pin": "482715" }

422 el PIN debe tener al menos 6 dígitos       // solo con pinOnlyUnlock
422 ese PIN ya lo usa otra persona             // solo con pinOnlyUnlock
```

El segundo mensaje **no dice quién**. Decirlo convertiría el formulario en un oráculo para averiguar
el PIN de un compañero probando.
