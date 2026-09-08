package crm

import (
	"context"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/outbox"
)

// Earning points from what happens elsewhere in the studio.
//
// These are outbox consumers, not calls from scheduling, wallet or access:
// those modules publish a fact inside their own transaction and have no idea
// loyalty exists. If the studio decides tomorrow that a class is worth nothing,
// only a rule changes and no booking code is touched.
//
// Every award carries the outbox message's own id as its idempotency key,
// because at-least-once delivery means these handlers will see the same event
// twice sooner or later — and points awarded twice are points given away.

// HandleBookingConfirmed awards points for booking a class.
func (s *Service) HandleBookingConfirmed(ctx context.Context, msg outbox.Message) error {
	payload, err := outbox.Decode[struct {
		BookingID string `json:"bookingId"`
		MemberID  string `json:"memberId"`
		SessionID string `json:"sessionId"`
	}](msg)
	if err != nil {
		return err
	}
	// The same topic also carries schedule-change notices, which name no
	// member and are none of loyalty's business.
	if payload.MemberID == "" {
		return nil
	}

	_, err = s.Award(ctx, AwardInput{
		MemberID: payload.MemberID, Channel: domain.XPFromBooking, Type: "CONFIRMED",
		SourceID: payload.SessionID, Items: 1,
		IdempotencyKey: "outbox:" + msg.ID,
		ReferenceType:  "BOOKING", ReferenceID: payload.BookingID,
		Description: "Class booked",
	})
	return err
}

// HandlePaymentPaid awards points for money actually spent, and keeps lifetime
// spend current — the other half of a tier rule, so somebody can climb by
// spending as well as by turning up.
func (s *Service) HandlePaymentPaid(ctx context.Context, msg outbox.Message) error {
	payload, err := outbox.Decode[struct {
		PaymentID string  `json:"paymentId"`
		MemberID  string  `json:"memberId"`
		TotalIDR  float64 `json:"totalIdr"`
	}](msg)
	if err != nil {
		return err
	}
	if payload.MemberID == "" {
		return nil
	}

	_, err = s.Award(ctx, AwardInput{
		MemberID: payload.MemberID, Channel: domain.XPFromPayment, Type: "TOP_UP",
		AmountIDR: payload.TotalIDR, Items: 1,
		IdempotencyKey: "outbox:" + msg.ID,
		ReferenceType:  "PAYMENT", ReferenceID: payload.PaymentID,
		Description: "Top-up settled",
	})
	return err
}

// HandleVisitLogged awards points for actually turning up, which is a
// different thing from booking and worth rewarding separately.
//
// Access only publishes this for a charged entry, so free re-entry inside the
// grace window never earns a second award.
func (s *Service) HandleVisitLogged(ctx context.Context, msg outbox.Message) error {
	payload, err := outbox.Decode[struct {
		MemberID  string `json:"memberId"`
		BranchID  string `json:"branchId"`
		SessionID string `json:"sessionId"`
	}](msg)
	if err != nil {
		return err
	}
	if payload.MemberID == "" {
		return nil
	}

	_, err = s.Award(ctx, AwardInput{
		MemberID: payload.MemberID, Channel: domain.XPFromClass, Type: "ATTENDED",
		SourceID: payload.SessionID, BranchID: payload.BranchID, Items: 1,
		IdempotencyKey: "outbox:" + msg.ID,
		ReferenceType:  "VISIT", ReferenceID: payload.SessionID,
		Description: "Attended a class",
	})
	return err
}
