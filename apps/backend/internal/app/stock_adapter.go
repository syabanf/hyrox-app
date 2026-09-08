package app

import (
	"context"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/modules/identity"
	"github.com/syabanf/nuhabit-backend/internal/modules/inventory"
	"github.com/syabanf/nuhabit-backend/internal/modules/pos"
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
		inventory.Reference{Type: ref.Type, ID: ref.ID, Number: ref.Number,
			PackUnit: ref.PackUnit, PackFactor: ref.PackFactor,
			BatchCode: ref.BatchCode, ExpiresOn: ref.ExpiresOn},
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

// posStock and posLoyalty are the till's side of the same arrangement: POS
// declares two narrow ports and this is where they meet the real services.
type posStock struct{ inventory *inventory.Service }

func (a posStock) Issue(ctx context.Context, itemID, branchID string, qty domain.Quantity,
	ref pos.StockRef, actor pos.StockActor) error {
	_, err := a.inventory.Issue(ctx, itemID, branchID, qty, domain.MovementOut,
		inventory.Reference{Type: ref.Type, ID: ref.ID, Number: ref.Number,
			PackUnit: ref.PackUnit, PackFactor: ref.PackFactor},
		inventory.Actor{ID: actor.ID, Name: actor.Name})
	return err
}

func (a posStock) Restock(ctx context.Context, itemID, branchID string, qty domain.Quantity,
	unitCostIDR float64, ref pos.StockRef, actor pos.StockActor) error {
	_, err := a.inventory.Restore(ctx, itemID, branchID, qty, unitCostIDR,
		inventory.Reference{Type: ref.Type, ID: ref.ID, Number: ref.Number,
			PackUnit: ref.PackUnit, PackFactor: ref.PackFactor,
			RestoresType: ref.RestoresType, RestoresID: ref.RestoresID},
		inventory.Actor{ID: actor.ID, Name: actor.Name})
	return err
}

func (a posStock) UnitCost(ctx context.Context, itemID string) (float64, error) {
	return a.inventory.UnitCost(ctx, itemID)
}

func (a posStock) OnHand(ctx context.Context, itemID, branchID string) (domain.Quantity, error) {
	return a.inventory.OnHand(ctx, itemID, branchID)
}

// The two modules ask the same question and want different answers when the
// caller names no unit: a buyer means the carton they always order, a cashier
// means the single they always sell. Choosing between them is exactly the kind
// of decision an adapter exists to make, so neither module has to know the
// other's habits.
func (a purchasingStock) PackFor(ctx context.Context, itemID, unitCode string) (domain.ItemPack, error) {
	return a.inventory.PackFor(ctx, itemID, unitCode, domain.DefaultPurchasePack)
}

func (a posStock) PackFor(ctx context.Context, itemID, unitCode string) (domain.ItemPack, error) {
	return a.inventory.PackFor(ctx, itemID, unitCode, domain.DefaultSalePack)
}

// posSupervisors is the till's side of the override: it asks identity whether
// somebody holding a permission typed that PIN, and identity answers without
// the till ever seeing a hash.
type posSupervisors struct{ identity *identity.Service }

func (a posSupervisors) VerifyPIN(ctx context.Context, pin string,
	permission domain.Permission) (pos.SupervisorRef, bool, error) {

	supervisor, ok, err := a.identity.VerifyPIN(ctx, pin, permission)
	if err != nil || !ok {
		return pos.SupervisorRef{}, false, err
	}
	return pos.SupervisorRef{ID: supervisor.ID, Name: supervisor.Name}, true, nil
}
