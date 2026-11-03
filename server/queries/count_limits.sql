-- Límites del conteo por empresa (spec 044). RLS acota a la empresa de la sesión.

-- name: GetCountLimits :one
select * from count_limits;

-- name: UpsertCountLimits :exec
-- company_id sale del default de la columna (la empresa de la sesión).
insert into count_limits (max_mass, max_volume, max_length, max_piece,
                          confirm_mass, confirm_volume, confirm_length, confirm_piece)
values (@max_mass, @max_volume, @max_length, @max_piece,
        @confirm_mass, @confirm_volume, @confirm_length, @confirm_piece)
on conflict (company_id) do update set
  max_mass = excluded.max_mass, max_volume = excluded.max_volume,
  max_length = excluded.max_length, max_piece = excluded.max_piece,
  confirm_mass = excluded.confirm_mass, confirm_volume = excluded.confirm_volume,
  confirm_length = excluded.confirm_length, confirm_piece = excluded.confirm_piece,
  updated_at = now();
