package crm

import (
	"context"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// Badges, consent, the inbox and reviews.

// ── Badges ───────────────────────────────────────────────────────────────────

func (s *Service) Badges(ctx context.Context, activeOnly bool) ([]domain.Badge, error) {
	return s.repo.Badges(ctx, activeOnly)
}

// BadgeInput defines what earns a badge.
type BadgeInput struct {
	Code        string
	Name        string
	Description string
	Metric      domain.BadgeMetric
	Threshold   float64
	BonusXP     int
	Icon        *string
	SortOrder   int
	Active      bool
}

func (s *Service) SaveBadge(ctx context.Context, in BadgeInput, actor Actor) (domain.Badge, error) {
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" || strings.TrimSpace(in.Name) == "" {
		return domain.Badge{}, httpx.Invalid("A badge needs a code and a name.")
	}
	if !domain.IsValidBadgeMetric(string(in.Metric)) {
		return domain.Badge{}, httpx.Invalid("%q is not something a badge can measure.", in.Metric)
	}
	if in.Threshold < 0 {
		return domain.Badge{}, httpx.Invalid("A threshold cannot be negative.")
	}

	saved, err := s.repo.UpsertBadge(ctx, domain.Badge{
		ID: s.ids.New(id.Badge), Code: code, Name: strings.TrimSpace(in.Name),
		Description: in.Description, Metric: in.Metric, Threshold: in.Threshold,
		BonusXP: in.BonusXP, Icon: in.Icon, SortOrder: in.SortOrder, Active: in.Active,
	})
	if err != nil {
		return domain.Badge{}, err
	}
	s.record(ctx, "crm.badge", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

// MemberBadgeView is an earned badge with the badge itself, so a profile page
// does not have to join two lists.
type MemberBadgeView struct {
	domain.MemberBadge
	Badge domain.Badge `json:"badge"`
}

func (s *Service) MemberBadges(ctx context.Context, memberID string) ([]MemberBadgeView, error) {
	earned, err := s.repo.MemberBadges(ctx, memberID)
	if err != nil {
		return nil, err
	}
	badges, err := s.repo.Badges(ctx, false)
	if err != nil {
		return nil, err
	}
	byID := map[string]domain.Badge{}
	for _, badge := range badges {
		byID[badge.ID] = badge
	}

	views := make([]MemberBadgeView, 0, len(earned))
	for _, held := range earned {
		views = append(views, MemberBadgeView{MemberBadge: held, Badge: byID[held.BadgeID]})
	}
	return views, nil
}

// EvaluateBadges awards everything a member has newly qualified for.
//
// It runs after the things that move a counter — a visit, a sale — rather than
// on a schedule, so a member sees the badge when they earn it and not the next
// morning. Awarding is idempotent, so running it twice costs nothing.
func (s *Service) EvaluateBadges(ctx context.Context, memberID string, actor Actor) ([]domain.Badge, error) {
	var awarded []domain.Badge
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		badges, err := s.repo.Badges(ctx, true)
		if err != nil {
			return err
		}
		held, err := s.repo.MemberBadges(ctx, memberID)
		if err != nil {
			return err
		}
		have := map[string]bool{}
		for _, badge := range held {
			have[badge.BadgeID] = true
		}

		metrics, err := s.repo.MemberMetrics(ctx, memberID)
		if err != nil {
			return err
		}

		for _, badge := range domain.EarnedBadges(badges, metrics, have) {
			_, isNew, err := s.repo.AwardBadge(ctx, domain.MemberBadge{
				ID: s.ids.New(id.MemberBadge), MemberID: memberID, BadgeID: badge.ID,
				EarnedValue: metrics.Value(badge.Metric),
			})
			if err != nil {
				return err
			}
			if !isNew {
				continue
			}
			awarded = append(awarded, badge)

			// A badge worth points pays them through the ordinary ledger, so
			// the member's balance always equals the sum of its entries. The
			// key is the pair, so re-running the evaluation never pays twice.
			if badge.BonusXP > 0 {
				profile, err := s.profileFor(ctx, memberID, true)
				if err != nil {
					return err
				}
				if _, _, _, err := s.post(ctx, profile, badge.BonusXP, domain.XPEarn, postDetails{
					Channel: domain.XPFromManual, Type: "BADGE", SourceID: badge.ID,
					ReferenceType: "BADGE", ReferenceID: badge.ID,
					IdempotencyKey: "badge:" + memberID + ":" + badge.ID,
					Description:    "Badge: " + badge.Name,
				}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return awarded, nil
}

// AwardBadgeByHand is the manual path: the things no counter can see.
func (s *Service) AwardBadgeByHand(ctx context.Context, memberID, badgeID string,
	note *string, actor Actor) (domain.MemberBadge, error) {

	awarded, isNew, err := s.repo.AwardBadge(ctx, domain.MemberBadge{
		ID: s.ids.New(id.MemberBadge), MemberID: memberID, BadgeID: badgeID,
		AwardedBy: &actor.ID, Note: note,
	})
	if err != nil {
		return domain.MemberBadge{}, err
	}
	if !isNew {
		return domain.MemberBadge{}, httpx.Conflict("ALREADY_EARNED",
			"That member already has that badge.")
	}
	s.record(ctx, "crm.badge", badgeID, "AWARD", actor, note)
	return awarded, nil
}

func (s *Service) RevokeBadge(ctx context.Context, memberID, badgeID string, actor Actor) error {
	if err := s.repo.RevokeBadge(ctx, memberID, badgeID); err != nil {
		return err
	}
	s.record(ctx, "crm.badge", badgeID, "REVOKE", actor, nil)
	return nil
}

// ── Consent ──────────────────────────────────────────────────────────────────

func (s *Service) ContactPreferences(ctx context.Context, memberID string) ([]domain.ContactPreference, error) {
	return s.repo.ContactPreferences(ctx, memberID)
}

// PreferenceInput records what a member has said about being contacted.
type PreferenceInput struct {
	MemberID string
	Channel  domain.ContactChannel
	OptedIn  bool
	Reason   *string
	Scope    string
}

func (s *Service) SetContactPreference(ctx context.Context, in PreferenceInput, actor Actor) (domain.ContactPreference, error) {
	if !domain.IsValidContactChannel(string(in.Channel)) {
		return domain.ContactPreference{}, httpx.Invalid("%q is not a contact channel.", in.Channel)
	}
	scope := strings.ToUpper(strings.TrimSpace(in.Scope))
	if scope == "" {
		scope = "MARKETING"
	}
	if scope != "MARKETING" && scope != "ALL" {
		return domain.ContactPreference{}, httpx.Invalid(
			"A preference covers marketing or everything, not %q.", in.Scope)
	}

	saved, err := s.repo.SetContactPreference(ctx, domain.ContactPreference{
		MemberID: in.MemberID, Channel: in.Channel, OptedIn: in.OptedIn,
		Reason: in.Reason, Scope: scope, ChangedBy: &actor.ID,
	})
	if err != nil {
		return domain.ContactPreference{}, err
	}
	s.record(ctx, "crm.consent", in.MemberID, "SET", actor, in.Reason)
	return saved, nil
}

// ── Reviews ──────────────────────────────────────────────────────────────────

// ReviewView is a review with who left it.
type ReviewView struct {
	domain.Review
	MemberName string `json:"memberName"`
}

func (s *Service) Reviews(ctx context.Context, filter ReviewFilter) ([]ReviewView, error) {
	reviews, err := s.repo.Reviews(ctx, filter)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(reviews))
	for _, review := range reviews {
		ids = append(ids, review.MemberID)
	}
	members, err := s.members.MembersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	views := make([]ReviewView, 0, len(reviews))
	for _, review := range reviews {
		views = append(views, ReviewView{
			Review: review, MemberName: members[review.MemberID].FullName,
		})
	}
	return views, nil
}

// ReviewsFor is what one thing is rated, with the summary already computed.
type ReviewsFor struct {
	Summary domain.ReviewSummary `json:"summary"`
	Reviews []ReviewView         `json:"reviews"`
}

func (s *Service) ReviewsFor(ctx context.Context, subjectType, subjectID string) (ReviewsFor, error) {
	reviews, err := s.repo.Reviews(ctx, ReviewFilter{
		SubjectType: subjectType, SubjectID: subjectID, Limit: 500,
	})
	if err != nil {
		return ReviewsFor{}, err
	}
	views, err := s.Reviews(ctx, ReviewFilter{
		SubjectType: subjectType, SubjectID: subjectID, Limit: 500,
	})
	if err != nil {
		return ReviewsFor{}, err
	}
	return ReviewsFor{Summary: domain.SummarizeReviews(reviews), Reviews: views}, nil
}

// ReviewInput is what a member thought.
type ReviewInput struct {
	MemberID    string
	SubjectType string
	SubjectID   string
	Rating      int
	Comment     *string
}

// LeaveReview records an opinion.
//
// A review of a class the member did not attend is marked unverified rather
// than refused: they may have been there and the visit not logged, and
// throwing the opinion away would lose more than it protects. The flag lets a
// report weigh the two differently.
func (s *Service) LeaveReview(ctx context.Context, in ReviewInput, actor Actor) (domain.Review, error) {
	if !domain.IsValidReviewSubject(in.SubjectType) {
		return domain.Review{}, httpx.Invalid("%q is not something that can be reviewed.", in.SubjectType)
	}
	if in.Rating < 1 || in.Rating > 5 {
		return domain.Review{}, httpx.Invalid("A rating runs from one to five stars.")
	}

	verified, visitID, err := s.repo.VisitBehind(ctx, in.MemberID, in.SubjectType, in.SubjectID)
	if err != nil {
		return domain.Review{}, err
	}

	saved, err := s.repo.UpsertReview(ctx, domain.Review{
		ID: s.ids.New(id.Review), MemberID: in.MemberID, SubjectType: in.SubjectType,
		SubjectID: in.SubjectID, Rating: in.Rating, Comment: in.Comment,
		Verified: verified, VisitID: visitID, Status: "PUBLISHED",
	})
	if err != nil {
		return domain.Review{}, err
	}
	s.record(ctx, "crm.review", saved.ID, "LEAVE", actor, nil)
	return saved, nil
}

func (s *Service) ReplyToReview(ctx context.Context, reviewID, reply string, actor Actor) (domain.Review, error) {
	if strings.TrimSpace(reply) == "" {
		return domain.Review{}, httpx.Invalid("A reply needs something in it.")
	}
	saved, err := s.repo.ReplyToReview(ctx, reviewID, strings.TrimSpace(reply), actor.ID)
	if err != nil {
		return domain.Review{}, err
	}
	s.record(ctx, "crm.review", reviewID, "REPLY", actor, nil)
	return saved, nil
}

// SetReviewStatus hides or flags a review.
//
// Hiding is not deleting: the review stays, it leaves the average, and there
// is a record that somebody decided. A shop that can silently delete its bad
// reviews has a rating that means nothing.
func (s *Service) SetReviewStatus(ctx context.Context, reviewID, status string, actor Actor) (domain.Review, error) {
	saved, err := s.repo.SetReviewStatus(ctx, reviewID, strings.ToUpper(strings.TrimSpace(status)))
	if err != nil {
		return domain.Review{}, err
	}
	s.record(ctx, "crm.review", reviewID, "STATUS", actor, &status)
	return saved, nil
}
