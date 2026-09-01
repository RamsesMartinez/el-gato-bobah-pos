# Quickstart: verificar bloqueo y cambio de operador por PIN

Cómo comprobar que la feature hace lo que dice, en el ambiente de pruebas
(`app-dev.elgatobobah.com`). Cada recorrido corresponde a una historia del spec.

## Antes de empezar

- Dos personas con PIN configurado. Hoy 6 de 8 usuarios activos lo tienen; si falta, se fija desde
  la ficha del empleado.
- La caja principal abierta, porque cobrar lo exige.

## US1 — Cobrar en la estación que esté libre

1. En el POS, deja que la tableta se bloquee (o bloquéala a mano).
2. Toca el nombre de la primera persona y teclea su PIN. Cobra una venta en efectivo.
3. Bloquea otra vez. Toca el nombre de la **segunda** persona, teclea su PIN, cobra otra venta.
4. Abre **Caja → arqueo**.

**Se espera**: la tabla "Cobrado por" separa las dos ventas, cada una bajo su persona.

**Falla si**: las dos aparecen bajo la misma persona. Significa que el token no cambió al desbloquear
y la atribución sigue siendo del primero.

## US2 — Se bloquea sola y no se pierde nada

1. Captura tres productos en una cuenta. **No cobres.**
2. No toques la tableta el tiempo configurado (por default 3 minutos).
3. Cuando se bloquee, desbloquea con **otra** persona.

**Se espera**: la cuenta sigue con sus tres productos. Las cuentas son del mostrador, no de quien las
capturó.

**Falla si**: la cuenta se vació. Es el fallo que enseña al operador a impedir el bloqueo, y con eso
se pierde toda la protección.

4. Con la tableta activa, tócala cada minuto durante cinco.

**Se espera**: no se bloquea. El reloj se reinicia con cada interacción.

## US3 — La sesión no dura más que un turno

Este no se puede esperar 8 horas en una prueba manual. Se verifica de dos maneras:

- **En pruebas**: baja `sessionHours` a 1 en los ajustes del negocio, entra, espera, y confirma que
  al volver pide usuario y contraseña —**no** el PIN.
- **Automatizado**: el test de integración adelanta el reloj más allá del plazo y verifica que el
  refresh se rechaza.

**Se espera**: credenciales completas, no PIN.

**Falla si**: basta el PIN. Significa que el desbloqueo está emitiendo sesión nueva y reiniciando el
reloj — el hallazgo 2 de la investigación.

### Y lo que hay que decidir en el despliegue

**Los refresh tokens ya emitidos traen 30 días por delante y bajar el ajuste NO los acorta.** El
plazo se fija al crear el token, no al usarlo. Si se dejan, la protección no aplica hasta que cada
tableta vuelva a entrar — que pueden ser semanas, y son justo las tabletas que llevan más tiempo
abiertas.

Cuántos son, antes de decidir:

```sql
select count(*) as vivos,
       count(*) filter (where expires_at > now() + interval '8 hours') as mas_alla_del_plazo
from refresh_tokens
where revoked_at is null and expires_at > now();
```

Dos caminos, y hay que elegir uno **a propósito**:

| | Qué pasa | Cuándo conviene |
| --- | --- | --- |
| **Revocarlos todos al desplegar** | Todas las tabletas piden usuario y contraseña una vez. La protección aplica desde el minuto uno | Es lo recomendado: son dos o tres tabletas y un solo login |
| **Dejarlos morir solos** | Nadie nota el despliegue, pero las sesiones viejas siguen vivas hasta un mes | Solo si el despliegue cae en hora pico y no se puede pedir a nadie que vuelva a entrar |

Para revocarlos, dentro del mismo despliegue:

```sql
update refresh_tokens set revoked_at = now() where revoked_at is null;
```

**No va como migración**: revocar sesiones es una operación de despliegue, no un cambio de esquema,
y meterla en una migración la haría correr también en cada ambiente de pruebas y en cada base
restaurada de un respaldo, cerrando sesiones que nadie pidió cerrar.

## US4 — El modo de solo-PIN

1. Con los PINs actuales (4 dígitos), intenta encender **Cobrar solo con PIN** en los ajustes.

**Se espera**: lo rechaza y **nombra** a las personas cuyo PIN no cumple.

**Falla si**: lo acepta. El negocio queda con dos personas capaces de desbloquearse mutuamente.

2. Ponles a todas un PIN de 6 dígitos distinto. Enciende el ajuste.

**Se espera**: la pantalla de bloqueo deja de pedir el nombre y solo pide el PIN.

3. Intenta ponerle a alguien un PIN igual al de otra persona.

**Se espera**: lo rechaza **sin decir de quién es**.

## Lo que se revisa en 1024×600

Con la tableta en su resolución real:

- La rejilla de nombres cabe **sin desplazarse** con las 6 personas que tienen PIN.
- Cada nombre y cada tecla del PIN miden al menos 44 px.
- El camino de "entrar con usuario y contraseña" se ve **sin buscarlo**: es la salida de quien olvidó
  su PIN, y esconderla la vuelve inútil.
