package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LostUpdate shows two transactions each reading a value, computing a new
// value based on it, and writing back — the second write silently discards
// the first transaction's update.
func LostUpdate(ctx context.Context, pool *pgxpool.Pool) {
	resetAccounts(ctx, pool)

	var (
		wg          sync.WaitGroup
		t1Read      = make(chan struct{})
		t2Read      = make(chan struct{})
		t1Committed = make(chan struct{})
	)

	wg.Add(2)

	// T1: reads balance, waits for T2 to also read, then writes +500 and commits.
	go func() {
		defer wg.Done()

		tx, _ := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		defer tx.Rollback(ctx)

		var balance int
		tx.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = 1").Scan(&balance)
		fmt.Printf("[T1] READ balance = %d\n", balance)
		close(t1Read)

		<-t2Read // ensure T2 also has the stale read before T1 writes

		newBal := balance + 500
		tx.Exec(ctx, "UPDATE accounts SET balance = $1 WHERE id = 1", newBal)
		tx.Commit(ctx)
		fmt.Printf("[T1] UPDATE balance → %d  COMMIT  (+500)\n", newBal)
		close(t1Committed)
	}()

	// T2: reads balance (same stale value as T1), waits for T1 to commit,
	// then blindly writes +300 — overwriting T1's committed change.
	go func() {
		defer wg.Done()

		<-t1Read

		tx, _ := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		defer tx.Rollback(ctx)

		var balance int
		tx.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = 1").Scan(&balance)
		fmt.Printf("[T2] READ balance = %d  (same stale snapshot as T1)\n", balance)
		close(t2Read)

		<-t1Committed // T1 has already committed 1500; T2 still holds stale 1000

		newBal := balance + 300
		tx.Exec(ctx, "UPDATE accounts SET balance = $1 WHERE id = 1", newBal)
		tx.Commit(ctx)
		fmt.Printf("[T2] UPDATE balance → %d  COMMIT  (+300, based on stale read)\n", newBal)
	}()

	wg.Wait()

	var final int
	pool.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = 1").Scan(&final)

	fmt.Println()
	fmt.Printf("ANOMALY: Final balance = %d — expected 1800 (1000+500+300).\n", final)
	fmt.Println("         T1's +500 update was silently overwritten by T2.")
	fmt.Println("FIX: Use SELECT FOR UPDATE to lock the row before reading,")
	fmt.Println("     or use atomic UPDATE accounts SET balance = balance + ? WHERE id = 1,")
	fmt.Println("     or use SERIALIZABLE isolation (T2 would be aborted and retried).")
}
