-- +goose Up
-- +goose StatementBegin
CREATE TRIGGER users_singleton_insert
BEFORE INSERT ON users
WHEN EXISTS (SELECT 1 FROM users)
BEGIN
    SELECT RAISE(ABORT, 'only one user is allowed');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER users_singleton_insert;
