package app

import (
	"context"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/modules/access"
	"github.com/syabanf/hyrox-app/apps/backend/internal/modules/catalog"
	"github.com/syabanf/hyrox-app/apps/backend/internal/modules/engagement"
	"github.com/syabanf/hyrox-app/apps/backend/internal/modules/identity"
	"github.com/syabanf/hyrox-app/apps/backend/internal/modules/training"
	"github.com/syabanf/hyrox-app/apps/backend/internal/modules/wallet"
)

// Adapters for the athlete side of the app. Training speaks in Athletes and a
// Library; identity and catalog speak in Members and Exercises. This is the
// only file where the two meet.

// trainingMembers turns identity's members into the athletes the training
// module names.
type trainingMembers struct{ identity *identity.Service }

func (a trainingMembers) Athletes(ctx context.Context, ids []string) (map[string]training.Athlete, error) {
	members, err := a.identity.MembersByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[string]training.Athlete, len(members))
	for id, member := range members {
		out[id] = training.Athlete{
			ID: member.ID, Name: member.FullName, AvatarURL: member.AvatarURL,
			Active: member.Status == domain.MemberActive,
		}
	}
	return out, nil
}

// ActiveAthletes is who the follow suggestions are drawn from. Archived and
// suspended memberships are not people to go and follow.
func (a trainingMembers) ActiveAthletes(ctx context.Context) ([]training.Athlete, error) {
	members, err := a.identity.Members(ctx, identity.MemberFilter{
		Status: string(domain.MemberActive), Limit: suggestionPool,
	})
	if err != nil {
		return nil, err
	}
	out := make([]training.Athlete, 0, len(members))
	for _, member := range members {
		out = append(out, training.Athlete{
			ID: member.ID, Name: member.FullName, AvatarURL: member.AvatarURL, Active: true,
		})
	}
	return out, nil
}

// suggestionPool is how many members the follow suggestions are drawn from.
// The screen shows a handful; this is the pool they are filtered out of.
const suggestionPool = 200

// trainingLibrary is the exercise catalogue the workout generator draws on.
type trainingLibrary struct{ catalog *catalog.Service }

func (a trainingLibrary) Exercises(ctx context.Context) ([]domain.Exercise, error) {
	return a.catalog.Exercises(ctx)
}

func (a trainingLibrary) Substitutions(ctx context.Context) ([]domain.SubstitutionRule, error) {
	return a.catalog.Substitutions(ctx)
}

// trainingChallenges is engagement's challenges, as the athlete tab reads them.
type trainingChallenges struct{ engagement *engagement.Service }

func (a trainingChallenges) Running(ctx context.Context, now time.Time) ([]domain.Challenge, error) {
	return a.engagement.Running(ctx, now)
}

func (a trainingChallenges) Participants(ctx context.Context) (map[string][]string, error) {
	return a.engagement.Participants(ctx)
}

func (a trainingChallenges) Join(ctx context.Context, challengeID, memberID string) error {
	return a.engagement.Join(ctx, challengeID, memberID)
}

// ── Campaign audiences ───────────────────────────────────────────────────────

// campaignAudience resolves a segment to the members in it.
//
// Every segment is a question about somebody's wallet, their visits or their
// join date, and each of those belongs to a different module. Engagement asks
// the question; this is where it gets answered.
type campaignAudience struct {
	identity *identity.Service
	wallet   *wallet.Service
	access   *access.Service
	rules    *catalog.Service
	now      func() time.Time
}

// noVisitDays is the window the NO_VISIT_14D segment names.
const noVisitDays = 14

// newMemberDays is how recently somebody joined to count as new.
const newMemberDays = 30

func (a campaignAudience) Resolve(ctx context.Context, segment domain.MemberSegment, filter *domain.SegmentFilter) ([]engagement.AudienceMember, error) {
	// Every segment starts from the active membership: a campaign is never
	// sent to somebody who has left.
	members, err := a.identity.Members(ctx, identity.MemberFilter{
		Status: string(domain.MemberActive), Limit: audienceLimit,
	})
	if err != nil {
		return nil, err
	}
	if filter != nil && filter.BranchID != nil {
		kept := members[:0]
		for _, m := range members {
			if m.PreferredBranchID != nil && *m.PreferredBranchID == *filter.BranchID {
				kept = append(kept, m)
			}
		}
		members = kept
	}

	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.ID)
	}

	now := a.now()
	keep := func(domain.Member) bool { return true }

	switch segment {
	case domain.SegmentAllActive:
		// Everybody active, which is what the default already is.

	case domain.SegmentLowBalance:
		rules, err := a.rules.Rules(ctx)
		if err != nil {
			return nil, err
		}
		balances, err := a.wallet.Balances(ctx, ids)
		if err != nil {
			return nil, err
		}
		keep = func(m domain.Member) bool { return balances[m.ID] <= rules.LowBalanceThreshold }

	case domain.SegmentExpiringCredits:
		// One query per member, which is why the audience is capped: this is
		// the expensive segment, and it is also the smallest.
		expiring := map[string]int{}
		for _, id := range ids {
			credits, err := a.wallet.ExpiringCredits(ctx, id)
			if err != nil {
				return nil, err
			}
			expiring[id] = credits
		}
		keep = func(m domain.Member) bool { return expiring[m.ID] > 0 }

	case domain.SegmentNewMembersOnly:
		cutoff := now.AddDate(0, 0, -newMemberDays)
		keep = func(m domain.Member) bool { return m.CreatedAt.After(cutoff) }

	case domain.SegmentNoVisit14D:
		visits, err := a.access.VisitCounts(ctx, ids)
		if err != nil {
			return nil, err
		}
		cutoff := now.AddDate(0, 0, -noVisitDays)
		keep = func(m domain.Member) bool {
			summary := visits[m.ID]
			return summary.LastVisitAt == nil || summary.LastVisitAt.Before(cutoff)
		}

	case domain.SegmentCustom:
		keep, err = a.customFilter(ctx, ids, filter, now)
		if err != nil {
			return nil, err
		}
	}

	out := []engagement.AudienceMember{}
	for _, m := range members {
		if keep(m) {
			out = append(out, engagement.AudienceMember{ID: m.ID, Name: m.FullName})
		}
	}
	return out, nil
}

// audienceLimit caps a send. A studio with more members than this needs a
// queue rather than a request that runs for a minute.
const audienceLimit = 5000

// customFilter builds the predicate for a hand-rolled audience. The conditions
// combine with AND, which is what the panel's builder shows.
func (a campaignAudience) customFilter(ctx context.Context, ids []string, filter *domain.SegmentFilter, now time.Time) (func(domain.Member) bool, error) {
	if filter == nil {
		return func(domain.Member) bool { return true }, nil
	}

	var balances map[string]int
	if filter.MaxBalance != nil {
		loaded, err := a.wallet.Balances(ctx, ids)
		if err != nil {
			return nil, err
		}
		balances = loaded
	}
	var visits map[string]access.VisitSummary
	if filter.MinDaysSinceLastVisit != nil {
		loaded, err := a.access.VisitCounts(ctx, ids)
		if err != nil {
			return nil, err
		}
		visits = loaded
	}

	return func(m domain.Member) bool {
		if filter.MaxBalance != nil && balances[m.ID] > *filter.MaxBalance {
			return false
		}
		if filter.JoinedWithinDays != nil {
			if m.CreatedAt.Before(now.AddDate(0, 0, -*filter.JoinedWithinDays)) {
				return false
			}
		}
		if filter.MinDaysSinceLastVisit != nil {
			cutoff := now.AddDate(0, 0, -*filter.MinDaysSinceLastVisit)
			// Somebody who has never visited satisfies "not for N days".
			if last := visits[m.ID].LastVisitAt; last != nil && last.After(cutoff) {
				return false
			}
		}
		return true
	}, nil
}
