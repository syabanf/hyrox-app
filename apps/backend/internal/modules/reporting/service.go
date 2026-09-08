// Package reporting answers the questions that span modules: the dashboard,
// the sales and visit reports, and the 360-degree view of one member.
//
// Every other module owns one bounded context and guards it. This one owns no
// tables at all: it reads through the same ports an external service would
// use, which is what keeps cross-module reads in a single, visible place
// instead of scattered joins nobody can untangle later.
package reporting

import (
	"context"
	"sort"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/modules/access"
	"github.com/syabanf/hyrox-app/apps/backend/internal/modules/scheduling"
	"github.com/syabanf/hyrox-app/apps/backend/internal/modules/wallet"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/clock"
)

// Ports onto the other modules. Each is the narrowest read surface this module
// needs, declared here by the consumer rather than exported by the producer.
type (
	Members interface {
		Member(ctx context.Context, id string) (domain.Member, error)
		Members(ctx context.Context, filter MemberQuery) ([]domain.Member, error)
		MembersByIDs(ctx context.Context, ids []string) (map[string]domain.Member, error)
		MemberCounts(ctx context.Context) (map[string]int, error)
	}

	Wallet interface {
		Balance(ctx context.Context, memberID string) (int, error)
		Balances(ctx context.Context, memberIDs []string) (map[string]int, error)
		OutstandingCredits(ctx context.Context) (int, error)
		ExpiringCredits(ctx context.Context, memberID string) (int, error)
		Entries(ctx context.Context, memberID string) ([]domain.CreditLedgerEntry, error)
		Lots(ctx context.Context, memberID string) ([]domain.TopUpLot, error)
		Payments(ctx context.Context, filter wallet.PaymentFilter) ([]domain.Payment, error)
		PackageSales(ctx context.Context) (map[string]wallet.PackageSale, error)
		LiveVouchers(ctx context.Context) ([]domain.Voucher, error)
	}

	Scheduling interface {
		Sessions(ctx context.Context, filter scheduling.SessionFilter) ([]domain.ClassSession, error)
		Summaries(ctx context.Context, sessions []domain.ClassSession, memberID string) ([]scheduling.SessionSummary, error)
		MemberBookings(ctx context.Context, memberID string, limit int) ([]domain.Booking, error)
		BookingSummaries(ctx context.Context, bookings []domain.Booking) ([]scheduling.BookingSummary, error)
		BookingsForSessions(ctx context.Context, sessionIDs []string) (map[string][]domain.Booking, error)
	}

	Access interface {
		CountToday(ctx context.Context, since time.Time) (int, error)
		DailyVisits(ctx context.Context, since time.Time) (map[string]int, int, int, error)
		VisitCounts(ctx context.Context, memberIDs []string) (map[string]access.VisitSummary, error)
		MemberVisits(ctx context.Context, memberID string, limit int) ([]access.LogView, error)
	}

	Catalog interface {
		Branches(ctx context.Context) ([]domain.Branch, error)
		ClassTypes(ctx context.Context, activeOnly bool) ([]domain.ClassType, error)
		Packages(ctx context.Context, activeOnly bool) ([]domain.CreditPackage, error)
		Rules(ctx context.Context) (domain.BusinessRules, error)
	}

	Incentives interface {
		PayableIDR(ctx context.Context, periodMonth string) (int64, error)
	}

	AuditLog interface {
		List(ctx context.Context, limit int) ([]domain.AuditEvent, error)
	}

	// Notifications and Announcements are optional: when the engagement
	// module is not mounted the home feed is simply quiet rather than broken.
	Notifications interface {
		UnreadCount(ctx context.Context, memberID string) (int, error)
	}

	Announcements interface {
		Recent(ctx context.Context, limit int) ([]AnnouncementView, error)
	}
)

// MemberQuery mirrors the identity module's filter without importing it, so
// reporting stays decoupled from that module's internals.
type MemberQuery struct {
	Query  string
	Status string
	Limit  int
	Offset int
}

// Deps are the ports this module reads through. Incentives, Notifications and
// Announcements may be nil when those modules are not mounted.
type Deps struct {
	Members       Members
	Wallet        Wallet
	Scheduling    Scheduling
	Access        Access
	Catalog       Catalog
	Incentives    Incentives
	Notifications Notifications
	Announcements Announcements
	Audit         AuditLog
	Clock         clock.Clock
	// Studio is the timezone "today" is measured in. A Jakarta studio's day
	// does not start at midnight UTC.
	Studio *time.Location
}

// Service builds the cross-module read models.
type Service struct {
	members       Members
	wallet        Wallet
	scheduling    Scheduling
	access        Access
	catalog       Catalog
	incentives    Incentives
	notifications Notifications
	announcements Announcements
	audit         AuditLog
	clock         clock.Clock
	studio        *time.Location
}

func NewService(deps Deps) *Service {
	studio := deps.Studio
	if studio == nil {
		studio = time.UTC
	}
	return &Service{
		members: deps.Members, wallet: deps.Wallet, scheduling: deps.Scheduling,
		access: deps.Access, catalog: deps.Catalog, incentives: deps.Incentives,
		notifications: deps.Notifications, announcements: deps.Announcements,
		audit: deps.Audit, clock: deps.Clock, studio: studio,
	}
}

// startOfToday is the studio's local midnight, expressed as an instant.
func (s *Service) startOfToday() time.Time {
	local := s.clock.Now().In(s.studio)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, s.studio)
}

// ── Dashboard ────────────────────────────────────────────────────────────────

// DashboardView is the admin landing screen.
type DashboardView struct {
	VisitorsToday          int                         `json:"visitorsToday"`
	ClassesToday           int                         `json:"classesToday"`
	RevenueTodayIDR        int64                       `json:"revenueTodayIdr"`
	TopUpsTodayIDR         int64                       `json:"topUpsTodayIdr"`
	OutstandingCredits     int                         `json:"outstandingCredits"`
	ExpiringCredits        int                         `json:"expiringCredits"`
	ActiveMembers          int                         `json:"activeMembers"`
	CoachIncentivesPayable int64                       `json:"coachIncentivesPayableIdr"`
	PendingPayments        int                         `json:"pendingPayments"`
	TodaySessions          []scheduling.SessionSummary `json:"todaySessions"`
}

func (s *Service) Dashboard(ctx context.Context) (DashboardView, error) {
	var view DashboardView
	startOfDay := s.startOfToday()
	endOfDay := startOfDay.AddDate(0, 0, 1)

	visitors, err := s.access.CountToday(ctx, startOfDay)
	if err != nil {
		return view, err
	}
	view.VisitorsToday = visitors

	sessions, err := s.scheduling.Sessions(ctx, scheduling.SessionFilter{From: &startOfDay, To: &endOfDay})
	if err != nil {
		return view, err
	}
	view.ClassesToday = len(sessions)
	if view.TodaySessions, err = s.scheduling.Summaries(ctx, sessions, ""); err != nil {
		return view, err
	}

	payments, err := s.wallet.Payments(ctx, wallet.PaymentFilter{Limit: 1000})
	if err != nil {
		return view, err
	}
	for _, p := range payments {
		if p.Status == domain.PaymentPending {
			view.PendingPayments++
		}
		// Revenue is counted on settlement date, not creation date: an invoice
		// raised yesterday and paid today is today's money.
		if p.Status == domain.PaymentPaid && p.PaidAt != nil && !p.PaidAt.Before(startOfDay) {
			view.RevenueTodayIDR += p.TotalIDR
			view.TopUpsTodayIDR += p.TotalIDR
		}
	}

	if view.OutstandingCredits, err = s.wallet.OutstandingCredits(ctx); err != nil {
		return view, err
	}

	counts, err := s.members.MemberCounts(ctx)
	if err != nil {
		return view, err
	}
	view.ActiveMembers = counts[string(domain.MemberActive)]

	expiring, err := s.expiringAcrossMembers(ctx)
	if err != nil {
		return view, err
	}
	view.ExpiringCredits = expiring

	if s.incentives != nil {
		payable, err := s.incentives.PayableIDR(ctx, domain.PeriodMonthOf(s.clock.Now(), s.studio))
		if err != nil {
			return view, err
		}
		view.CoachIncentivesPayable = payable
	}
	return view, nil
}

// expiringAcrossMembers totals what the studio is about to stop owing. It walks
// active members rather than the whole ledger, which keeps it bounded.
func (s *Service) expiringAcrossMembers(ctx context.Context) (int, error) {
	members, err := s.members.Members(ctx, MemberQuery{Status: string(domain.MemberActive), Limit: 500})
	if err != nil {
		return 0, err
	}
	total := 0
	for _, m := range members {
		expiring, err := s.wallet.ExpiringCredits(ctx, m.ID)
		if err != nil {
			return 0, err
		}
		total += expiring
	}
	return total, nil
}

// ── Reports ──────────────────────────────────────────────────────────────────

// DailyPoint is one bar in a trend chart.
type DailyPoint struct {
	Date  string `json:"date"`
	Value int64  `json:"value"`
}

// SalesReport is money in over a window.
type SalesReport struct {
	TotalIDR   int64            `json:"totalIdr"`
	ByDay      []DailyPoint     `json:"byDay"`
	ByChannel  []ChannelTotal   `json:"byChannel"`
	ByPackage  []PackageRevenue `json:"byPackage"`
	RefundsIDR int64            `json:"refundsIdr"`
}

type ChannelTotal struct {
	Channel  string `json:"channel"`
	TotalIDR int64  `json:"totalIdr"`
}

type PackageRevenue struct {
	PackageID     string `json:"packageId"`
	PackageName   string `json:"packageName"`
	PurchaseCount int    `json:"purchaseCount"`
	RevenueIDR    int64  `json:"revenueIdr"`
}

func (s *Service) Sales(ctx context.Context, days int) (SalesReport, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	since := s.startOfToday().AddDate(0, 0, -(days - 1))

	payments, err := s.wallet.Payments(ctx, wallet.PaymentFilter{Limit: 5000})
	if err != nil {
		return SalesReport{}, err
	}

	report := SalesReport{ByDay: emptySeries(since, days, s.studio)}
	index := map[string]int{}
	for i, point := range report.ByDay {
		index[point.Date] = i
	}
	byChannel := map[string]int64{}

	for _, p := range payments {
		if p.PaidAt == nil || p.PaidAt.Before(since) {
			continue
		}
		switch p.Status {
		case domain.PaymentPaid:
			report.TotalIDR += p.TotalIDR
			byChannel[string(p.Channel)] += p.TotalIDR
			if i, ok := index[p.PaidAt.In(s.studio).Format("2006-01-02")]; ok {
				report.ByDay[i].Value += p.TotalIDR
			}
		case domain.PaymentRefunded:
			// A refunded payment is money that came in and went back out; it
			// belongs in the report, but not in the total.
			report.RefundsIDR += p.TotalIDR
		}
	}

	for channel, total := range byChannel {
		report.ByChannel = append(report.ByChannel, ChannelTotal{Channel: channel, TotalIDR: total})
	}
	sort.Slice(report.ByChannel, func(i, j int) bool {
		return report.ByChannel[i].TotalIDR > report.ByChannel[j].TotalIDR
	})

	sales, err := s.wallet.PackageSales(ctx)
	if err != nil {
		return SalesReport{}, err
	}
	packages, err := s.catalog.Packages(ctx, false)
	if err != nil {
		return SalesReport{}, err
	}
	for _, p := range packages {
		sale := sales[p.ID]
		report.ByPackage = append(report.ByPackage, PackageRevenue{
			PackageID: p.ID, PackageName: p.Name,
			PurchaseCount: sale.PurchaseCount, RevenueIDR: sale.RevenueIDR,
		})
	}
	sort.Slice(report.ByPackage, func(i, j int) bool {
		return report.ByPackage[i].RevenueIDR > report.ByPackage[j].RevenueIDR
	})
	return report, nil
}

// VisitsReport is footfall over a window.
type VisitsReport struct {
	Total   int          `json:"total"`
	ByDay   []DailyPoint `json:"byDay"`
	Denied  int          `json:"denied"`
	Offline int          `json:"offline"`
}

func (s *Service) Visits(ctx context.Context, days int) (VisitsReport, error) {
	if days <= 0 || days > 365 {
		days = 30
	}
	since := s.startOfToday().AddDate(0, 0, -(days - 1))

	byDay, denied, offline, err := s.access.DailyVisits(ctx, since)
	if err != nil {
		return VisitsReport{}, err
	}

	report := VisitsReport{ByDay: emptySeries(since, days, s.studio), Denied: denied, Offline: offline}
	for i, point := range report.ByDay {
		if value, ok := byDay[point.Date]; ok {
			report.ByDay[i].Value = int64(value)
			report.Total += value
		}
	}
	return report, nil
}

// CreditsReport is the studio's outstanding credit liability.
type CreditsReport struct {
	OutstandingTotal int                  `json:"outstandingTotal"`
	ExpiringTotal    int                  `json:"expiringTotal"`
	PerMember        []MemberCreditsEntry `json:"perMember"`
}

type MemberCreditsEntry struct {
	MemberID   string `json:"memberId"`
	MemberName string `json:"memberName"`
	Balance    int    `json:"balance"`
	Expiring   int    `json:"expiring"`
}

func (s *Service) Credits(ctx context.Context) (CreditsReport, error) {
	members, err := s.members.Members(ctx, MemberQuery{Limit: 1000})
	if err != nil {
		return CreditsReport{}, err
	}
	ids := make([]string, 0, len(members))
	for _, m := range members {
		if m.Status != domain.MemberArchived {
			ids = append(ids, m.ID)
		}
	}
	balances, err := s.wallet.Balances(ctx, ids)
	if err != nil {
		return CreditsReport{}, err
	}

	report := CreditsReport{PerMember: []MemberCreditsEntry{}}
	for _, m := range members {
		balance := balances[m.ID]
		if balance == 0 {
			continue
		}
		expiring, err := s.wallet.ExpiringCredits(ctx, m.ID)
		if err != nil {
			return CreditsReport{}, err
		}
		report.PerMember = append(report.PerMember, MemberCreditsEntry{
			MemberID: m.ID, MemberName: m.FullName, Balance: balance, Expiring: expiring,
		})
		if balance > 0 {
			report.OutstandingTotal += balance
		}
		report.ExpiringTotal += expiring
	}
	sort.Slice(report.PerMember, func(i, j int) bool {
		return report.PerMember[i].Balance > report.PerMember[j].Balance
	})
	return report, nil
}

// ClassesReport is attendance quality per class type.
type ClassesReport struct {
	PerType       []ClassTypeStats `json:"perType"`
	RecentNoShows []NoShowEntry    `json:"recentNoShows"`
}

type ClassTypeStats struct {
	ClassTypeID    string  `json:"classTypeId"`
	ClassTypeName  string  `json:"classTypeName"`
	SessionsHeld   int     `json:"sessionsHeld"`
	Booked         int     `json:"booked"`
	Attended       int     `json:"attended"`
	NoShows        int     `json:"noShows"`
	AttendanceRate float64 `json:"attendanceRate"`
}

type NoShowEntry struct {
	MemberName    string    `json:"memberName"`
	ClassTypeName string    `json:"classTypeName"`
	StartsAt      time.Time `json:"startsAt"`
}

func (s *Service) Classes(ctx context.Context, days int) (ClassesReport, error) {
	if days <= 0 || days > 365 {
		days = 90
	}
	since := s.startOfToday().AddDate(0, 0, -days)

	sessions, err := s.scheduling.Sessions(ctx, scheduling.SessionFilter{
		From:     &since,
		Statuses: []domain.SessionStatus{domain.SessionCompleted},
		Limit:    2000,
	})
	if err != nil {
		return ClassesReport{}, err
	}
	sessionIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		sessionIDs = append(sessionIDs, session.ID)
	}
	bookings, err := s.scheduling.BookingsForSessions(ctx, sessionIDs)
	if err != nil {
		return ClassesReport{}, err
	}
	classTypes, err := s.catalog.ClassTypes(ctx, false)
	if err != nil {
		return ClassesReport{}, err
	}
	names := map[string]string{}
	for _, t := range classTypes {
		names[t.ID] = t.Name
	}

	stats := map[string]*ClassTypeStats{}
	var noShowBookings []domain.Booking
	noShowSession := map[string]domain.ClassSession{}

	for _, session := range sessions {
		entry, ok := stats[session.ClassTypeID]
		if !ok {
			entry = &ClassTypeStats{
				ClassTypeID:   session.ClassTypeID,
				ClassTypeName: names[session.ClassTypeID],
			}
			stats[session.ClassTypeID] = entry
		}
		entry.SessionsHeld++
		for _, b := range bookings[session.ID] {
			switch b.Status {
			case domain.BookingCheckedIn, domain.BookingCompleted:
				entry.Booked++
				entry.Attended++
			case domain.BookingNoShow:
				entry.Booked++
				entry.NoShows++
				noShowBookings = append(noShowBookings, b)
				noShowSession[b.ID] = session
			case domain.BookingConfirmed:
				entry.Booked++
			}
		}
	}

	report := ClassesReport{PerType: []ClassTypeStats{}, RecentNoShows: []NoShowEntry{}}
	for _, entry := range stats {
		if entry.Booked > 0 {
			entry.AttendanceRate = float64(entry.Attended) / float64(entry.Booked)
		}
		report.PerType = append(report.PerType, *entry)
	}
	sort.Slice(report.PerType, func(i, j int) bool {
		return report.PerType[i].SessionsHeld > report.PerType[j].SessionsHeld
	})

	sort.Slice(noShowBookings, func(i, j int) bool {
		return noShowBookings[i].UpdatedAt.After(noShowBookings[j].UpdatedAt)
	})
	if len(noShowBookings) > 15 {
		noShowBookings = noShowBookings[:15]
	}
	memberIDs := make([]string, 0, len(noShowBookings))
	for _, b := range noShowBookings {
		memberIDs = append(memberIDs, b.MemberID)
	}
	members, err := s.members.MembersByIDs(ctx, memberIDs)
	if err != nil {
		return ClassesReport{}, err
	}
	for _, b := range noShowBookings {
		session := noShowSession[b.ID]
		report.RecentNoShows = append(report.RecentNoShows, NoShowEntry{
			MemberName:    members[b.MemberID].FullName,
			ClassTypeName: names[session.ClassTypeID],
			StartsAt:      session.StartsAt,
		})
	}
	return report, nil
}

// emptySeries builds a zero-filled day series, so a chart shows quiet days as
// gaps at zero rather than skipping them.
func emptySeries(since time.Time, days int, loc *time.Location) []DailyPoint {
	points := make([]DailyPoint, 0, days)
	for i := 0; i < days; i++ {
		points = append(points, DailyPoint{Date: since.AddDate(0, 0, i).In(loc).Format("2006-01-02")})
	}
	return points
}

// ── Members ──────────────────────────────────────────────────────────────────

// MemberSummary is one row of the admin member list.
type MemberSummary struct {
	Member      domain.Member `json:"member"`
	Balance     int           `json:"balance"`
	TotalVisits int           `json:"totalVisits"`
	LastVisitAt *time.Time    `json:"lastVisitAt"`
}

func (s *Service) MemberList(ctx context.Context, query MemberQuery) ([]MemberSummary, error) {
	members, err := s.members.Members(ctx, query)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.ID)
	}
	balances, err := s.wallet.Balances(ctx, ids)
	if err != nil {
		return nil, err
	}
	visits, err := s.access.VisitCounts(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make([]MemberSummary, 0, len(members))
	for _, m := range members {
		visit := visits[m.ID]
		out = append(out, MemberSummary{
			Member: m, Balance: balances[m.ID],
			TotalVisits: visit.Total, LastVisitAt: visit.LastVisitAt,
		})
	}
	return out, nil
}

// MemberDetail is the 360-degree view: everything the studio knows about one
// member, gathered from every module.
type MemberDetail struct {
	Member           domain.Member               `json:"member"`
	Balance          int                         `json:"balance"`
	ExpiringCredits  int                         `json:"expiringCredits"`
	TotalVisits      int                         `json:"totalVisits"`
	LastVisitAt      *time.Time                  `json:"lastVisitAt"`
	UpcomingBookings []scheduling.BookingSummary `json:"upcomingBookings"`
	Bookings         []scheduling.BookingSummary `json:"bookings"`
	Entries          []domain.CreditLedgerEntry  `json:"entries"`
	Lots             []domain.TopUpLot           `json:"lots"`
	Visits           []access.LogView            `json:"visits"`
	Payments         []domain.Payment            `json:"payments"`
}

func (s *Service) MemberDetail(ctx context.Context, memberID string) (MemberDetail, error) {
	var detail MemberDetail

	member, err := s.members.Member(ctx, memberID)
	if err != nil {
		return detail, err
	}
	detail.Member = member

	if detail.Balance, err = s.wallet.Balance(ctx, memberID); err != nil {
		return detail, err
	}
	if detail.ExpiringCredits, err = s.wallet.ExpiringCredits(ctx, memberID); err != nil {
		return detail, err
	}
	if detail.Entries, err = s.wallet.Entries(ctx, memberID); err != nil {
		return detail, err
	}
	if detail.Lots, err = s.wallet.Lots(ctx, memberID); err != nil {
		return detail, err
	}
	if detail.Payments, err = s.wallet.Payments(ctx, wallet.PaymentFilter{MemberID: memberID, Limit: 100}); err != nil {
		return detail, err
	}
	if detail.Visits, err = s.access.MemberVisits(ctx, memberID, 50); err != nil {
		return detail, err
	}

	bookings, err := s.scheduling.MemberBookings(ctx, memberID, 100)
	if err != nil {
		return detail, err
	}
	if detail.Bookings, err = s.scheduling.BookingSummaries(ctx, bookings); err != nil {
		return detail, err
	}

	now := s.clock.Now()
	detail.UpcomingBookings = []scheduling.BookingSummary{}
	for _, b := range detail.Bookings {
		if b.Session.StartsAt.After(now) && b.Booking.IsActive() {
			detail.UpcomingBookings = append(detail.UpcomingBookings, b)
		}
	}
	sort.Slice(detail.UpcomingBookings, func(i, j int) bool {
		return detail.UpcomingBookings[i].Session.StartsAt.Before(detail.UpcomingBookings[j].Session.StartsAt)
	})

	visits, err := s.access.VisitCounts(ctx, []string{memberID})
	if err != nil {
		return detail, err
	}
	detail.TotalVisits = visits[memberID].Total
	detail.LastVisitAt = visits[memberID].LastVisitAt
	return detail, nil
}

// Audit returns the recent audit trail.
func (s *Service) Audit(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	return s.audit.List(ctx, limit)
}
