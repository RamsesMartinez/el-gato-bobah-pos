-- +goose Up
-- Revoca TODAS las sesiones vivas: las que se emitieron antes de que el turno durara un turno.
--
-- Hasta 0050 el refresh se emitía con 30 días fijos. Esas credenciales siguen en la base con su
-- vencimiento viejo, y el arreglo de la rotación las CONSERVA a propósito —rotar no debe correr el
-- fin del turno hacia adelante—, así que sin esto una tableta que ya estaba dentro se quedaría con
-- un mes de sesión y el límite de horas no aplicaría a nadie hasta que cada una volviera a entrar.
-- En producción hay una con cuatro sesiones vivas, la más vieja de tres días antes.
--
-- El costo es que todos vuelven a entrar UNA vez, con usuario y contraseña. Se hace en la
-- migración y no a mano porque a mano se olvida, y lo que se olvida aquí es la feature entera.
--
-- No se borran las filas: el histórico de sesiones es lo que deja ver un reuso más adelante.
update refresh_tokens set revoked_at = now() where revoked_at is null;

-- +goose Down
-- Sin vuelta atrás, y no es un descuido.
--
-- Revivir una credencial revocada es lo contrario de lo que hace el resto del sistema: la
-- detección de reuso trata un refresh revocado que reaparece como robo. Deshacer esto sería
-- reactivar en bloque justo lo que esa detección persigue, y además no se puede distinguir de las
-- que ya estaban revocadas por un logout o por un relevo. El rollback es no hacer nada: quien
-- vuelva a entrar tendrá su sesión nueva.
select 1;
