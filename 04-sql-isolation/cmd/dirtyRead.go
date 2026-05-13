package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DirtyRead attempts to read uncommitted data from another transaction.
// PostgreSQL maps READ UNCOMMITTED → READ COMMITTED internally, so dirty reads
// cannot occur — this demo proves that prevention.
func DirtyRead(ctx context.Context, pool *pgxpool.Pool) {
	resetAccounts(ctx, pool)

	var (
		wg      sync.WaitGroup
		t1Wrote = make(chan struct{})
		t2Done  = make(chan struct{})
	)

	var t2Balance int

	wg.Add(2)

	// T1: update balance to 9999 but do NOT commit, then roll back.
	go func() {
		defer wg.Done()

		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			fmt.Printf("[T1] begin: %v\n", err)
			close(t1Wrote)
			return
		}

		if _, err := tx.Exec(ctx, "UPDATE accounts SET balance = 9999 WHERE id = 1"); err != nil {
			fmt.Printf("[T1] update: %v\n", err)
			tx.Rollback(ctx)
			close(t1Wrote)
			return
		}
		fmt.Println("[T1] UPDATE balance → 9999  (not committed yet)")

		close(t1Wrote) // signal T2 to read
		<-t2Done       // wait for T2 to finish

		tx.Rollback(ctx)
		fmt.Println("[T1] ROLLBACK  (the 9999 never existed)")
	}()

	// T2: try to read T1's uncommitted data using READ UNCOMMITTED.
	go func() {
		defer wg.Done()
		defer close(t2Done)

		<-t1Wrote

		tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadUncommitted})
		if err != nil {
			fmt.Printf("[T2] begin: %v\n", err)
			return
		}
		defer tx.Rollback(ctx)

		if err := tx.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = 1").Scan(&t2Balance); err != nil {
			fmt.Printf("[T2] read: %v\n", err)
			return
		}
		fmt.Printf("[T2] READ balance = %d  (isolation: READ UNCOMMITTED)\n", t2Balance)
	}()

	wg.Wait()

	fmt.Println()
	if t2Balance == 9999 {
		fmt.Println("ANOMALY: Dirty read occurred — T2 saw T1's uncommitted data!")
	} else {
		fmt.Printf("PREVENTED: T2 read %d (committed value), not 9999 (T1's dirty write).\n", t2Balance)
		fmt.Println("           PostgreSQL silently upgrades READ UNCOMMITTED → READ COMMITTED,")
		fmt.Println("           making dirty reads impossible.")
	}
	fmt.Println("FIX: Use READ COMMITTED or higher (PostgreSQL default).")
}
