-- name: ListProjectsByAccount :many
SELECT * FROM project WHERE account_id = $1 ORDER BY name ASC;

-- name: GetProjectForAccount :one
SELECT * FROM project WHERE id = $1 AND account_id = $2 LIMIT 1;

-- name: InsertProject :one
INSERT INTO project (account_id, name, paths) VALUES ($1, $2, $3) RETURNING *;

-- name: UpdateProject :one
UPDATE project SET name = $2, paths = $3, updated_at = now()
WHERE id = $1 AND account_id = $4
RETURNING *;

-- name: DeleteProject :execrows
DELETE FROM project WHERE id = $1 AND account_id = $2;

-- name: ListProjectPathsByAccount :many
-- Per-account paths fetch. The router lazily loads one account's bucket on
-- first match, instead of eagerly pulling every user's projects into memory.
SELECT id AS project_id, jsonb_array_elements_text(paths) AS path
FROM project
WHERE account_id = $1 AND jsonb_array_length(paths) > 0;

-- name: UpsertProjectSeen :exec
UPDATE project
SET first_seen_at = LEAST(COALESCE(first_seen_at, sqlc.arg('seen_at')::timestamp), sqlc.arg('seen_at')::timestamp),
    last_seen_at  = GREATEST(COALESCE(last_seen_at,  sqlc.arg('seen_at')::timestamp), sqlc.arg('seen_at')::timestamp),
    updated_at    = now()
WHERE id = $1;
