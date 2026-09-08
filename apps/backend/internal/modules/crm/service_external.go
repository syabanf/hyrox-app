package crm

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// Partners, and acting on what they tell us.

func (s *Service) Partners(ctx context.Context, activeOnly bool) ([]domain.IntegrationPartner, error) {
	return s.repo.Partners(ctx, activeOnly)
}

// PartnerInput describes somebody who sends us facts.
type PartnerInput struct {
	Code         string
	Name         string
	Kind         domain.PartnerKind
	ContactName  *string
	ContactEmail *string
	AwardsXP     bool
	Active       bool
	// Secret rotates the shared key. Nil leaves whatever is there, so renaming
	// a partner does not silently break their integration.
	Secret *string
}

func (s *Service) SavePartner(ctx context.Context, in PartnerInput, actor Actor) (domain.IntegrationPartner, error) {
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" || strings.TrimSpace(in.Name) == "" {
		return domain.IntegrationPartner{}, httpx.Invalid("A partner needs a code and a name.")
	}
	kind := in.Kind
	if kind == "" {
		kind = domain.PartnerOther
	}
	if !domain.IsValidPartnerKind(string(kind)) {
		return domain.IntegrationPartner{}, httpx.Invalid("%q is not a kind of partner.", in.Kind)
	}
	if in.Secret != nil && len(strings.TrimSpace(*in.Secret)) < 16 {
		// A shared secret shorter than this is guessable, and it is the only
		// thing standing between a public endpoint and the loyalty ledger.
		return domain.IntegrationPartner{}, httpx.Invalid(
			"A partner secret needs at least 16 characters.")
	}

	saved, err := s.repo.UpsertPartner(ctx, domain.IntegrationPartner{
		ID: s.ids.New(id.Partner), Code: code, Name: strings.TrimSpace(in.Name), Kind: kind,
		ContactName: in.ContactName, ContactEmail: in.ContactEmail,
		AwardsXP: in.AwardsXP, Active: in.Active,
	}, in.Secret)
	if err != nil {
		return domain.IntegrationPartner{}, err
	}
	s.record(ctx, "crm.partner", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

// EventView is a claim with who it turned out to be about.
type EventView struct {
	domain.ExternalEvent
	PartnerName string `json:"partnerName"`
	MemberName  string `json:"memberName"`
}

func (s *Service) ExternalEvents(ctx context.Context, filter EventFilter) ([]EventView, error) {
	events, err := s.repo.ExternalEvents(ctx, filter)
	if err != nil {
		return nil, err
	}
	partners, err := s.repo.Partners(ctx, false)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, partner := range partners {
		names[partner.ID] = partner.Name
	}

	ids := []string{}
	for _, event := range events {
		if event.MemberID != nil {
			ids = append(ids, *event.MemberID)
		}
	}
	members, err := s.members.MembersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	views := make([]EventView, 0, len(events))
	for _, event := range events {
		view := EventView{ExternalEvent: event, PartnerName: names[event.PartnerID]}
		if event.MemberID != nil {
			view.MemberName = members[*event.MemberID].FullName
		}
		views = append(views, view)
	}
	return views, nil
}

// EventInput is one claim arriving from a partner.
type EventInput struct {
	PartnerCode string
	ExternalID  string
	EventType   string
	Subject     string
	OccurredAt  *time.Time
	Payload     map[string]any
	// XP is what the partner says the event is worth. It is only honoured for
	// a partner trusted to award points, and capped, because a partner's own
	// idea of what their event is worth is not the studio's.
	XP int
}

// ReceiveEvent stores a claim and acts on it.
//
// Storing comes first and always: an event we cannot match to anybody is still
// a fact somebody sent, and throwing it away means it cannot be matched when
// that person joins next week.
func (s *Service) ReceiveEvent(ctx context.Context, in EventInput) (domain.ExternalEvent, bool, error) {
	var event domain.ExternalEvent
	isNew := false

	err := s.db.InTx(ctx, func(ctx context.Context) error {
		partner, _, err := s.repo.PartnerByCode(ctx, strings.ToUpper(strings.TrimSpace(in.PartnerCode)))
		if err != nil {
			return err
		}
		if strings.TrimSpace(in.ExternalID) == "" {
			return httpx.Invalid("An event needs the sender's own id for it.")
		}

		occurred := s.clock.Now()
		if in.OccurredAt != nil {
			occurred = *in.OccurredAt
		}
		payload := in.Payload
		if payload == nil {
			payload = map[string]any{}
		}

		stored, created, err := s.repo.InsertEvent(ctx, domain.ExternalEvent{
			ID: s.ids.New(id.ExternalEvent), PartnerID: partner.ID,
			ExternalID: strings.TrimSpace(in.ExternalID),
			EventType:  strings.ToUpper(strings.TrimSpace(in.EventType)),
			Subject:    strings.TrimSpace(in.Subject), OccurredAt: occurred,
			Payload: payload, Status: domain.EventReceived,
		})
		if err != nil {
			return err
		}
		// A partner retrying is the same event. Nothing further happens,
		// including the points.
		if !created {
			existing, err := s.repo.ExternalEvents(ctx, EventFilter{
				PartnerID: partner.ID, Limit: 1,
			})
			if err != nil {
				return err
			}
			if len(existing) > 0 {
				event = existing[0]
			}
			return nil
		}
		isNew = true
		event = stored

		candidates, err := s.repo.MatchCandidates(ctx)
		if err != nil {
			return err
		}
		memberID, matched := domain.MatchSubject(in.Subject, candidates)
		if matched {
			event.MemberID = &memberID
		}

		outcome := domain.DecideEvent(partner, matched)
		event.Status = outcome.Status

		if outcome.AwardXP && in.XP > 0 {
			// The partner's figure is capped: what an event is worth is the
			// studio's decision, not the sender's.
			awarded := in.XP
			if awarded > partnerXPCap {
				awarded = partnerXPCap
			}
			profile, err := s.profileFor(ctx, memberID, true)
			if err != nil {
				return err
			}
			if _, _, _, err := s.post(ctx, profile, awarded, domain.XPEarn, postDetails{
				Channel: domain.XPFromManual, Type: "EXTERNAL", SourceID: event.ID,
				ReferenceType: "EXTERNAL_EVENT", ReferenceID: event.ID,
				IdempotencyKey: "external:" + event.ID,
				Description:    partner.Name + ": " + event.EventType,
			}); err != nil {
				return err
			}
			event.XPAwarded = awarded
		}

		now := s.clock.Now()
		event.ProcessedAt = &now
		event, err = s.repo.SettleEvent(ctx, event)
		return err
	})
	return event, isNew, err
}

// partnerXPCap is the most points one external event may be worth.
//
// A partner sending 1,000,000 is either a bug or an attack, and either way the
// answer is the same: record what they said, award what the studio is willing
// to give.
const partnerXPCap = 1000

// RematchEvent points an unmatched event at a member by hand.
//
// The payload is untouched: what a partner sent stays what they sent, and only
// our side of the match changes.
func (s *Service) RematchEvent(ctx context.Context, eventID, memberID string, actor Actor) (domain.ExternalEvent, error) {
	var event domain.ExternalEvent
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		existing, err := s.repo.ExternalEvent(ctx, eventID, true)
		if err != nil {
			return err
		}
		if _, err := s.members.Member(ctx, memberID); err != nil {
			return err
		}

		existing.MemberID = &memberID
		existing.Status = domain.EventMatched
		event, err = s.repo.SettleEvent(ctx, existing)
		return err
	})
	if err != nil {
		return domain.ExternalEvent{}, err
	}
	s.record(ctx, "crm.event", eventID, "REMATCH", actor, nil)
	return event, nil
}

// SignedBy checks a partner's HMAC over the raw body.
//
// The same shape as the Instagram webhook, and for the same reason: a public
// endpoint that writes to the loyalty ledger without checking a signature is
// an open door. A partner with no secret configured can post nothing.
func (s *Service) SignedBy(ctx context.Context, partnerCode string, body []byte, signature string) (bool, error) {
	partner, secret, err := s.repo.PartnerByCode(ctx, strings.ToUpper(strings.TrimSpace(partnerCode)))
	if err != nil {
		return false, err
	}
	if secret == "" || !partner.Active {
		return false, nil
	}

	const prefix = "sha256="
	if !strings.HasPrefix(signature, prefix) {
		return false, nil
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, prefix))
	if err != nil {
		return false, nil
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil)), nil
}
