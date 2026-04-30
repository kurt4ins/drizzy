package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DB struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*DB, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &DB{pool: pool}, nil
}

func (d *DB) Close() { d.pool.Close() }

func (d *DB) Init(ctx context.Context) error {
	_, err := d.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS kv (key TEXT PRIMARY KEY, value TEXT NOT NULL)`)
	return err
}

func (d *DB) Reset(ctx context.Context) error {
	_, err := d.pool.Exec(ctx, `TRUNCATE TABLE kv`)
	return err
}

func (d *DB) Seed(ctx context.Context, n int) error {
	b := &pgx.Batch{}
	for i := 1; i <= n; i++ {
		b.Queue(
			`INSERT INTO kv(key, value) VALUES($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
			fmt.Sprintf("key:%d", i), fmt.Sprintf("value:%d", i),
		)
	}
	br := d.pool.SendBatch(ctx, b)
	for i := 0; i < n; i++ {
		if _, err := br.Exec(); err != nil {
			br.Close()
			return err
		}
	}
	return br.Close()
}

func (d *DB) Get(ctx context.Context, key string) (string, bool, error) {
	var val string
	err := d.pool.QueryRow(ctx, `SELECT value FROM kv WHERE key = $1`, key).Scan(&val)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return val, true, nil
}

func (d *DB) Set(ctx context.Context, key, value string) error {
	_, err := d.pool.Exec(ctx,
		`INSERT INTO kv(key, value) VALUES($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		key, value,
	)
	return err
}
