package crm

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// ── Partners ─────────────────────────────────────────────────────────────────

func (h *Handler) listPartners(w http.ResponseWriter, r *http.Request) {
	partners, err := h.service.Partners(r.Context(), httpx.QueryBool(r, "activeOnly", false))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, partners)
}

type partnerBody struct {
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	ContactName  *string `json:"contactName"`
	ContactEmail *string `json:"contactEmail"`
	AwardsXP     bool    `json:"awardsXp"`
	Active       *bool   `json:"active"`
	// Omitted leaves the existing secret alone: rotating it should be a
	// deliberate act, not a side effect of renaming a partner.
	Secret *string `json:"secret"`
}

func (p *partnerBody) Validate() error {
	if strings.TrimSpace(p.Code) == "" || strings.TrimSpace(p.Name) == "" {
		return httpx.Invalid("A partner needs a code and a name.")
	}
	if p.Kind != "" && !domain.IsValidPartnerKind(strings.ToUpper(p.Kind)) {
		return httpx.Invalid("%q is not a kind of partner.", p.Kind)
	}
	return nil
}

func (h *Handler) savePartner(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[partnerBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	partner, err := h.service.SavePartner(r.Context(), PartnerInput{
		Code: body.Code, Name: body.Name,
		Kind:        domain.PartnerKind(strings.ToUpper(body.Kind)),
		ContactName: body.ContactName, ContactEmail: body.ContactEmail,
		AwardsXP: body.AwardsXP, Active: body.Active == nil || *body.Active,
		Secret: body.Secret,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, partner)
}

// ── Events ───────────────────────────────────────────────────────────────────

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	events, err := h.service.ExternalEvents(r.Context(), EventFilter{
		PartnerID: httpx.Query(r, "partnerId"),
		MemberID:  httpx.Query(r, "memberId"),
		Status:    strings.ToUpper(httpx.Query(r, "status")),
		EventType: strings.ToUpper(httpx.Query(r, "eventType")),
		Limit:     httpx.QueryInt(r, "limit", 100),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, events)
}

type rematchBody struct {
	MemberID string `json:"memberId"`
}

func (m *rematchBody) Validate() error {
	if strings.TrimSpace(m.MemberID) == "" {
		return httpx.Invalid("Which member is it about?")
	}
	return nil
}

func (h *Handler) rematchEvent(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[rematchBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	event, err := h.service.RematchEvent(r.Context(), httpx.Param(r, "id"),
		body.MemberID, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, event)
}

// externalEventBody is what a partner posts.
type externalEventBody struct {
	ExternalID string         `json:"externalId"`
	EventType  string         `json:"eventType"`
	Subject    string         `json:"subject"`
	OccurredAt *string        `json:"occurredAt"`
	Payload    map[string]any `json:"payload"`
	XP         int            `json:"xp"`
}

// receiveEvent is the partner's endpoint.
//
// Signed rather than authenticated, like the Instagram webhook and for the
// same reason: the caller has no session, and the question is whether this
// body was written by somebody holding the shared secret.
func (h *Handler) receiveEvent(w http.ResponseWriter, r *http.Request) {
	partnerCode := httpx.Param(r, "partner")

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("That event could not be read."))
		return
	}
	signed, err := h.service.SignedBy(r.Context(), partnerCode, body, r.Header.Get("X-Signature-256"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !signed {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("That event is not signed by the partner."))
		return
	}

	var event externalEventBody
	if err := json.Unmarshal(body, &event); err != nil {
		httpx.Fail(w, r, httpx.Invalid("That event is not valid JSON."))
		return
	}
	if strings.TrimSpace(event.ExternalID) == "" {
		httpx.Fail(w, r, httpx.Invalid("An event needs your own id for it, so a retry is not a second event."))
		return
	}

	var occurred *time.Time
	if event.OccurredAt != nil {
		if parsed, err := time.Parse(time.RFC3339, *event.OccurredAt); err == nil {
			occurred = &parsed
		}
	}

	stored, isNew, err := h.service.ReceiveEvent(r.Context(), EventInput{
		PartnerCode: partnerCode, ExternalID: event.ExternalID, EventType: event.EventType,
		Subject: event.Subject, OccurredAt: occurred, Payload: event.Payload, XP: event.XP,
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// A retry gets 200 rather than 201, so a partner can tell the difference
	// between "stored" and "stored again".
	if isNew {
		httpx.Created(w, stored)
		return
	}
	httpx.OK(w, stored)
}

// ── Campaign preview ─────────────────────────────────────────────────────────

type campaignPreviewBody struct {
	Message   string   `json:"message"`
	DeepLink  string   `json:"deepLink"`
	MemberIDs []string `json:"memberIds"`
	Channel   string   `json:"channel"`
}

func (c *campaignPreviewBody) Validate() error {
	if strings.TrimSpace(c.Message) == "" {
		return httpx.Invalid("A campaign needs something to say.")
	}
	if len(c.MemberIDs) == 0 {
		return httpx.Invalid("Preview it against at least one member.")
	}
	if c.Channel != "" && !domain.IsValidContactChannel(strings.ToUpper(c.Channel)) {
		return httpx.Invalid("%q is not a contact channel.", c.Channel)
	}
	return nil
}

func (h *Handler) previewCampaign(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[campaignPreviewBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	previews, err := h.service.PreviewCampaign(r.Context(), body.Message, body.DeepLink,
		body.MemberIDs, domain.ContactChannel(strings.ToUpper(body.Channel)))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, previews)
}

// ReceiveEvent is exported so the partner webhook can be mounted outside the
// guarded surface: it authenticates by signature, not by session.
func (h *Handler) ReceiveEvent(w http.ResponseWriter, r *http.Request) {
	h.receiveEvent(w, r)
}
