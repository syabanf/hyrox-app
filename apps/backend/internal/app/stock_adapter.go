package app

import (
	"context"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/modules/inventory"
	"github.com/syabanf/nuhabit-backend/internal/modules/purchasing"
)

// purchasingStock satisfies the port purchasing declares, in terms of the
// inventory service.
//
// This is one of the handful of adapters that exist so no module imports
// another's shapes: purchasing speaks its own StockRef and StockActor, and
// this is the only place those become inventory's. When inventory moves into
// its own service, this file is what becomes an HTTP client.
type purchasingStock struct{ inventory *inventory.Service }

func (a purchasingStock) Receive(ctx context.Context, itemID, branchID string,
	qty domain.Quantity, unitCostIDR float64, ref purchasing.StockRef, actor purchasing.StockActor) error {
	_, err := a.inventory.Receive(ctx, itemID, branchID, qty, unitCostIDR,
		inventory.Reference{Type: ref.Type, ID: ref.ID, Number: ref.Number},
		inventory.Actor{ID: actor.ID, Name: actor.Name})
	return err
}

func (a purchasingStock) Return(ctx context.Context, itemID, branchID string,
	qty domain.Quantity, ref purchasing.StockRef, actor purchasing.StockActor) error {
	_, err := a.inventory.Issue(ctx, itemID, branchID, qty, domain.MovementReturn,
		inventory.Reference{Type: ref.Type, ID: ref.ID, Number: ref.Number},
		inventory.Actor{ID: actor.ID, Name: actor.Name})
	return err
}

func (a purchasingStock) ReserveOnOrder(ctx context.Context, itemID, branchID string, qty domain.Quantity) error {
	return a.inventory.ReserveOnOrder(ctx, itemID, branchID, qty)
}

func (a purchasingStock) ReleaseOnOrder(ctx context.Context, itemID, branchID string, qty domain.Quantity) error {
	return a.inventory.ReleaseOnOrder(ctx, itemID, branchID, qty)
}

func (a purchasingStock) ItemNames(ctx context.Context) (map[string]string, error) {
	return a.inventory.ItemNames(ctx)
}
