package models

import "time"

type Customer struct {
	CustomerID int
	FirstName  string
	LastName   string
	Email      string
}

type Product struct {
	ProductID   int
	ProductName string
	Price       float64
}

type Order struct {
	OrderID     int
	CustomerID  int
	OrderDate   time.Time
	TotalAmount float64
}

type OrderItem struct {
	OrderItemID int
	OrderID     int
	ProductID   int
	Quantity    int
	Subtotal    float64
}
