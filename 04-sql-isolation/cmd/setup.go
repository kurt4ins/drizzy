package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setup(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		DROP TABLE IF EXISTS accounts;
		DROP TABLE IF EXISTS orders;

		CREATE TABLE accounts (
			id      SERIAL PRIMARY KEY,
			name    TEXT NOT NULL,
			balance INTEGER NOT NULL
		);

		CREATE TABLE orders (
			id     SERIAL PRIMARY KEY,
			amount INTEGER NOT NULL
		);

		INSERT INTO accounts (name, balance) VALUES ('Goga', 1000);
		INSERT INTO orders (amount) VALUES (50), (150), (200);
	`)
	return err
}

func resetAccounts(ctx context.Context, pool *pgxpool.Pool) {
	pool.Exec(ctx, "UPDATE accounts SET balance = 1000 WHERE id = 1")
}

func resetOrders(ctx context.Context, pool *pgxpool.Pool) {
	pool.Exec(ctx, "DELETE FROM orders WHERE amount = 500")
}
