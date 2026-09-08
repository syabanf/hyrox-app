package inventory

import (
	"context"
	"strings"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// Packs are the only place a quantity changes meaning.
//
// Everywhere else in the system a quantity is in the item's base unit, which
// is what makes movements addable. A pack is the translation at the edge: the
// buyer orders in cartons, the cashier sells in six-packs, and both convert
// here, once, on the way in.

func (s *Service) Units(ctx context.Context, activeOnly bool) ([]domain.Unit, error) {
	return s.repo.Units(ctx, activeOnly)
}

// UnitInput creates or renames a unit.
type UnitInput struct {
	Code      string
	Name      string
	Kind      domain.UnitKind
	SortOrder int
	Active    bool
}

func (s *Service) SaveUnit(ctx context.Context, in UnitInput, actor Actor) (domain.Unit, error) {
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" {
		return domain.Unit{}, httpx.Invalid("A unit needs a code.")
	}
	if strings.TrimSpace(in.Name) == "" {
		return domain.Unit{}, httpx.Invalid("A unit needs a name.")
	}
	kind := in.Kind
	if kind == "" {
		kind = domain.UnitCount
	}
	if !domain.IsValidUnitKind(string(kind)) {
		return domain.Unit{}, httpx.Invalid("%q is not a kind of unit.", in.Kind)
	}

	saved, err := s.repo.UpsertUnit(ctx, domain.Unit{
		ID: s.ids.New(id.Unit), Code: code, Name: strings.TrimSpace(in.Name),
		Kind: kind, SortOrder: in.SortOrder, Active: in.Active,
	})
	if err != nil {
		return domain.Unit{}, err
	}
	s.record(ctx, "inventory.unit", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

// PackView is a pack with the arithmetic already done, so a form does not have
// to repeat it.
type PackView struct {
	domain.ItemPack
	// Label reads the way a shelf edge does: "CTN (24 PCS)".
	Label string `json:"label"`
	// OnHandPacks is the branch's stock expressed in this pack, so somebody
	// looking at a carton row sees cartons.
	OnHandPacks *domain.Quantity `json:"onHandPacks,omitempty"`
}

func (s *Service) ItemPacks(ctx context.Context, itemID, branchID string) ([]PackView, error) {
	item, err := s.repo.Item(ctx, itemID, false)
	if err != nil {
		return nil, err
	}
	packs, err := s.repo.ItemPacks(ctx, itemID)
	if err != nil {
		return nil, err
	}

	var onHand domain.Quantity
	haveStock := false
	if branchID != "" {
		if level, err := s.repo.Level(ctx, itemID, branchID, false); err == nil {
			onHand, haveStock = level.QtyOnHand, true
		}
	}

	views := make([]PackView, 0, len(packs))
	for _, pack := range packs {
		view := PackView{ItemPack: pack, Label: pack.Label(item.Unit)}
		if haveStock {
			held := domain.BaseToPack(onHand, pack.Factor)
			view.OnHandPacks = &held
		}
		views = append(views, view)
	}
	return views, nil
}

// PackInput defines one way of handing an item over.
type PackInput struct {
	ItemID          string
	UnitCode        string
	Factor          float64
	Barcode         *string
	PurchaseDefault bool
	SaleDefault     bool
	Active          bool
}

// SavePack adds or changes a pack.
//
// The base pack is never created here — it is written when the item is, and it
// is the fixed point everything else is defined against.
func (s *Service) SavePack(ctx context.Context, in PackInput, actor Actor) (domain.ItemPack, error) {
	var saved domain.ItemPack
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		item, err := s.repo.Item(ctx, in.ItemID, false)
		if err != nil {
			return err
		}
		code := strings.ToUpper(strings.TrimSpace(in.UnitCode))
		unit, err := s.repo.Unit(ctx, code)
		if err != nil {
			return err
		}

		existing, err := s.repo.ItemPacks(ctx, in.ItemID)
		if err != nil {
			return err
		}
		candidate := domain.ItemPack{
			ItemID: in.ItemID, UnitCode: code, Factor: in.Factor,
			Barcode: in.Barcode, IsBase: code == item.Unit,
			PurchaseDefault: in.PurchaseDefault, SaleDefault: in.SaleDefault,
			Active: in.Active,
		}
		for _, pack := range existing {
			if pack.UnitCode == code {
				candidate.ID = pack.ID
				candidate.IsBase = pack.IsBase
			}
		}
		if candidate.ID == "" {
			candidate.ID = s.ids.New(id.ItemPack)
		}
		if rejection := domain.EvaluatePack(existing, candidate, unit.Kind); rejection != "" {
			return packRefusal(rejection, unit.Code)
		}

		// One default each way, so the old one steps aside first rather than
		// the partial unique index refusing the write.
		if candidate.PurchaseDefault {
			if err := s.repo.ClearPurchaseDefault(ctx, in.ItemID); err != nil {
				return err
			}
		}
		if candidate.SaleDefault {
			if err := s.repo.ClearSaleDefault(ctx, in.ItemID); err != nil {
				return err
			}
		}

		saved, err = s.repo.UpsertPack(ctx, candidate)
		return err
	})
	if err != nil {
		return domain.ItemPack{}, err
	}
	s.record(ctx, "inventory.pack", saved.ID, "SAVE", actor, nil)
	return saved, nil
}

func packRefusal(rejection domain.PackRejection, unit string) error {
	switch rejection {
	case domain.PackRejectFactor:
		return httpx.Invalid("A pack holds a positive number of base units.")
	case domain.PackRejectBaseFactor:
		return httpx.Invalid("The base pack holds exactly one base unit.")
	case domain.PackRejectFractional:
		return httpx.Invalid(
			"%s is counted in whole things, so it cannot hold a fraction of a base unit.", unit)
	case domain.PackRejectDuplicate:
		return httpx.Conflict("DUPLICATE", "That item already has a %s pack.", unit)
	}
	return httpx.Invalid("That pack cannot be defined.")
}

func (s *Service) DeletePack(ctx context.Context, packID string, actor Actor) error {
	if err := s.repo.DeletePack(ctx, packID); err != nil {
		return err
	}
	s.record(ctx, "inventory.pack", packID, "DELETE", actor, nil)
	return nil
}

// PackFor resolves the pack a document line should use.
//
// A named unit is looked up; a code the item has no pack for is refused rather
// than guessed at, because inventing a factor of 1 is precisely how an order
// for ten cartons is received as ten pieces. An empty code falls back to the
// pack `fallback` chooses — which differs between buying and selling, and is
// therefore the caller's decision rather than this function's.
func (s *Service) PackFor(ctx context.Context, itemID, unitCode string,
	fallback func([]domain.ItemPack) (domain.ItemPack, bool)) (domain.ItemPack, error) {

	packs, err := s.repo.ItemPacks(ctx, itemID)
	if err != nil {
		return domain.ItemPack{}, err
	}
	if len(packs) == 0 {
		return domain.ItemPack{}, httpx.Conflict("NO_PACKS",
			"That item has no unit defined, so a quantity of it has no meaning.")
	}

	code := strings.ToUpper(strings.TrimSpace(unitCode))
	if code == "" {
		if pack, ok := fallback(packs); ok {
			return pack, nil
		}
		return packs[0], nil
	}
	pack, ok := domain.FindPack(packs, code)
	if !ok {
		return domain.ItemPack{}, httpx.Invalid(
			"That item is not stocked in %s. Define the pack first.", code)
	}
	return pack, nil
}
