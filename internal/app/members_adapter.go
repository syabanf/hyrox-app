package app

import (
	"context"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/modules/identity"
	"github.com/syabanf/nuhabit-backend/internal/modules/reporting"
)

// reportingMembers adapts the identity service to the shape reporting asked
// for. The translation is trivial today, and that is the point: reporting
// declares its own query type, so identity can change its filter without
// breaking a module that only wanted to list members.
type reportingMembers struct {
	identity *identity.Service
}

func (m reportingMembers) Member(ctx context.Context, id string) (domain.Member, error) {
	return m.identity.Member(ctx, id)
}

func (m reportingMembers) Members(ctx context.Context, query reporting.MemberQuery) ([]domain.Member, error) {
	return m.identity.Members(ctx, identity.MemberFilter{
		Query:  query.Query,
		Status: query.Status,
		Limit:  query.Limit,
		Offset: query.Offset,
	})
}

func (m reportingMembers) MembersByIDs(ctx context.Context, ids []string) (map[string]domain.Member, error) {
	return m.identity.MembersByIDs(ctx, ids)
}

func (m reportingMembers) MemberCounts(ctx context.Context) (map[string]int, error) {
	return m.identity.MemberCounts(ctx)
}
