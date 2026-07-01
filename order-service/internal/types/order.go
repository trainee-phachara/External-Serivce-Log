package types

import "time"

type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusProcessing OrderStatus = "processing"
	OrderStatusShipped    OrderStatus = "shipped"
	OrderStatusDelivered  OrderStatus = "delivered"
	OrderStatusCancelled  OrderStatus = "cancelled"
)

type OrderItem struct {
	ProductID   int     `json:"product_id"`
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type Order struct {
	ID          int         `json:"id"`
	UserID      int         `json:"user_id"`
	Items       []OrderItem `json:"items"`
	Status      OrderStatus `json:"status"`
	TotalAmount float64     `json:"total_amount"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

type CreateOrderInput struct {
	UserID int
	Items  []OrderItem
}

type UpdateOrderInput struct {
	Status *OrderStatus
}
