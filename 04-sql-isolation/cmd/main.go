package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if err := setup(ctx, pool); err != nil {
		log.Fatalf("setup: %v", err)
	}

	sep := "════════════════════════════════════════════════"

	fmt.Printf("\n%s\n  1. DIRTY READ\n%s\n", sep, sep)
	DirtyRead(ctx, pool)

	fmt.Printf("\n%s\n  2. NON-REPEATABLE READ\n%s\n", sep, sep)
	NonRepeatableRead(ctx, pool)

	fmt.Printf("\n%s\n  3. PHANTOM READ\n%s\n", sep, sep)
	PhantomRead(ctx, pool)

	fmt.Printf("\n%s\n  4. LOST UPDATE\n%s\n", sep, sep)
	LostUpdate(ctx, pool)

	fmt.Println()
}
