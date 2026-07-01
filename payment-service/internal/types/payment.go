package types

import "time"

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusCompleted PaymentStatus = "completed"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusRefunded  PaymentStatus = "refunded"
)

type PaymentMethod string

const (
	PaymentMethodCreditCard    PaymentMethod = "credit_card"
	PaymentMethodBankTransfer  PaymentMethod = "bank_transfer"
	PaymentMethodPromptPay     PaymentMethod = "promptpay"
)

type Payment struct {
	ID        int           `json:"id"`
	OrderID   int           `json:"order_id"`
	UserID    int           `json:"user_id"`
	Amount    float64       `json:"amount"`
	Status    PaymentStatus `json:"status"`
	Method    PaymentMethod `json:"method"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type CreatePaymentInput struct {
	OrderID int
	UserID  int
	Amount  float64
	Method  PaymentMethod
}

type UpdatePaymentInput struct {
	Status *PaymentStatus
}
