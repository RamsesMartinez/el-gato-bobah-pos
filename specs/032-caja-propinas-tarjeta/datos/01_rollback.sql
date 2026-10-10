-- ============================================================================
-- Rollback de 01_propinas_de_gastos.sql: devuelve los movimientos y los gastos a como estaban,
-- desde los respaldos _bak_032_*. NO SE HA CORRIDO EN NINGÚN AMBIENTE.
-- ============================================================================
begin;

update register_cash_movements m
   set kind = b.kind, expense_id = b.expense_id, concept = b.concept,
       recipient_user_id = null, recipient_name = null
  from _bak_032_movimientos b
 where m.id = b.id;

update expenses e
   set status = b.status, cancelled_at = b.cancelled_at, cancel_reason = b.cancel_reason, cancelled_by = b.cancelled_by
  from _bak_032_gastos b
 where e.id = b.id;

-- Verificación: nada quedó como propina de los respaldados.
do $$
begin
  if exists (select 1 from register_cash_movements m join _bak_032_movimientos b on b.id = m.id where m.kind = 'propina') then
    raise exception 'quedaron movimientos como propina';
  end if;
end $$;

-- commit;   -- con el visto bueno del dueño
rollback;
