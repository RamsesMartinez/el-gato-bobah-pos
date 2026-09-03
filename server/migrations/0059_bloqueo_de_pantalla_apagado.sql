-- +goose Up
-- El bloqueo de pantalla pasa a nacer APAGADO, y se apaga en los negocios que ya existen.
--
-- Nacía en 180 segundos. En un local donde la tableta vive a la vista del mostrador, bloquearse
-- cada tres minutos son dos toques y un PIN a media venta a cambio de nada: la barrera que de
-- verdad protege es la caducidad de la SESIÓN, que la aplica el servidor y que esta migración no
-- toca.
--
-- Cero ya era un valor válido —"no se bloquea"— y la pantalla ya lo decía en letra chica. Lo que
-- cambia es cuál de los dos es el default, y que Ajustes lo exponga como interruptor en vez de
-- pedir que alguien adivine escribir un cero.
alter table business_settings alter column lock_after_seconds set default 0;

-- Corre como owner, así que RLS no aplica y alcanza a TODAS las empresas, no solo a la del GUC.
-- Es lo buscado: el dueño lo pidió apagado y ninguna empresa existente puede "volver a nacer".
-- Encenderlo de vuelta es un interruptor en Ajustes.
update business_settings set lock_after_seconds = 0 where lock_after_seconds <> 0;

-- +goose Down
-- Devuelve el default viejo. NO restaura el valor que cada negocio tenía antes: esta migración no
-- lo guardó, y reponerle 180 a quien lo tenía en 600 sería inventar un ajuste que nadie eligió.
alter table business_settings alter column lock_after_seconds set default 180;
