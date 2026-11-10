-- +goose Up
-- Spec 073: dos alcances nuevos para la puerta para agentes, compras (mercancía al almacén) y pedidos
-- (cancelar). Ampliar la lista es retrocompatible: la API anterior no escribe los nuevos, y al leerlos
-- los trata como desconocidos sin usarlos. La tabla tiene pocos renglones: el candado dura nada.
alter table api_tokens drop constraint api_tokens_scopes,
  add constraint api_tokens_scopes check (
    scopes <@ array['read', 'write:expenses', 'write:cash', 'write:inventory', 'write:catalog',
                    'write:purchases', 'write:orders']::text[]
    and 'read' = any (scopes)
    and array_position(scopes, null) is null
    and cardinality(scopes) between 1 and 7);

-- +goose Down
-- Se niega si algún token ya tiene un alcance nuevo: quitárselo en silencio cambiaría lo que su dueño
-- creó sin que nadie lo supiera.
-- +goose StatementBegin
do $$
begin
  if exists (select 1 from api_tokens where 'write:purchases' = any (scopes) or 'write:orders' = any (scopes)) then
    raise exception 'hay tokens con el alcance de compras o de pedidos: revócalos o quítaselos antes de bajar';
  end if;
end;
$$;
-- +goose StatementEnd
alter table api_tokens drop constraint api_tokens_scopes,
  add constraint api_tokens_scopes check (
    scopes <@ array['read', 'write:expenses', 'write:cash', 'write:inventory', 'write:catalog']::text[]
    and 'read' = any (scopes)
    and array_position(scopes, null) is null
    and cardinality(scopes) between 1 and 5);
