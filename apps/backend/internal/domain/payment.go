package domain

import "time"

// PaymentStatus is the lifecycle of one top-up transaction.
type PaymentStatus string

const (
	PaymentDraft    PaymentStatus = "DRAFT"
	PaymentPending  PaymentStatus = "PENDING"
	PaymentPaid     PaymentStatus = "PAID"
	PaymentFailed   PaymentStatus = "FAILED"
	PaymentExpired  PaymentStatus = "EXPIRED"
	PaymentRefunded PaymentStatus = "REFUNDED"
)

// PaymentTransitions are driven by the gateway callback, never by the client.
var PaymentTransitions = TransitionMap[PaymentStatus]{
	PaymentDraft:    {PaymentPending},
	PaymentPending:  {PaymentPaid, PaymentFailed, PaymentExpired},
	PaymentPaid:     {PaymentRefunded},
	PaymentFailed:   {},
	PaymentExpired:  {},
	PaymentRefunded: {},
}

// PaymentChannel is the payment method offered at checkout.
type PaymentChannel string

const (
	ChannelQRIS           PaymentChannel = "QRIS"
	ChannelEWallet        PaymentChannel = "EWALLET"
	ChannelVirtualAccount PaymentChannel = "VIRTUAL_ACCOUNT"
	ChannelCard           PaymentChannel = "CARD"
)

// IsValidPaymentChannel validates a channel arriving from a request.
func IsValidPaymentChannel(value string) bool {
	switch PaymentChannel(value) {
	case ChannelQRIS, ChannelEWallet, ChannelVirtualAccount, ChannelCard:
		return true
	}
	return false
}

// Payment is a money transaction. It is NOT the credit movement: a paid
// payment produces a TOP_UP ledger entry and a lot, and those stay separate so
// money and credits can each be audited on their own terms.
type Payment struct {
	ID          string         `json:"id"`
	MemberID    string         `json:"memberId"`
	PackageID   string         `json:"packageId"`
	Credits     int            `json:"credits"`
	AmountIDR   int64          `json:"amountIdr"`
	DiscountIDR int64          `json:"discountIdr"`
	TotalIDR    int64          `json:"totalIdr"`
	VoucherCode *string        `json:"voucherCode"`
	Channel     PaymentChannel `json:"channel"`
	Status      PaymentStatus  `json:"status"`
	CreatedAt   time.Time      `json:"createdAt"`
	PaidAt      *time.Time     `json:"paidAt"`
	RefundedAt  *time.Time     `json:"refundedAt"`
}
