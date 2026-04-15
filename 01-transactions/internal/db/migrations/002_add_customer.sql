-- +goose Up
INSERT INTO customers (first_name, last_name, email)
VALUES ('Goga', 'Goga', 'goga@gmail.com');

-- +goose Down
DELETE FROM customers WHERE email = 'goga@gmail.com';