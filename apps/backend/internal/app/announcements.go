package app

import (
	"context"

	"github.com/syabanf/nuhabit-backend/internal/modules/engagement"
	"github.com/syabanf/nuhabit-backend/internal/modules/reporting"
)

// announcementFeed turns sent campaigns into the home-screen announcements the
// member app renders. Reporting declares the shape it wants; engagement owns
// the rows; this adapter is the seam between them.
type announcementFeed struct {
	engagement *engagement.Service
}

func (a announcementFeed) Recent(ctx context.Context, limit int) ([]reporting.AnnouncementView, error) {
	campaigns, err := a.engagement.SentCampaigns(ctx, limit)
	if err != nil {
		return nil, err
	}
	views := make([]reporting.AnnouncementView, 0, len(campaigns))
	for _, c := range campaigns {
		views = append(views, reporting.AnnouncementView{
			ID:        c.ID,
			Title:     c.Name,
			Message:   c.Message,
			DeepLink:  c.DeepLink,
			ImageURL:  c.ImageURL,
			CreatedAt: c.CreatedAt,
		})
	}
	return views, nil
}
