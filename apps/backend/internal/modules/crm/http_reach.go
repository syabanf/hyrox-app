package crm

import (
	"net/http"
	"strings"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// ── Badges ───────────────────────────────────────────────────────────────────

func (h *Handler) listBadges(w http.ResponseWriter, r *http.Request) {
	badges, err := h.service.Badges(r.Context(), httpx.QueryBool(r, "activeOnly", false))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, badges)
}

type badgeBody struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Metric      string  `json:"metric"`
	Threshold   float64 `json:"threshold"`
	BonusXP     int     `json:"bonusXp"`
	Icon        *string `json:"icon"`
	SortOrder   int     `json:"sortOrder"`
	Active      *bool   `json:"active"`
}

func (b *badgeBody) Validate() error {
	if strings.TrimSpace(b.Code) == "" || strings.TrimSpace(b.Name) == "" {
		return httpx.Invalid("A badge needs a code and a name.")
	}
	if !domain.IsValidBadgeMetric(strings.ToUpper(b.Metric)) {
		return httpx.Invalid("%q is not something a badge can measure.", b.Metric)
	}
	if b.BonusXP < 0 {
		return httpx.Invalid("Bonus points cannot be negative.")
	}
	return nil
}

func (h *Handler) saveBadge(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[badgeBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	badge, err := h.service.SaveBadge(r.Context(), BadgeInput{
		Code: body.Code, Name: body.Name, Description: body.Description,
		Metric:    domain.BadgeMetric(strings.ToUpper(body.Metric)),
		Threshold: body.Threshold, BonusXP: body.BonusXP, Icon: body.Icon,
		SortOrder: body.SortOrder, Active: body.Active == nil || *body.Active,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, badge)
}

func (h *Handler) memberBadges(w http.ResponseWriter, r *http.Request) {
	badges, err := h.service.MemberBadges(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, badges)
}

func (h *Handler) myBadges(w http.ResponseWriter, r *http.Request) {
	badges, err := h.service.MemberBadges(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, badges)
}

type awardBadgeBody struct {
	BadgeID string  `json:"badgeId"`
	Note    *string `json:"note"`
}

func (a *awardBadgeBody) Validate() error {
	if strings.TrimSpace(a.BadgeID) == "" {
		return httpx.Invalid("Which badge?")
	}
	return nil
}

func (h *Handler) awardBadge(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[awardBadgeBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	awarded, err := h.service.AwardBadgeByHand(r.Context(), httpx.Param(r, "id"),
		body.BadgeID, body.Note, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, awarded)
}

func (h *Handler) revokeBadge(w http.ResponseWriter, r *http.Request) {
	if err := h.service.RevokeBadge(r.Context(), httpx.Param(r, "id"),
		httpx.Param(r, "badgeId"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"revoked": true})
}

func (h *Handler) evaluateBadges(w http.ResponseWriter, r *http.Request) {
	awarded, err := h.service.EvaluateBadges(r.Context(), httpx.Param(r, "id"), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if awarded == nil {
		awarded = []domain.Badge{}
	}
	httpx.OK(w, awarded)
}

// ── Consent ──────────────────────────────────────────────────────────────────

func (h *Handler) listConsent(w http.ResponseWriter, r *http.Request) {
	prefs, err := h.service.ContactPreferences(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, prefs)
}

func (h *Handler) myConsent(w http.ResponseWriter, r *http.Request) {
	prefs, err := h.service.ContactPreferences(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, prefs)
}

type consentBody struct {
	Channel string  `json:"channel"`
	OptedIn bool    `json:"optedIn"`
	Reason  *string `json:"reason"`
	Scope   string  `json:"scope"`
}

func (c *consentBody) Validate() error {
	if !domain.IsValidContactChannel(strings.ToUpper(c.Channel)) {
		return httpx.Invalid("%q is not a contact channel.", c.Channel)
	}
	return nil
}

func (h *Handler) setConsent(w http.ResponseWriter, r *http.Request) {
	h.saveConsent(w, r, httpx.Param(r, "id"))
}

// setMyConsent is the member changing their own mind, which is the only way
// most opt-outs ever happen.
func (h *Handler) setMyConsent(w http.ResponseWriter, r *http.Request) {
	h.saveConsent(w, r, auth.MemberID(r.Context()))
}

func (h *Handler) saveConsent(w http.ResponseWriter, r *http.Request, memberID string) {
	body, err := httpx.Decode[consentBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	pref, err := h.service.SetContactPreference(r.Context(), PreferenceInput{
		MemberID: memberID, Channel: domain.ContactChannel(strings.ToUpper(body.Channel)),
		OptedIn: body.OptedIn, Reason: body.Reason, Scope: body.Scope,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, pref)
}

// ── Inbox ────────────────────────────────────────────────────────────────────

func (h *Handler) inboxOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.service.InboxOverview(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, overview)
}

func (h *Handler) listConversations(w http.ResponseWriter, r *http.Request) {
	conversations, err := h.service.Conversations(r.Context(), ConversationFilter{
		Status:     strings.ToUpper(httpx.Query(r, "status")),
		Channel:    strings.ToUpper(httpx.Query(r, "channel")),
		AssignedTo: httpx.Query(r, "assignedTo"),
		MemberID:   httpx.Query(r, "memberId"),
		Query:      httpx.Query(r, "query"),
		Limit:      httpx.QueryInt(r, "limit", 100),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, conversations)
}

func (h *Handler) getConversation(w http.ResponseWriter, r *http.Request) {
	conversation, err := h.service.Conversation(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, conversation)
}

type conversationBody struct {
	MemberID      *string  `json:"memberId"`
	ContactName   string   `json:"contactName"`
	ContactHandle *string  `json:"contactHandle"`
	Channel       string   `json:"channel"`
	Subject       string   `json:"subject"`
	Priority      string   `json:"priority"`
	BranchID      *string  `json:"branchId"`
	Tags          []string `json:"tags"`
	Body          string   `json:"body"`
	Inbound       bool     `json:"inbound"`
}

func (c *conversationBody) Validate() error {
	if strings.TrimSpace(c.ContactName) == "" && c.MemberID == nil {
		return httpx.Invalid("A conversation needs somebody on the other end.")
	}
	return nil
}

func (h *Handler) openConversation(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[conversationBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	conversation, err := h.service.OpenConversation(r.Context(), ConversationInput{
		MemberID: body.MemberID, ContactName: body.ContactName,
		ContactHandle: body.ContactHandle, Channel: body.Channel, Subject: body.Subject,
		Priority: body.Priority, BranchID: body.BranchID, Tags: body.Tags,
		Body: body.Body, Inbound: body.Inbound,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, conversation)
}

type conversationUpdateBody struct {
	Status       string   `json:"status"`
	Priority     string   `json:"priority"`
	AssignedTo   *string  `json:"assignedTo"`
	AssignedName *string  `json:"assignedName"`
	Tags         []string `json:"tags"`
	MemberID     *string  `json:"memberId"`
	SetMember    bool     `json:"setMember"`
}

func (h *Handler) updateConversation(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[conversationUpdateBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	conversation, err := h.service.UpdateConversation(r.Context(), httpx.Param(r, "id"),
		ConversationUpdate{
			Status: body.Status, Priority: body.Priority, AssignedTo: body.AssignedTo,
			AssignedName: body.AssignedName, Tags: body.Tags,
			MemberID: body.MemberID, SetMember: body.SetMember,
		}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, conversation)
}

type messageBody struct {
	Body       string  `json:"body"`
	TemplateID *string `json:"templateId"`
	Internal   bool    `json:"internal"`
	Inbound    bool    `json:"inbound"`
}

func (m *messageBody) Validate() error {
	if strings.TrimSpace(m.Body) == "" {
		return httpx.Invalid("A message needs something in it.")
	}
	return nil
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[messageBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	conversation, err := h.service.Reply(r.Context(), httpx.Param(r, "id"), MessageInput{
		Body: body.Body, TemplateID: body.TemplateID,
		Internal: body.Internal, Inbound: body.Inbound,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, conversation)
}

// ── Templates ────────────────────────────────────────────────────────────────

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.service.Templates(r.Context(), httpx.Query(r, "channel"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, templates)
}

type templateBody struct {
	Code      string   `json:"code"`
	Name      string   `json:"name"`
	Channel   string   `json:"channel"`
	Subject   *string  `json:"subject"`
	Body      string   `json:"body"`
	Variables []string `json:"variables"`
	Active    *bool    `json:"active"`
}

func (t *templateBody) Validate() error {
	if strings.TrimSpace(t.Code) == "" || strings.TrimSpace(t.Name) == "" ||
		strings.TrimSpace(t.Body) == "" {
		return httpx.Invalid("A template needs a code, a name and a body.")
	}
	return nil
}

func (h *Handler) saveTemplate(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[templateBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	template, err := h.service.SaveTemplate(r.Context(), TemplateInput{
		Code: body.Code, Name: body.Name, Channel: body.Channel, Subject: body.Subject,
		Body: body.Body, Variables: body.Variables,
		Active: body.Active == nil || *body.Active,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, template)
}

type previewBody struct {
	Values map[string]string `json:"values"`
}

func (h *Handler) previewTemplate(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[previewBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	preview, err := h.service.PreviewTemplate(r.Context(), httpx.Param(r, "id"), body.Values)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, preview)
}

// ── Reviews ──────────────────────────────────────────────────────────────────

func (h *Handler) listReviews(w http.ResponseWriter, r *http.Request) {
	reviews, err := h.service.Reviews(r.Context(), ReviewFilter{
		SubjectType:    strings.ToUpper(httpx.Query(r, "subjectType")),
		SubjectID:      httpx.Query(r, "subjectId"),
		MemberID:       httpx.Query(r, "memberId"),
		Status:         strings.ToUpper(httpx.Query(r, "status")),
		UnansweredOnly: httpx.QueryBool(r, "unansweredOnly", false),
		MaxRating:      httpx.QueryInt(r, "maxRating", 0),
		Limit:          httpx.QueryInt(r, "limit", 100),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, reviews)
}

func (h *Handler) reviewSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.ReviewsFor(r.Context(),
		strings.ToUpper(httpx.Query(r, "subjectType")), httpx.Query(r, "subjectId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, summary)
}

type reviewBody struct {
	SubjectType string  `json:"subjectType"`
	SubjectID   string  `json:"subjectId"`
	Rating      int     `json:"rating"`
	Comment     *string `json:"comment"`
}

func (rb *reviewBody) Validate() error {
	if !domain.IsValidReviewSubject(strings.ToUpper(rb.SubjectType)) {
		return httpx.Invalid("%q is not something that can be reviewed.", rb.SubjectType)
	}
	if strings.TrimSpace(rb.SubjectID) == "" {
		return httpx.Invalid("A review needs to be about something.")
	}
	if rb.Rating < 1 || rb.Rating > 5 {
		return httpx.Invalid("A rating runs from one to five stars.")
	}
	return nil
}

func (h *Handler) leaveReview(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[reviewBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	review, err := h.service.LeaveReview(r.Context(), ReviewInput{
		MemberID:    auth.MemberID(r.Context()),
		SubjectType: strings.ToUpper(body.SubjectType), SubjectID: body.SubjectID,
		Rating: body.Rating, Comment: body.Comment,
	}, Actor{ID: auth.MemberID(r.Context()), Name: "member"})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, review)
}

type replyBody struct {
	Reply string `json:"reply"`
}

func (rb *replyBody) Validate() error {
	if strings.TrimSpace(rb.Reply) == "" {
		return httpx.Invalid("A reply needs something in it.")
	}
	return nil
}

func (h *Handler) replyToReview(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[replyBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	review, err := h.service.ReplyToReview(r.Context(), httpx.Param(r, "id"),
		body.Reply, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, review)
}

type reviewStatusBody struct {
	Status string `json:"status"`
}

func (rb *reviewStatusBody) Validate() error {
	switch strings.ToUpper(rb.Status) {
	case "PUBLISHED", "HIDDEN", "FLAGGED":
		return nil
	}
	return httpx.Invalid("%q is not a review status.", rb.Status)
}

func (h *Handler) setReviewStatus(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[reviewStatusBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	review, err := h.service.SetReviewStatus(r.Context(), httpx.Param(r, "id"),
		body.Status, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, review)
}

// ── Campaign delivery ────────────────────────────────────────────────────────

func (h *Handler) campaignReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.service.CampaignReport(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, report)
}

type markBody struct {
	MemberID string `json:"memberId"`
	Event    string `json:"event"`
}

func (m *markBody) Validate() error {
	if strings.TrimSpace(m.MemberID) == "" {
		return httpx.Invalid("Whose copy?")
	}
	switch strings.ToUpper(m.Event) {
	case "OPENED", "CLICKED":
		return nil
	}
	return httpx.Invalid("%q is not something that happens to a message.", m.Event)
}

func (h *Handler) markCampaign(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[markBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.MarkCampaign(r.Context(), httpx.Param(r, "id"),
		body.MemberID, body.Event); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"marked": true})
}
