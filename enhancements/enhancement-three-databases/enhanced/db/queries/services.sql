-- name: RegisterService :one
INSERT INTO services (name, base_url, prefixes)
VALUES ($1, $2, $3)
ON CONFLICT (name) DO UPDATE SET
    base_url = EXCLUDED.base_url,
    prefixes = EXCLUDED.prefixes,
    updated_at = NOW()
RETURNING *;

-- name: GetService :one
SELECT * FROM services WHERE name = $1;

-- name: GetAllServices :many
SELECT * FROM services ORDER BY name;

-- name: DeleteService :exec
DELETE FROM services WHERE name = $1;

-- name: GetServicesByPrefix :many
SELECT * FROM services WHERE $1 = ANY(prefixes) ORDER BY name;