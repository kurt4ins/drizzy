package service

import (
	"context"
	"fmt"
	"net/mail"
	"time"
	"transactions/pkg/models"
)

type Repository interface {
	CreateOrder(ctx context.Context, order models.Order, items []models.OrderItem) error
	UpdateEmail(ctx context.Context, customerID int, newEmail string) error
	CreateProduct(ctx context.Context, product models.Product) error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

type OrderItemInput struct {
	ProductID int
	Quantity  int
	Price     float64
}

func (s *Service) CreateOrder(ctx context.Context, customerID int, inputs []OrderItemInput) error {
	if len(inputs) == 0 {
		return fmt.Errorf("order must have at least one item")
	}

	order := models.Order{
		CustomerID: customerID,
		OrderDate:  time.Now(),
	}

	items := make([]models.OrderItem, len(inputs))
	for i, input := range inputs {
		items[i] = models.OrderItem{
			ProductID: input.ProductID,
			Quantity:  input.Quantity,
			Subtotal:  float64(input.Quantity) * input.Price,
		}
	}

	return s.repo.CreateOrder(ctx, order, items)
}

func (s *Service) UpdateEmail(ctx context.Context, customerID int, newEmail string) error {
	if newEmail == "" {
		return fmt.Errorf("email cannot be empty")
	} else if _, err := mail.ParseAddress(newEmail); err != nil {
		return fmt.Errorf("invalid email")
	}

	return s.repo.UpdateEmail(ctx, customerID, newEmail)
}

func (s *Service) CreateProduct(ctx context.Context, name string, price float64) error {
	if name == "" {
		return fmt.Errorf("product name cannot be empty")
	}
	if price < 0 {
		return fmt.Errorf("price cannot be negative")
	}

	product := models.Product{
		ProductName: name,
		Price:       price,
	}

	return s.repo.CreateProduct(ctx, product)
}
