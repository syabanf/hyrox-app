package crm

import (
	"context"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// The inbox: a member says something, somebody answers.
//
// The one number that matters is the gap between those two, so it is computed
// from the messages rather than maintained by hand — a status field somebody
// has to remember to set back is a status field that lies by Wednesday.

// ConversationView is a thread with what has been said in it.
type ConversationView struct {
	domain.Conversation
	MemberName string                       `json:"memberName"`
	Waiting    bool                         `json:"waiting"`
	WaitedSecs int                          `json:"waitedSeconds"`
	Messages   []domain.ConversationMessage `json:"messages"`
}

func (s *Service) Conversations(ctx context.Context, filter ConversationFilter) ([]ConversationView, error) {
	conversations, err := s.repo.Conversations(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.viewConversations(ctx, conversations, false)
}

func (s *Service) viewConversations(ctx context.Context, conversations []domain.Conversation,
	withMessages bool) ([]ConversationView, error) {

	ids := []string{}
	for _, conversation := range conversations {
		if conversation.MemberID != nil {
			ids = append(ids, *conversation.MemberID)
		}
	}
	members, err := s.members.MembersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	views := make([]ConversationView, 0, len(conversations))
	for _, conversation := range conversations {
		view := ConversationView{
			Conversation: conversation,
			Waiting:      conversation.Waiting(),
			WaitedSecs:   conversation.WaitedSeconds(now),
			Messages:     []domain.ConversationMessage{},
		}
		if conversation.MemberID != nil {
			view.MemberName = members[*conversation.MemberID].FullName
		}
		if view.MemberName == "" {
			view.MemberName = conversation.ContactName
		}
		if withMessages {
			messages, err := s.repo.Messages(ctx, conversation.ID)
			if err != nil {
				return nil, err
			}
			view.Messages = messages
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Service) Conversation(ctx context.Context, conversationID string) (ConversationView, error) {
	conversation, err := s.repo.Conversation(ctx, conversationID, false)
	if err != nil {
		return ConversationView{}, err
	}
	views, err := s.viewConversations(ctx, []domain.Conversation{conversation}, true)
	if err != nil {
		return ConversationView{}, err
	}
	return views[0], nil
}

// InboxOverview is how the desk is doing right now.
type InboxOverview struct {
	Metrics domain.InboxMetrics `json:"metrics"`
	// Oldest is the queue itself: unanswered, longest wait first.
	Oldest []ConversationView `json:"oldest"`
}

func (s *Service) InboxOverview(ctx context.Context) (InboxOverview, error) {
	conversations, err := s.repo.Conversations(ctx, ConversationFilter{Limit: 500})
	if err != nil {
		return InboxOverview{}, err
	}
	views, err := s.viewConversations(ctx, conversations, false)
	if err != nil {
		return InboxOverview{}, err
	}

	overview := InboxOverview{
		Metrics: domain.SummarizeInbox(conversations, s.clock.Now()),
		Oldest:  []ConversationView{},
	}
	for _, view := range views {
		if view.Waiting && len(overview.Oldest) < 10 {
			overview.Oldest = append(overview.Oldest, view)
		}
	}
	return overview, nil
}

// ConversationInput opens a thread.
type ConversationInput struct {
	MemberID      *string
	ContactName   string
	ContactHandle *string
	Channel       string
	Subject       string
	Priority      string
	BranchID      *string
	Tags          []string
	// Body is the first message. A thread with nothing in it is a row nobody
	// will ever look at.
	Body string
	// ExternalID is the provider's own id for an inbound message, which is the
	// whole of the idempotency story for a webhook.
	ExternalID *string
	Inbound    bool
}

// OpenConversation starts a thread, or continues an existing one with the same
// person on the same channel.
//
// Continuing rather than starting is the difference between an inbox and a
// list: somebody who writes three times about one thing has one problem.
func (s *Service) OpenConversation(ctx context.Context, in ConversationInput, actor Actor) (ConversationView, error) {
	var conversationID string
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		channel := strings.ToUpper(strings.TrimSpace(in.Channel))
		if channel == "" {
			channel = "INBOX"
		}

		var conversation domain.Conversation
		found := false
		if in.ContactHandle != nil && *in.ContactHandle != "" {
			existing, ok, err := s.repo.ConversationByHandle(ctx, channel, *in.ContactHandle)
			if err != nil {
				return err
			}
			conversation, found = existing, ok
		}

		if !found {
			priority := strings.ToUpper(strings.TrimSpace(in.Priority))
			if priority == "" {
				priority = "NORMAL"
			}
			tags := in.Tags
			if tags == nil {
				tags = []string{}
			}
			created, err := s.repo.InsertConversation(ctx, domain.Conversation{
				ID: s.ids.New(id.Conversation), MemberID: in.MemberID,
				ContactName: strings.TrimSpace(in.ContactName), ContactHandle: in.ContactHandle,
				Channel: channel, Subject: strings.TrimSpace(in.Subject),
				Status: domain.ConversationOpen, Priority: priority,
				BranchID: in.BranchID, Tags: tags,
			})
			if err != nil {
				return err
			}
			conversation = created
		}
		conversationID = conversation.ID

		if strings.TrimSpace(in.Body) == "" {
			return nil
		}
		return s.postMessage(ctx, conversation, MessageInput{
			Body: in.Body, ExternalID: in.ExternalID, Inbound: in.Inbound,
		}, actor)
	})
	if err != nil {
		return ConversationView{}, err
	}
	return s.Conversation(ctx, conversationID)
}

// MessageInput is one thing said.
type MessageInput struct {
	Body       string
	TemplateID *string
	ExternalID *string
	// Internal is a note to colleagues rather than a reply. It does not count
	// as answering the member, which is why it cannot move the response clock.
	Internal bool
	Inbound  bool
}

// Reply posts a message and moves the thread accordingly.
func (s *Service) Reply(ctx context.Context, conversationID string, in MessageInput, actor Actor) (ConversationView, error) {
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		conversation, err := s.repo.Conversation(ctx, conversationID, true)
		if err != nil {
			return err
		}
		if conversation.Status == domain.ConversationClosed {
			return httpx.Conflict("CLOSED", "That conversation is closed.")
		}
		return s.postMessage(ctx, conversation, in, actor)
	})
	if err != nil {
		return ConversationView{}, err
	}
	return s.Conversation(ctx, conversationID)
}

// postMessage writes one message and updates the thread's clocks.
//
// The first-response time is set once, from the first *outbound, non-internal*
// message: a note to a colleague is not an answer, and letting one stop the
// clock would make the desk look faster than it is.
func (s *Service) postMessage(ctx context.Context, conversation domain.Conversation,
	in MessageInput, actor Actor) error {

	body := strings.TrimSpace(in.Body)
	if body == "" {
		return httpx.Invalid("A message needs something in it.")
	}

	direction := "OUTBOUND"
	if in.Inbound {
		direction = "INBOUND"
	}

	message := domain.ConversationMessage{
		ID: s.ids.New(id.ChatMessage), ConversationID: conversation.ID,
		Direction: direction, Body: body, TemplateID: in.TemplateID,
		ExternalID: in.ExternalID, Internal: in.Internal,
	}
	if !in.Inbound {
		message.AuthorID = &actor.ID
		message.AuthorName = &actor.Name
	}

	_, isNew, err := s.repo.InsertMessage(ctx, message)
	if err != nil {
		return err
	}
	// A provider redelivering the same message is the same message. Nothing
	// further happens, including the clocks.
	if !isNew {
		return nil
	}

	now := s.clock.Now()
	if in.Inbound {
		conversation.LastMemberAt = &now
		// A member writing again reopens a resolved thread: they are not
		// finished, whatever we decided.
		if conversation.Status == domain.ConversationResolved {
			conversation.Status = domain.ConversationOpen
			conversation.ResolvedAt = nil
		}
	} else if !in.Internal {
		conversation.LastStaffAt = &now
		if conversation.FirstResponseSeconds == nil && conversation.LastMemberAt != nil {
			seconds := int(now.Sub(*conversation.LastMemberAt).Seconds())
			conversation.FirstResponseSeconds = &seconds
		}
		if conversation.Status == domain.ConversationOpen {
			conversation.Status = domain.ConversationPending
		}
	}

	_, err = s.repo.SaveConversation(ctx, conversation)
	return err
}

// ConversationUpdate changes who owns a thread and where it stands.
type ConversationUpdate struct {
	Status       string
	Priority     string
	AssignedTo   *string
	AssignedName *string
	Tags         []string
	MemberID     *string
	SetMember    bool
}

func (s *Service) UpdateConversation(ctx context.Context, conversationID string,
	in ConversationUpdate, actor Actor) (ConversationView, error) {

	err := s.db.InTx(ctx, func(ctx context.Context) error {
		conversation, err := s.repo.Conversation(ctx, conversationID, true)
		if err != nil {
			return err
		}

		if status := strings.ToUpper(strings.TrimSpace(in.Status)); status != "" {
			next, err := domain.Transition(domain.ConversationTransitions,
				conversation.Status, domain.ConversationStatus(status))
			if err != nil {
				return httpx.Conflict("INVALID_TRANSITION",
					"That conversation is %s and cannot become %s.",
					strings.ToLower(string(conversation.Status)), strings.ToLower(status))
			}
			conversation.Status = next
			if next == domain.ConversationResolved {
				now := s.clock.Now()
				conversation.ResolvedAt = &now
			}
		}
		if priority := strings.ToUpper(strings.TrimSpace(in.Priority)); priority != "" {
			conversation.Priority = priority
		}
		if in.AssignedTo != nil {
			conversation.AssignedTo = in.AssignedTo
			conversation.AssignedName = in.AssignedName
		}
		if in.Tags != nil {
			conversation.Tags = in.Tags
		}
		if in.SetMember {
			conversation.MemberID = in.MemberID
		}

		_, err = s.repo.SaveConversation(ctx, conversation)
		return err
	})
	if err != nil {
		return ConversationView{}, err
	}
	s.record(ctx, "crm.conversation", conversationID, "UPDATE", actor, nil)
	return s.Conversation(ctx, conversationID)
}

// ── Templates ────────────────────────────────────────────────────────────────

func (s *Service) Templates(ctx context.Context, channel string) ([]MessageTemplate, error) {
	return s.repo.Templates(ctx, strings.ToUpper(strings.TrimSpace(channel)))
}

// TemplateInput is words written once and sent many times.
type TemplateInput struct {
	Code      string
	Name      string
	Channel   string
	Subject   *string
	Body      string
	Variables []string
	Active    bool
}

func (s *Service) SaveTemplate(ctx context.Context, in TemplateInput, actor Actor) (MessageTemplate, error) {
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Body) == "" {
		return MessageTemplate{}, httpx.Invalid("A template needs a code, a name and a body.")
	}
	channel := strings.ToUpper(strings.TrimSpace(in.Channel))
	if channel == "" {
		channel = "INBOX"
	}
	variables := in.Variables
	if variables == nil {
		variables = []string{}
	}

	saved, err := s.repo.UpsertTemplate(ctx, MessageTemplate{
		ID: s.ids.New(id.Template), Code: code, Name: strings.TrimSpace(in.Name),
		Channel: channel, Subject: in.Subject, Body: in.Body,
		Variables: variables, Active: in.Active,
	})
	if err != nil {
		return MessageTemplate{}, err
	}
	s.record(ctx, "crm.template", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

// RenderTemplate fills a template's placeholders.
//
// A missing variable is left visible rather than blanked, so a preview shows
// "Hi {{name}}" and somebody notices before four hundred people do.
func RenderTemplate(body string, values map[string]string) string {
	rendered := body
	for key, value := range values {
		rendered = strings.ReplaceAll(rendered, "{{"+key+"}}", value)
	}
	return rendered
}

// TemplatePreview is a template with one member's details filled in.
type TemplatePreview struct {
	Subject string   `json:"subject"`
	Body    string   `json:"body"`
	Missing []string `json:"missing"`
}

// PreviewTemplate renders a template against real values, and says which
// placeholders had nothing to fill them.
func (s *Service) PreviewTemplate(ctx context.Context, templateID string,
	values map[string]string) (TemplatePreview, error) {

	template, err := s.repo.Template(ctx, templateID)
	if err != nil {
		return TemplatePreview{}, err
	}

	preview := TemplatePreview{Body: RenderTemplate(template.Body, values), Missing: []string{}}
	if template.Subject != nil {
		preview.Subject = RenderTemplate(*template.Subject, values)
	}
	for _, variable := range template.Variables {
		if _, ok := values[variable]; !ok {
			preview.Missing = append(preview.Missing, variable)
		}
	}
	return preview, nil
}

// ── Campaign delivery ────────────────────────────────────────────────────────

// CampaignReportView is what a send did, with the recipients behind it.
type CampaignReportView struct {
	Report     domain.CampaignReport      `json:"report"`
	Recipients []domain.CampaignRecipient `json:"recipients"`
}

func (s *Service) CampaignReport(ctx context.Context, campaignID string) (CampaignReportView, error) {
	recipients, err := s.repo.CampaignRecipients(ctx, campaignID)
	if err != nil {
		return CampaignReportView{}, err
	}
	return CampaignReportView{
		Report: domain.SummarizeCampaign(recipients), Recipients: recipients,
	}, nil
}

// MarkCampaign records an open or a click.
func (s *Service) MarkCampaign(ctx context.Context, campaignID, memberID, event string) error {
	event = strings.ToUpper(strings.TrimSpace(event))
	if event != "OPENED" && event != "CLICKED" {
		return httpx.Invalid("%q is not something that happens to a message.", event)
	}
	return s.repo.MarkRecipient(ctx, campaignID, memberID, event, s.clock.Now())
}

// AudienceFor narrows a list of members by consent.
//
// Consent is applied here rather than remembered by whoever builds the
// audience, which is the difference between an opt-out that works and one that
// works until somebody writes a new segment.
func (s *Service) AudienceFor(ctx context.Context, campaignID string, members []string,
	channel domain.ContactChannel) ([]string, error) {

	optedOut, err := s.repo.OptedOutOf(ctx, channel)
	if err != nil {
		return nil, err
	}

	allowed := make([]string, 0, len(members))
	reason := "opted out of marketing"
	for _, member := range members {
		if optedOut[member] {
			// A skip is recorded rather than passed over: a campaign that
			// reached 40 of 400 should say why, and "the list is decaying" is
			// a different problem from "the send failed".
			if err := s.repo.UpsertRecipient(ctx, domain.CampaignRecipient{
				ID: s.ids.New(id.Recipient), CampaignID: campaignID, MemberID: member,
				Status: domain.RecipientSkipped, SkipReason: &reason,
			}); err != nil {
				return nil, err
			}
			continue
		}
		allowed = append(allowed, member)
	}
	return allowed, nil
}

// RecordSend notes that a member's copy went out.
func (s *Service) RecordSend(ctx context.Context, campaignID, memberID string,
	notificationID *string, sentAt time.Time) error {

	return s.repo.UpsertRecipient(ctx, domain.CampaignRecipient{
		ID: s.ids.New(id.Recipient), CampaignID: campaignID, MemberID: memberID,
		Status: domain.RecipientSent, NotificationID: notificationID, SentAt: &sentAt,
	})
}

// ── Campaign preview ─────────────────────────────────────────────────────────

// CampaignPreview is a campaign as one member will actually receive it.
//
// Different from a template preview, which renders words against values
// somebody typed. This renders against a real member's real details, which is
// where you find out that half the audience has no first name on file.
type CampaignPreview struct {
	MemberID   string `json:"memberId"`
	MemberName string `json:"memberName"`
	Message    string `json:"message"`
	DeepLink   string `json:"deepLink"`
	// Missing names the placeholders this member has nothing to fill, which is
	// the thing worth knowing before four hundred people read "Hi {{name}}".
	Missing []string `json:"missing"`
	// Deliverable is whether this member would actually be sent it: consent is
	// part of the preview, because "who will get this" is the question.
	Deliverable bool   `json:"deliverable"`
	SkipReason  string `json:"skipReason,omitempty"`
}

// PreviewCampaign renders a campaign for particular members.
//
// When no members are named it takes the first few of the audience, so
// somebody can see what the send looks like before committing to it.
func (s *Service) PreviewCampaign(ctx context.Context, message, deepLink string,
	memberIDs []string, channel domain.ContactChannel) ([]CampaignPreview, error) {

	if strings.TrimSpace(message) == "" {
		return nil, httpx.Invalid("A campaign needs something to say.")
	}
	if len(memberIDs) == 0 {
		return nil, httpx.Invalid("Preview it against at least one member.")
	}
	if channel == "" {
		channel = domain.ContactPush
	}

	members, err := s.members.MembersByIDs(ctx, memberIDs)
	if err != nil {
		return nil, err
	}

	previews := make([]CampaignPreview, 0, len(memberIDs))
	for _, memberID := range memberIDs {
		member, known := members[memberID]
		if !known {
			continue
		}

		values := map[string]string{
			"name":      member.FullName,
			"firstName": firstWord(member.FullName),
			"email":     member.Email,
			"phone":     member.Phone,
		}
		preview := CampaignPreview{
			MemberID: memberID, MemberName: member.FullName,
			Message: RenderTemplate(message, values),
			Missing: []string{},
		}
		if deepLink != "" {
			preview.DeepLink = RenderTemplate(deepLink, values)
		}
		// Anything still in braces had nothing to fill it.
		for _, placeholder := range placeholders(preview.Message) {
			preview.Missing = append(preview.Missing, placeholder)
		}

		prefs, err := s.repo.ContactPreferences(ctx, memberID)
		if err != nil {
			return nil, err
		}
		preview.Deliverable = domain.MayContact(prefs, channel, domain.MessageMarketing)
		if !preview.Deliverable {
			preview.SkipReason = "opted out of marketing"
		}
		previews = append(previews, preview)
	}
	return previews, nil
}

func firstWord(value string) string {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// placeholders finds the {{...}} that survived rendering.
func placeholders(text string) []string {
	out := []string{}
	rest := text
	for {
		open := strings.Index(rest, "{{")
		if open < 0 {
			return out
		}
		rest = rest[open+2:]
		close := strings.Index(rest, "}}")
		if close < 0 {
			return out
		}
		out = append(out, strings.TrimSpace(rest[:close]))
		rest = rest[close+2:]
	}
}
