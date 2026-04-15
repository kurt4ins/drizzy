package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"transactions/internal/db"
	"transactions/internal/repo"
	"transactions/internal/service"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	database, err := db.Open(dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer database.Close()

	r := repo.NewRepo(database)
	svc := service.NewService(r)
	ctx := context.Background()

	customerID := 1

	// 3) add products
	fmt.Println("\n3) Add products")

	if err := svc.CreateProduct(ctx, "Laptop", 999.99); err != nil {
		log.Fatalf("create product: %v", err)
	}
	fmt.Println("Created product: Laptop $999.99")

	if err := svc.CreateProduct(ctx, "Mouse", 29.99); err != nil {
		log.Fatalf("create product: %v", err)
	}
	fmt.Println("Created product: Mouse $29.99")

	// 1) place an order
	fmt.Println("\n1) Place an order")

	items := []service.OrderItemInput{
		{ProductID: 1, Quantity: 1, Price: 999.99},
		{ProductID: 2, Quantity: 2, Price: 29.99},
	}

	if err := svc.CreateOrder(ctx, customerID, items); err != nil {
		log.Fatalf("create order: %v", err)
	}
	fmt.Printf("Order placed for customer id=%d (1x Laptop, 2x Mouse)\n", customerID)

	// 2) update customer email
	fmt.Println("\n2) Update customer email")

	if err := svc.UpdateEmail(ctx, customerID, "goga.new@gmail.com"); err != nil {
		log.Fatalf("update email: %v", err)
	}
	fmt.Printf("Updated email for customer id=%d\n", customerID)

	fmt.Println("\nAll scenarios completed successfully")
}
