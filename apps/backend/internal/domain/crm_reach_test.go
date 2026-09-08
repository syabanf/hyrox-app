package domain

import (
	"testing"
	"time"
)

func at(hhmm string) time.Time {
	t, _ := time.Parse(time.RFC3339, "2026-09-08T"+hhmm+":00+07:00")
	return t
}

func TestMarketingIsOptOutAndTransactionalIsNot(t *testing.T) {
	// Silence means yes for marketing: somebody who signed up has agreed to
	// hear from the place they signed up to.
	if !MayContact(nil, ContactPush, MessageMarketing) {
		t.Fatal("a member who has never been asked may be sent a campaign")
	}

	optedOut := []ContactPreference{{Channel: ContactPush, OptedIn: false, Scope: "MARKETING"}}
	if MayContact(optedOut, ContactPush, MessageMarketing) {
		t.Fatal("a marketing opt-out must stop a campaign")
	}
	// But not the message telling them their class is cancelled.
	if !MayContact(optedOut, ContactPush, MessageTransactional) {
		t.Fatal("a marketing opt-out must not silence a booking confirmation")
	}
	// "Never contact me on this channel" means it.
	all := []ContactPreference{{Channel: ContactPush, OptedIn: false, Scope: "ALL"}}
	if MayContact(all, ContactPush, MessageTransactional) {
		t.Fatal("an ALL-scope refusal stops everything on that channel")
	}
	// And a refusal on one channel says nothing about another.
	if !MayContact(all, ContactEmail, MessageMarketing) {
		t.Fatal("refusing push does not refuse email")
	}
}

func TestABadgeIsEarnedOnceAndNeverAutomaticallyWhenManual(t *testing.T) {
	badges := []Badge{
		{ID: "b10", Metric: BadgeVisits, Threshold: 10, Active: true, SortOrder: 1},
		{ID: "b50", Metric: BadgeVisits, Threshold: 50, Active: true, SortOrder: 2},
		{ID: "hand", Metric: BadgeManual, Threshold: 0, Active: true, SortOrder: 3},
		{ID: "off", Metric: BadgeVisits, Threshold: 1, Active: false},
	}
	metrics := MemberMetrics{Visits: 60}

	earned := EarnedBadges(badges, metrics, nil)
	if len(earned) != 2 || earned[0].ID != "b10" || earned[1].ID != "b50" {
		t.Fatalf("both visit badges are earned, and neither the manual nor the disabled one: %v", earned)
	}

	// Already held is not earned again: a badge that can be earned twice is a
	// counter wearing a badge's clothes.
	again := EarnedBadges(badges, metrics, map[string]bool{"b10": true})
	if len(again) != 1 || again[0].ID != "b50" {
		t.Fatalf("a held badge is not re-awarded, got %v", again)
	}
}

func TestCampaignRatesAreAgainstWhatWasSentNotTheAudience(t *testing.T) {
	opened, clicked := at("09:00"), at("09:05")
	skip := "opted out"
	recipients := []CampaignRecipient{
		{Status: RecipientSent, SentAt: &opened},
		{Status: RecipientSent, SentAt: &opened, OpenedAt: &opened},
		// Somebody who clicked also opened. Counting them as separate
		// populations would make every campaign look worse than it was.
		{Status: RecipientClicked, SentAt: &opened, OpenedAt: &opened, ClickedAt: &clicked},
		{Status: RecipientSkipped, SkipReason: &skip},
		{Status: RecipientSkipped, SkipReason: &skip},
		{Status: RecipientFailed},
	}

	report := SummarizeCampaign(recipients)
	if report.Audience != 6 || report.Sent != 3 || report.Skipped != 2 || report.Failed != 1 {
		t.Fatalf("unexpected counts: %+v", report)
	}
	if report.Opened != 2 || report.Clicked != 1 {
		t.Fatalf("a click is also an open, got %d opened and %d clicked", report.Opened, report.Clicked)
	}
	// Two of three sent, not two of six.
	if report.OpenRate != 66.67 {
		t.Fatalf("the open rate is against what was sent, got %v", report.OpenRate)
	}
	if report.SkipReasons["opted out"] != 2 {
		t.Fatalf("skips should be counted by reason, got %v", report.SkipReasons)
	}
}

func TestAThreadIsWaitingWhenTheMemberSpokeLast(t *testing.T) {
	member, staff := at("09:00"), at("09:30")

	// Answered: we spoke after they did.
	answered := Conversation{Status: ConversationOpen, LastMemberAt: &member, LastStaffAt: &staff}
	if answered.Waiting() {
		t.Fatal("a thread answered after the last member message is not waiting")
	}

	// They wrote again. This is the case a status field alone gets wrong,
	// because nobody remembers to set it back to OPEN.
	later := at("10:00")
	reopened := Conversation{Status: ConversationOpen, LastMemberAt: &later, LastStaffAt: &staff}
	if !reopened.Waiting() {
		t.Fatal("a member writing again puts the thread back in the queue")
	}
	if got := reopened.WaitedSeconds(at("10:30")); got != 1800 {
		t.Fatalf("waited half an hour, got %d seconds", got)
	}

	// Resolved threads are nobody's queue.
	resolved := Conversation{Status: ConversationResolved, LastMemberAt: &later, LastStaffAt: &staff}
	if resolved.Waiting() {
		t.Fatal("a resolved thread is not waiting")
	}
}

func TestInboxUsesAMedianResponseNotAMean(t *testing.T) {
	seconds := func(v int) *int { return &v }
	member := at("09:00")
	conversations := []Conversation{
		{Status: ConversationOpen, FirstResponseSeconds: seconds(60)},
		{Status: ConversationOpen, FirstResponseSeconds: seconds(120)},
		{Status: ConversationResolved, FirstResponseSeconds: seconds(180)},
		// One thread answered a week late. A mean would make the whole desk
		// look broken; the median says what most people actually experienced.
		{Status: ConversationResolved, FirstResponseSeconds: seconds(604_800)},
		{Status: ConversationOpen, LastMemberAt: &member},
	}

	metrics := SummarizeInbox(conversations, at("09:10"))
	if metrics.MedianFirstResponseSeconds != 150 {
		t.Fatalf("the median of 60, 120, 180 and 604800 is 150, got %d",
			metrics.MedianFirstResponseSeconds)
	}
	if metrics.Waiting != 1 || metrics.Unassigned != 1 {
		t.Fatalf("one unanswered and unassigned thread, got %+v", metrics)
	}
	if metrics.LongestWaitSeconds != 600 {
		t.Fatalf("the oldest has waited ten minutes, got %d", metrics.LongestWaitSeconds)
	}
}

func TestHiddenReviewsAreNotInTheAverage(t *testing.T) {
	reply := "Thank you"
	reviews := []Review{
		{Rating: 5, Status: "PUBLISHED", Verified: true, Reply: &reply},
		{Rating: 4, Status: "PUBLISHED", Verified: true},
		{Rating: 2, Status: "PUBLISHED"},
		// An average that counts reviews nobody can see is an average of a
		// different set than the one on the page.
		{Rating: 1, Status: "HIDDEN"},
		{Rating: 1, Status: "FLAGGED"},
	}

	summary := SummarizeReviews(reviews)
	if summary.Count != 3 {
		t.Fatalf("three published reviews, got %d", summary.Count)
	}
	if summary.Average != 3.67 {
		t.Fatalf("5, 4 and 2 average 3.67, got %v", summary.Average)
	}
	if summary.Verified != 2 {
		t.Fatalf("two are backed by a visit, got %d", summary.Verified)
	}
	// The list somebody should actually work through: poorly rated and
	// unanswered.
	if summary.NegativeNew != 1 || summary.Unanswered != 2 {
		t.Fatalf("one unanswered complaint of two unanswered, got %+v", summary)
	}
}
