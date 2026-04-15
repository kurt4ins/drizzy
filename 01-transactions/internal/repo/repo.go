package repo

import (
	"context"
	"database/sql"
	"fmt"
	"transactions/pkg/models"
)

type Repo struct {
	db *sql.DB
}

func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) CreateOrder(ctx context.Context, order models.Order, items []models.OrderItem) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var orderID int

	err = tx.QueryRowContext(ctx,
		`INSERT INTO orders (customer_id, order_date, total_amount)
         VALUES ($1, $2, 0)
         RETURNING order_id`,
		order.CustomerID, order.OrderDate,
	).Scan(&orderID)
	if err != nil {
		return fmt.Errorf("insert order: %w", err)
	}

	for _, item := range items {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO order_items (order_id, product_id, quantity, subtotal)
             VALUES ($1, $2, $3, $4)`,
			orderID, item.ProductID, item.Quantity, item.Subtotal,
		)
		if err != nil {
			return fmt.Errorf("insert order item: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE orders
         SET total_amount = (
             SELECT COALESCE(SUM(subtotal), 0)
             FROM order_items
             WHERE order_id = $1
         )
         WHERE order_id = $1`,
		orderID,
	)
	if err != nil {

		return fmt.Errorf("update total: %w", err)
	}

	return tx.Commit()
}

func (r *Repo) UpdateEmail(ctx context.Context, customerID int, newEmail string) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE customers SET email = $1 WHERE customer_id = $2`,
		newEmail, customerID,
	)
	if err != nil {
		return fmt.Errorf("update email: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("customer %d not found", customerID)
	}

	return tx.Commit()
}

func (r *Repo) CreateProduct(ctx context.Context, product models.Product) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var productID int
	err = tx.QueryRowContext(ctx,
		`INSERT INTO products (product_name, price)
         VALUES ($1, $2)
         RETURNING product_id`,
		product.ProductName, product.Price,
	).Scan(&productID)
	if err != nil {
		return fmt.Errorf("insert product: %w", err)
	}

	return tx.Commit()
}
