package reporting

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/modules/scheduling"
)

// The member app's home screen is a composite of several modules: the wallet
// for the balance and live promos, scheduling for today's classes, engagement
// for announcements. Assembling it here keeps that fan-out in the one module
// that is allowed to read across boundaries.

// MeView is what the app needs to render its chrome: who is signed in, what
// they can spend, and whether anything needs their attention.
type MeView struct {
	Member              domain.Member `json:"member"`
	Balance             int           `json:"balance"`
	ExpiringCredits     int           `json:"expiringCredits"`
	LowBalance          bool          `json:"lowBalance"`
	UnreadNotifications int           `json:"unreadNotifications"`
}

func (s *Service) Me(ctx context.Context, memberID string) (MeView, error) {
	member, err := s.members.Member(ctx, memberID)
	if err != nil {
		return MeView{}, err
	}
	balance, err := s.wallet.Balance(ctx, memberID)
	if err != nil {
		return MeView{}, err
	}
	expiring, err := s.wallet.ExpiringCredits(ctx, memberID)
	if err != nil {
		return MeView{}, err
	}
	rules, err := s.catalog.Rules(ctx)
	if err != nil {
		return MeView{}, err
	}

	unread := 0
	if s.notifications != nil {
		if unread, err = s.notifications.UnreadCount(ctx, memberID); err != nil {
			return MeView{}, err
		}
	}

	return MeView{
		Member:              member,
		Balance:             balance,
		ExpiringCredits:     expiring,
		LowBalance:          balance <= rules.LowBalanceThreshold,
		UnreadNotifications: unread,
	}, nil
}

// AnnouncementView is a studio message on the home feed.
type AnnouncementView struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	DeepLink  *string   `json:"deepLink"`
	ImageURL  *string   `json:"imageUrl"`
	CreatedAt time.Time `json:"createdAt"`
}

// PromoView is a live discount code, rendered as a card the member can tap
// straight through to checkout.
type PromoView struct {
	VoucherID      string    `json:"voucherId"`
	Code           string    `json:"code"`
	Label          string    `json:"label"`
	Description    string    `json:"description"`
	EndsAt         time.Time `json:"endsAt"`
	NewMembersOnly bool      `json:"newMembersOnly"`
}

// HomeView is the member app's landing screen.
type HomeView struct {
	Announcements []AnnouncementView `json:"announcements"`
	Promos        []PromoView        `json:"promos"`
	// RailDay says whether the class rail is showing today or, once today is
	// over, tomorrow.
	RailDay       string                      `json:"railDay"`
	TodaySessions []scheduling.SessionSummary `json:"todaySessions"`
	Challenge     *HomeChallenge              `json:"challenge"`
	SpotlightRace *HomeSpotlightRace          `json:"spotlightRace"`
}

type HomeChallenge struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	ProgressKm float64 `json:"progressKm"`
	TargetKm   float64 `json:"targetKm"`
}

type HomeSpotlightRace struct {
	RaceEventID string    `json:"raceEventId"`
	Name        string    `json:"name"`
	City        string    `json:"city"`
	ImageURL    *string   `json:"imageUrl"`
	StartsAt    time.Time `json:"startsAt"`
	DaysToRace  int       `json:"daysToRace"`
	Joined      bool      `json:"joined"`
	GoalSec     *int      `json:"goalSec"`
}

// Home assembles the landing screen.
//
// The rail falls forward to tomorrow once today's last class has finished, so
// a member opening the app late at night sees something they can still book
// rather than an empty day.
func (s *Service) Home(ctx context.Context, memberID string) (HomeView, error) {
	view := HomeView{
		Announcements: []AnnouncementView{},
		Promos:        []PromoView{},
		TodaySessions: []scheduling.SessionSummary{},
		RailDay:       "TODAY",
	}

	now := s.clock.Now()
	startOfDay := s.startOfToday()
	endOfDay := startOfDay.AddDate(0, 0, 1)

	sessions, err := s.bookableBetween(ctx, now, endOfDay, memberID)
	if err != nil {
		return view, err
	}
	if len(sessions) == 0 {
		tomorrowEnd := endOfDay.AddDate(0, 0, 1)
		sessions, err = s.bookableBetween(ctx, endOfDay, tomorrowEnd, memberID)
		if err != nil {
			return view, err
		}
		if len(sessions) > 0 {
			view.RailDay = "TOMORROW"
		}
	}
	if len(sessions) > 10 {
		sessions = sessions[:10]
	}
	view.TodaySessions = sessions

	vouchers, err := s.wallet.LiveVouchers(ctx)
	if err != nil {
		return view, err
	}
	for _, v := range vouchers {
		view.Promos = append(view.Promos, PromoView{
			VoucherID:      v.ID,
			Code:           v.Code,
			Label:          voucherLabel(v),
			Description:    fmt.Sprintf("Use code %s at checkout.", v.Code),
			EndsAt:         v.EndsAt,
			NewMembersOnly: v.EligibleSegment == domain.SegmentNewMembers,
		})
	}

	// Announcements come from sent campaigns. Until the engagement module is
	// mounted the feed is simply empty rather than an error.
	if s.announcements != nil {
		announcements, err := s.announcements.Recent(ctx, 5)
		if err != nil {
			return view, err
		}
		view.Announcements = announcements
	}
	return view, nil
}

// bookableBetween returns the classes a member could still take in a window.
func (s *Service) bookableBetween(ctx context.Context, from, to time.Time, memberID string) ([]scheduling.SessionSummary, error) {
	sessions, err := s.scheduling.Sessions(ctx, scheduling.SessionFilter{
		From: &from,
		To:   &to,
		Statuses: []domain.SessionStatus{
			domain.SessionPublished, domain.SessionFull,
		},
		Limit: 50,
	})
	if err != nil {
		return nil, err
	}
	// A class that has already finished is not something to offer.
	upcoming := make([]domain.ClassSession, 0, len(sessions))
	for _, session := range sessions {
		if session.EndsAt.After(s.clock.Now()) {
			upcoming = append(upcoming, session)
		}
	}
	sort.SliceStable(upcoming, func(i, j int) bool {
		return upcoming[i].StartsAt.Before(upcoming[j].StartsAt)
	})
	return s.scheduling.Summaries(ctx, upcoming, memberID)
}

func voucherLabel(v domain.Voucher) string {
	if v.Type == domain.VoucherPercent {
		return fmt.Sprintf("%d%% off", v.Value)
	}
	return fmt.Sprintf("Rp %s off", formatThousands(v.Value))
}

// formatThousands renders an IDR amount with dot separators, the way the
// apps display money.
func formatThousands(value int64) string {
	digits := fmt.Sprintf("%d", value)
	if len(digits) <= 3 {
		return digits
	}
	var out []byte
	for i, c := range []byte(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	return string(out)
}
