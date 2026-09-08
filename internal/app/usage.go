package app

import (
	"context"

	"github.com/syabanf/nuhabit-backend/internal/modules/catalog"
	"github.com/syabanf/nuhabit-backend/internal/modules/scheduling"
	"github.com/syabanf/nuhabit-backend/internal/modules/wallet"
)

// catalogUsage answers catalog's questions about other modules: may this row
// be deleted, and how has this package sold.
//
// This adapter is the seam. Today it calls the scheduling and wallet services
// in-process; when those become their own services it becomes an HTTP or gRPC
// client and nothing in the catalog module changes.
type catalogUsage struct {
	scheduling *scheduling.Service
	wallet     *wallet.Service
}

func (u catalogUsage) SessionsForClassType(ctx context.Context, classTypeID string) (int, error) {
	return u.scheduling.CountSessionsForClassType(ctx, classTypeID)
}

func (u catalogUsage) UpcomingSessionsForCoach(ctx context.Context, coachID string) (int, error) {
	return u.scheduling.CountUpcomingForCoach(ctx, coachID)
}

func (u catalogUsage) PaymentsForPackage(ctx context.Context, packageID string) (int, error) {
	return u.wallet.CountPaymentsForPackage(ctx, packageID)
}

func (u catalogUsage) PackageStats(ctx context.Context) (map[string]catalog.PackageStat, error) {
	sales, err := u.wallet.PackageSales(ctx)
	if err != nil {
		return nil, err
	}
	stats := make(map[string]catalog.PackageStat, len(sales))
	for packageID, sale := range sales {
		stats[packageID] = catalog.PackageStat{
			PurchaseCount: sale.PurchaseCount,
			RevenueIDR:    sale.RevenueIDR,
		}
	}
	return stats, nil
}
