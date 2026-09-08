package inventory

import (
	"context"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Dated stock: what is on the shelf, when it goes off, and what it is worth.

// BatchView is a batch with the question a shop actually asks answered: how
// long has this got.
type BatchView struct {
	domain.Batch
	ItemName   string             `json:"itemName"`
	ItemSKU    string             `json:"itemSku"`
	BranchName string             `json:"branchName"`
	Unit       string             `json:"unit"`
	State      domain.ExpiryState `json:"state"`
	// DaysLeft is negative once a batch is past its date. Null when it has no
	// date at all, which is different from having none left.
	DaysLeft *int    `json:"daysLeft"`
	ValueIDR float64 `json:"valueIdr"`
}

// BatchQuery narrows the shelf.
type BatchQuery struct {
	ItemID   string
	BranchID string
	// State filters to NEAR or EXPIRED, which are the two the reorder desk
	// looks at. Empty returns everything still holding stock.
	State string
	Limit int
}

// Batches lists dated stock, judged against each item's own warning window.
func (s *Service) Batches(ctx context.Context, q BatchQuery) ([]BatchView, error) {
	batches, err := s.repo.Batches(ctx, BatchFilter{
		ItemID: q.ItemID, BranchID: q.BranchID, InStockOnly: true, Limit: q.Limit,
	})
	if err != nil {
		return nil, err
	}
	return s.viewBatches(ctx, batches, strings.ToUpper(q.State))
}

func (s *Service) viewBatches(ctx context.Context, batches []domain.Batch, state string) ([]BatchView, error) {
	items, err := s.repo.Items(ctx, ItemFilter{})
	if err != nil {
		return nil, err
	}
	byID := map[string]domain.InventoryItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	branches, err := s.branchNames(ctx)
	if err != nil {
		return nil, err
	}

	today := domain.DateOf(s.clock.Now(), s.studio)
	views := make([]BatchView, 0, len(batches))
	for _, batch := range batches {
		item := byID[batch.ItemID]
		warning := item.ExpiryWarningDays
		if warning <= 0 {
			warning = 30
		}
		view := BatchView{
			Batch: batch, ItemName: item.Name, ItemSKU: item.SKU, Unit: item.Unit,
			BranchName: branches[batch.BranchID],
			State:      batch.StateOn(today, warning),
			DaysLeft:   batch.DaysUntilExpiry(today),
			ValueIDR:   float64(batch.QtyOnHand) * batch.UnitCostIDR,
		}
		if state != "" && string(view.State) != state {
			continue
		}
		views = append(views, view)
	}
	return views, nil
}

// ExpiryReport is what a branch is about to lose, and the batches making it up.
type ExpiryReport struct {
	Summary domain.ExpirySummary `json:"summary"`
	// Near comes first and in date order, because it is the list somebody can
	// still do something about.
	Near    []BatchView `json:"near"`
	Expired []BatchView `json:"expired"`
}

func (s *Service) ExpiryReport(ctx context.Context, branchID string) (ExpiryReport, error) {
	batches, err := s.repo.Batches(ctx, BatchFilter{BranchID: branchID, InStockOnly: true})
	if err != nil {
		return ExpiryReport{}, err
	}
	warningDays, err := s.repo.WarningDaysOf(ctx)
	if err != nil {
		return ExpiryReport{}, err
	}
	views, err := s.viewBatches(ctx, batches, "")
	if err != nil {
		return ExpiryReport{}, err
	}

	report := ExpiryReport{
		Summary: domain.SummarizeExpiry(batches, domain.DateOf(s.clock.Now(), s.studio), warningDays),
		Near:    []BatchView{}, Expired: []BatchView{},
	}
	for _, view := range views {
		switch view.State {
		case domain.ExpiryNear:
			report.Near = append(report.Near, view)
		case domain.ExpiryExpired:
			report.Expired = append(report.Expired, view)
		}
	}
	return report, nil
}

// WriteOffExpired takes expired stock off the shelf.
//
// It is an ordinary adjustment with a reason, not a special kind of deletion:
// the goods really did leave, somebody really did decide, and a stock report a
// month later has to be able to say what happened and what it cost.
func (s *Service) WriteOffExpired(ctx context.Context, batchID string, actor Actor) (domain.StockMovement, error) {
	var movement domain.StockMovement
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		batches, err := s.repo.Batches(ctx, BatchFilter{Limit: 0})
		if err != nil {
			return err
		}
		var batch domain.Batch
		found := false
		for _, candidate := range batches {
			if candidate.ID == batchID {
				batch, found = candidate, true
				break
			}
		}
		if !found {
			return httpx.NotFound("batch")
		}
		if batch.QtyOnHand <= 0 {
			return httpx.Conflict("EMPTY_BATCH", "That batch is already empty.")
		}

		today := domain.DateOf(s.clock.Now(), s.studio)
		if batch.StateOn(today, 0) != domain.ExpiryExpired {
			return httpx.Conflict("NOT_EXPIRED",
				"That batch has not expired. Writing off good stock is an adjustment with a reason.")
		}

		reason := "Expired: batch " + batch.BatchCode
		// The movement takes the whole batch off the level, and names the
		// batch rather than letting FEFO choose — FEFO refuses expired stock
		// by design, which is exactly the stock being removed here.
		movement, err = s.Post(ctx, Movement{
			ItemID: batch.ItemID, BranchID: batch.BranchID, Kind: domain.MovementAdjustment,
			Qty: -batch.QtyOnHand, BatchID: batch.ID, Reason: &reason,
		}, actor)
		return err
	})
	if err != nil {
		return domain.StockMovement{}, err
	}
	s.record(ctx, "inventory.batch", batchID, "WRITE_OFF", actor, nil)
	return movement, nil
}
