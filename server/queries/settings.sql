-- name: GetBusinessSettings :one
select * from business_settings where id = true;

-- name: UpdateDeliveryFee :one
update business_settings
set delivery_fee = $1, updated_at = now(), updated_by = $2
where id = true
returning *;
