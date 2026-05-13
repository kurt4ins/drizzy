package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NonRepeatableRead shows a transaction reading different values for the same
// row within a single transaction because another transaction committed a change
// in between the two reads.
func NonRepeatableRead(ctx context.Context, pool *pgxpool.Pool) {
	resetAccounts(ctx, pool)

	var (
		wg          sync.WaitGroup
		t1FirstRead = make(chan struct{})
		t2Committed = make(chan struct{})
	)

	var read1, read2 int

	wg.Add(2)

	// T1: reads balance twice; an update from T2 lands between the reads.
	go func() {
		defer wg.Done()

		tx, _ := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		defer tx.Rollback(ctx)

		tx.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = 1").Scan(&read1)
		fmt.Printf("[T1] 1st READ balance = %d\n", read1)

		close(t1FirstRead) // let T2 run
		<-t2Committed      // wait for T2 to commit

		tx.QueryRow(ctx, "SELECT balance FROM accounts WHERE id = 1").Scan(&read2)
		fmt.Printf("[T1] 2nd READ balance = %d\n", read2)
	}()

	// T2: updates and commits between T1's two reads.
	go func() {
		defer wg.Done()

		<-t1FirstRead

		tx, _ := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		tx.Exec(ctx, "UPDATE accounts SET balance = 2000 WHERE id = 1")
		tx.Commit(ctx)
		fmt.Println("[T2] UPDATE balance → 2000  COMMIT")

		close(t2Committed)
	}()

	wg.Wait()

	fmt.Println()
	if read1 != read2 {
		fmt.Printf("ANOMALY: Non-repeatable read — T1 saw %d then %d for the same row within one transaction!\n", read1, read2)
	} else {
		fmt.Printf("PREVENTED: both reads returned %d.\n", read1)
	}
	fmt.Println("FIX: Use REPEATABLE READ — T1 operates on a consistent snapshot of the DB.")
}
