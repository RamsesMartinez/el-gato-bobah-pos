-- name: AdminListProducts :many
select p.id, p.name, p.price, p.current_cost, p.type, p.is_active, p.is_favorite,
       p.available_from, p.available_until, c.name as category
from products p
join categories c on c.id = p.category_id
order by p.name;

-- name: AdminUpdateProduct :exec
update products
set name = $2, price = $3, is_favorite = $4, is_active = $5,
    available_from = $6, available_until = $7, updated_at = now()
where id = $1;
