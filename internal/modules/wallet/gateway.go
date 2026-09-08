package wallet

import (
	"context"
	"fmt"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
)

// InvoiceRequest is what the studio asks the payment provider to collect.
type InvoiceRequest struct {
	PaymentID   string
	MemberID    string
	MemberName  string
	MemberEmail string
	AmountIDR   int64
	Channel     domain.PaymentChannel
	Description string
	ExpiresAt   time.Time
}

// Invoice is the provider's answer: an id to reconcile the callback against,
// and whatever the member needs in order to pay.
type Invoice struct {
	ExternalID string `json:"externalId"`
	// PaymentURL is a hosted checkout page, when the channel has one.
	PaymentURL string `json:"paymentUrl,omitempty"`
	// QRString is the QRIS payload the app renders.
	QRString string `json:"qrString,omitempty"`
	// VirtualAccount is the transfer destination for VA payments.
	VirtualAccount string    `json:"virtualAccount,omitempty"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

// Gateway is the payment provider port.
//
// Settlement is not modelled as a method here on purpose: real providers tell
// you a payment succeeded by calling your webhook, so the inbound callback is
// the only settlement path, and the mock provider drives it the same way.
type Gateway interface {
	Name() string
	CreateInvoice(ctx context.Context, req InvoiceRequest) (Invoice, error)
	// VerifyCallback authenticates an inbound webhook and returns the payment
	// it refers to along with its new status.
	VerifyCallback(ctx context.Context, token string, body []byte) (CallbackEvent, error)
}

// CallbackEvent is a settlement notification from the provider.
type CallbackEvent struct {
	PaymentID  string
	ExternalID string
	Status     domain.PaymentStatus
	PaidAt     time.Time
}

// MockGateway stands in for Xendit while the studio is in demo or staging.
//
// It creates invoices that look real enough for the member app to render, and
// accepts a callback from the "simulate payment" action. Swapping in the real
// provider is a wiring change, not a code change anywhere else.
type MockGateway struct {
	clock func() time.Time
}

func NewMockGateway(now func() time.Time) *MockGateway { return &MockGateway{clock: now} }

func (m *MockGateway) Name() string { return "mock" }

func (m *MockGateway) CreateInvoice(_ context.Context, req InvoiceRequest) (Invoice, error) {
	invoice := Invoice{
		ExternalID: "mock_" + req.PaymentID,
		ExpiresAt:  req.ExpiresAt,
	}
	switch req.Channel {
	case domain.ChannelQRIS:
		invoice.QRString = fmt.Sprintf("00020101021226%s5204739953033605802ID", req.PaymentID)
	case domain.ChannelVirtualAccount:
		// A stable pseudo-account derived from the payment id, so the number
		// shown to the member does not change if they reload the page.
		invoice.VirtualAccount = fmt.Sprintf("8808%010d", hash32(req.PaymentID)%1_000_000_0000)
	case domain.ChannelEWallet, domain.ChannelCard:
		invoice.PaymentURL = "https://checkout.invalid/mock/" + req.PaymentID
	}
	return invoice, nil
}

// VerifyCallback accepts the simulated settlement. There is no signature to
// check because there is no real provider behind it.
func (m *MockGateway) VerifyCallback(_ context.Context, _ string, _ []byte) (CallbackEvent, error) {
	return CallbackEvent{}, fmt.Errorf("wallet: the mock gateway has no webhook; settle through the simulate endpoint")
}

func hash32(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
