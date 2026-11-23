-- +goose Up
-- ---------------------------------------------------------------------------------------------
-- #335: una escritura del MCP que falló sin confirmar nada en la base deja su llave en «failed» con
-- el error, en vez de borrarla (rechazo) o dejarla en «unknown» (error interno). Así quien la revisa
-- sabe con certeza que reintentar es seguro; «unknown» queda solo para lo que pudo haberse aplicado.
--
-- Un intento fallido se puede retomar con la misma llave y argumentos corregidos (como cuando la
-- llave se borraba): por eso el rol de la API puede reescribir la huella y la herramienta.
--
-- Solo ensancha los checks: la API anterior, durante el traslape, nunca escribe «failed», y si lee
-- uno lo trata como «unknown» (pide revisar con una lectura), que es lo seguro.
-- ---------------------------------------------------------------------------------------------
-- Un solo ALTER: una sola toma del candado y ningún momento sin check. Con lock_timeout, si una
-- escritura del MCP retiene la tabla, la migración falla limpio en vez de encolar a todas detrás.
set lock_timeout = '3s';
alter table mcp_idempotency_keys
  drop constraint mcp_idempotency_keys_status,
  drop constraint mcp_idempotency_keys_result,
  add constraint mcp_idempotency_keys_status check (status in ('pending', 'done', 'unknown', 'failed')),
  add constraint mcp_idempotency_keys_result check ((status in ('done', 'failed')) = (result is not null));
reset lock_timeout;
grant update (args_hash, tool) on mcp_idempotency_keys to gatobobah_app;

-- +goose Down
-- Borrar una llave «failed» es lo que hacía la API antes con un rechazo: nada se aplicó y la llave
-- queda libre para reintentar. No se pierde un hecho.
revoke update (args_hash, tool) on mcp_idempotency_keys from gatobobah_app;
delete from mcp_idempotency_keys where status = 'failed';
alter table mcp_idempotency_keys drop constraint mcp_idempotency_keys_result;
alter table mcp_idempotency_keys add constraint mcp_idempotency_keys_result
  check ((status = 'done') = (result is not null));
alter table mcp_idempotency_keys drop constraint mcp_idempotency_keys_status;
alter table mcp_idempotency_keys add constraint mcp_idempotency_keys_status
  check (status in ('pending', 'done', 'unknown'));
