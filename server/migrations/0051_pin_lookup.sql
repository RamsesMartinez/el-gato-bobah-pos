-- +goose Up
-- Huella determinista del PIN, para poder compararlo por igualdad sin guardarlo de forma reversible.
--
-- `pin_hash` es bcrypt y saliniza, así que de él NO se puede saber si dos personas tienen el mismo
-- PIN ni de quién es uno dado. Las dos cosas son exactamente lo que el modo de solo-PIN necesita:
-- ahí el PIN deja de PROBAR quién eres y pasa a DECIRLO.
--
-- Esta columna guarda un HMAC del PIN con un secreto del servidor. Determinista —dos PINs iguales
-- dan el mismo valor, y por eso el índice de abajo puede impedirlos— pero inútil sin el secreto:
-- quien se lleve la base no puede recorrer el millón de combinaciones de seis dígitos sin él.
--
-- Sigue siendo bcrypt el que VERIFICA al desbloquear. Este valor solo sirve para comparar y para
-- encontrar; nunca para autenticar por sí solo.
alter table users add column pin_lookup text;

-- Dos personas de la misma empresa no pueden compartir PIN. Lo impide la base y no solo el
-- servicio: entre validar y escribir cabe otra transacción poniendo el mismo, y el resultado sería
-- justo el estado que vuelve inútil todo el modo.
--
-- Parcial: quien no tiene PIN, o lo tiene de antes de esta columna, no choca con nadie.
create unique index users_pin_lookup_unico
  on users (company_id, pin_lookup)
  where pin_lookup is not null;

-- +goose Down
drop index if exists users_pin_lookup_unico;
alter table users drop column pin_lookup;
