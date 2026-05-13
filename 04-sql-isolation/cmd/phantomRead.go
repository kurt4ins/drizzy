package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PhantomRead shows a range query returning a different number of rows within
// a single transaction because another transaction inserted a matching row
// between the two scans.
func PhantomRead(ctx context.Context, pool *pgxpool.Pool) {
	resetOrders(ctx, pool)

	var (
		wg          sync.WaitGroup
		t1FirstScan = make(chan struct{})
		t2Committed = make(chan struct{})
	)

	var count1, count2 int

	wg.Add(2)

	// T1: counts matching rows twice; a new matching row is inserted in between.
	go func() {
		defer wg.Done()

		tx, _ := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		defer tx.Rollback(ctx)

		tx.QueryRow(ctx, "SELECT COUNT(*) FROM orders WHERE amount > 100").Scan(&count1)
		fmt.Printf("[T1] 1st COUNT(amount > 100) = %d\n", count1)

		close(t1FirstScan)
		<-t2Committed

		tx.QueryRow(ctx, "SELECT COUNT(*) FROM orders WHERE amount > 100").Scan(&count2)
		fmt.Printf("[T1] 2nd COUNT(amount > 100) = %d\n", count2)
	}()

	// T2: inserts a row that matches T1's range predicate, then commits.
	go func() {
		defer wg.Done()

		<-t1FirstScan

		tx, _ := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		tx.Exec(ctx, "INSERT INTO orders (amount) VALUES (500)")
		tx.Commit(ctx)
		fmt.Println("[T2] INSERT orders(amount=500)  COMMIT")

		close(t2Committed)
	}()

	wg.Wait()

	fmt.Println()
	if count2 > count1 {
		fmt.Printf("ANOMALY: Phantom read — T1 counted %d rows, then %d rows for the same predicate!\n", count1, count2)
	} else {
		fmt.Printf("PREVENTED: both counts returned %d.\n", count1)
	}
	fmt.Println("FIX: Use SERIALIZABLE isolation (or REPEATABLE READ in PostgreSQL,")
	fmt.Println("     whose snapshot implementation also eliminates phantom reads).")
}
