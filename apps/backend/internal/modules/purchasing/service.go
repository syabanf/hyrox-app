// Package purchasing is how stock gets onto the shelves.
//
// The spine is request → order → receipt, and each step is a different pair of
// hands on purpose: raising a purchase, approving it and signing for what
// arrives are three separate grants, because one person doing all three is how
// invoices get paid for goods that never came.
//
// It reaches outside itself in exactly one place. Posting a goods receipt asks
// inventory to move stock, through a four-method port, in the same transaction
// — never by writing inventory's tables.
package purchasing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/audit"
	"github.com/syabanf/nuhabit-backend/internal/platform/clock"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
	"github.com/syabanf/nuhabit-backend/internal/platform/id"
)

// StockRef is what a stock movement points back at.
type StockRef struct {
	Type, ID, Number string
	// The pack the document was written in, so the ledger row can say "10
	// CTN" beside the 240 pieces it actually moved.
	PackUnit   string
	PackFactor float64
	// What the delivery note said about dates. Ignored for goods that do not
	// carry one.
	BatchCode string
	ExpiresOn *domain.Date
}

// StockActor is who caused one.
type StockActor struct{ ID, Name string }

// Stock is the port purchasing needs from inventory. Four methods, not the
// forty the inventory service exposes: when inventory moves out, this is the
// complete list of what has to be reimplemented over the network.
type Stock interface {
	// Receive books accepted goods in at the price actually paid, which is
	// what moves the item's weighted-average cost.
	Receive(ctx context.Context, itemID, branchID string, qty domain.Quantity,
		unitCostIDR float64, ref StockRef, actor StockActor) error
	// Return sends goods back out again.
	Return(ctx context.Context, itemID, branchID string, qty domain.Quantity,
		ref StockRef, actor StockActor) error
	// ReserveOnOrder and ReleaseOnOrder keep the reorder screen honest about
	// stock that has been ordered but has not arrived.
	ReserveOnOrder(ctx context.Context, itemID, branchID string, qty domain.Quantity) error
	ReleaseOnOrder(ctx context.Context, itemID, branchID string, qty domain.Quantity) error
	// ItemName is for showing a line without a second round trip.
	ItemNames(ctx context.Context) (map[string]string, error)
	// PackFor resolves the unit a line is ordered in to how many base units
	// it holds. Purchasing never invents a factor: an unknown unit is refused
	// here rather than silently treated as one piece, which is how an order
	// for ten cartons is received as ten pieces.
	PackFor(ctx context.Context, itemID, unitCode string) (domain.ItemPack, error)
}

// Catalog is the port for branches.
type Catalog interface {
	Branches(ctx context.Context) ([]domain.Branch, error)
}

// Service implements the buying use cases.
type Service struct {
	db         *database.DB
	repo       *Repository
	stock      Stock
	catalog    Catalog
	ids        id.Generator
	clock      clock.Clock
	auditor    audit.Recorder
	studio     *time.Location
	thresholds domain.ApprovalThresholds
}

func NewService(db *database.DB, repo *Repository, stock Stock, catalog Catalog,
	ids id.Generator, c clock.Clock, auditor audit.Recorder, studio *time.Location) *Service {
	if studio == nil {
		studio = time.UTC
	}
	return &Service{db: db, repo: repo, stock: stock, catalog: catalog, ids: ids, clock: c,
		auditor: auditor, studio: studio, thresholds: domain.DefaultApprovalThresholds()}
}

// Actor identifies who performed an action, and at what authority.
type Actor struct {
	ID   string
	Name string
	Role domain.AdminRole
}

func (s *Service) record(ctx context.Context, entity, entityID, action string, actor Actor, reason *string) {
	_ = s.auditor.Record(ctx, audit.Event{
		EntityType: entity, EntityID: entityID, Action: action,
		ActorID: actor.ID, ActorName: string(actor.Role), Reason: reason,
	})
}

func (s *Service) stockActor(actor Actor) StockActor {
	return StockActor{ID: actor.ID, Name: string(actor.Role)}
}

// documentNumber builds a human-quotable reference: PO-20260908-4F2A1B.
func (s *Service) documentNumber(prefix, idPrefix string) string {
	generated := s.ids.New(idPrefix)
	tail := generated
	if _, cut, found := strings.Cut(generated, "_"); found && len(cut) >= 6 {
		tail = cut[len(cut)-6:]
	}
	return fmt.Sprintf("%s-%s-%s", prefix, s.clock.Now().In(s.studio).Format("20060102"), tail)
}

func (s *Service) today() domain.Date { return domain.DateOf(s.clock.Now(), s.studio) }

// ── Suppliers ────────────────────────────────────────────────────────────────

func (s *Service) Suppliers(ctx context.Context, filter SupplierFilter) ([]domain.Supplier, error) {
	return s.repo.Suppliers(ctx, filter)
}

func (s *Service) Supplier(ctx context.Context, id string) (domain.Supplier, error) {
	return s.repo.Supplier(ctx, id)
}

// SupplierInput is a whole supplier record.
type SupplierInput struct {
	Code         string
	Name         string
	ContactName  *string
	ContactPhone *string
	Email        *string
	Address      *string
	City         *string
	TaxNumber    *string
	PaymentTerms domain.PaymentTerms
	BankName     *string
	BankAccount  *string
	BankHolder   *string
	Category     *string
	Status       domain.SupplierStatus
	Note         *string
}

func (in SupplierInput) apply(s domain.Supplier) domain.Supplier {
	s.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	s.Name = strings.TrimSpace(in.Name)
	s.ContactName = in.ContactName
	s.ContactPhone = in.ContactPhone
	s.Email = in.Email
	s.Address = in.Address
	s.City = in.City
	s.TaxNumber = in.TaxNumber
	s.PaymentTerms = in.PaymentTerms
	s.BankName = in.BankName
	s.BankAccount = in.BankAccount
	s.BankHolder = in.BankHolder
	s.Category = in.Category
	s.Status = in.Status
	s.Note = in.Note
	return s
}

func (s *Service) CreateSupplier(ctx context.Context, in SupplierInput, actor Actor) (domain.Supplier, error) {
	created, err := s.repo.InsertSupplier(ctx, in.apply(domain.Supplier{ID: s.ids.New(id.Supplier)}))
	if err != nil {
		return domain.Supplier{}, err
	}
	s.record(ctx, "purchasing.supplier", created.ID, "CREATE", actor, nil)
	return created, nil
}

func (s *Service) UpdateSupplier(ctx context.Context, supplierID string, in SupplierInput, actor Actor) (domain.Supplier, error) {
	existing, err := s.repo.Supplier(ctx, supplierID)
	if err != nil {
		return domain.Supplier{}, err
	}
	updated, err := s.repo.UpdateSupplier(ctx, in.apply(existing))
	if err != nil {
		return domain.Supplier{}, err
	}

	action := "UPDATE"
	if existing.Status != domain.SupplierBlocked && updated.Status == domain.SupplierBlocked {
		// Blocking a supplier is a decision worth finding on its own.
		action = "BLOCK"
	}
	s.record(ctx, "purchasing.supplier", supplierID, action, actor, updated.Note)
	return updated, nil
}

func (s *Service) DeleteSupplier(ctx context.Context, supplierID string, actor Actor) error {
	if err := s.repo.DeleteSupplier(ctx, supplierID); err != nil {
		return err
	}
	s.record(ctx, "purchasing.supplier", supplierID, "DELETE", actor, nil)
	return nil
}

func (s *Service) SupplierPrices(ctx context.Context, supplierID, itemID string) ([]domain.SupplierPrice, error) {
	return s.repo.SupplierPrices(ctx, supplierID, itemID)
}

// PriceInput records what a supplier charges.
type PriceInput struct {
	ItemID        string
	UnitPriceIDR  float64
	MinOrderQty   domain.Quantity
	LeadTimeDays  int
	EffectiveFrom domain.Date
}

func (s *Service) SetSupplierPrice(ctx context.Context, supplierID string, in PriceInput, actor Actor) (domain.SupplierPrice, error) {
	if in.EffectiveFrom == "" {
		in.EffectiveFrom = s.today()
	}
	saved, err := s.repo.UpsertSupplierPrice(ctx, domain.SupplierPrice{
		ID: s.ids.New(id.LineItem), SupplierID: supplierID, ItemID: in.ItemID,
		UnitPriceIDR: in.UnitPriceIDR, MinOrderQty: in.MinOrderQty,
		LeadTimeDays: in.LeadTimeDays, EffectiveFrom: in.EffectiveFrom, Active: true,
	})
	if err != nil {
		return domain.SupplierPrice{}, err
	}
	s.record(ctx, "purchasing.supplier", supplierID, "PRICE", actor, nil)
	return saved, nil
}

// ── Purchase requests ────────────────────────────────────────────────────────

// RequestView is a request with its lines and where it stands in the chain.
type RequestView struct {
	domain.PurchaseRequest
	BranchName string                       `json:"branchName"`
	Items      []domain.PurchaseRequestItem `json:"items"`
	// Chain is every signature the amount requires, in order, with who signed.
	Chain []ApprovalStep `json:"chain"`
	// AwaitingLevel is what it is waiting on now, empty when fully signed.
	AwaitingLevel domain.ApprovalLevel `json:"awaitingLevel"`
}

// ApprovalStep is one signature on the chain.
type ApprovalStep struct {
	Level    domain.ApprovalLevel `json:"level"`
	Signed   bool                 `json:"signed"`
	SignedBy *string              `json:"signedBy"`
	SignedAt *time.Time           `json:"signedAt"`
}

func (s *Service) viewRequest(ctx context.Context, request domain.PurchaseRequest) (RequestView, error) {
	items, err := s.repo.RequestItems(ctx, request.ID)
	if err != nil {
		return RequestView{}, err
	}
	branches, err := s.branchNames(ctx)
	if err != nil {
		return RequestView{}, err
	}

	view := RequestView{
		PurchaseRequest: request, BranchName: branches[request.BranchID], Items: items,
	}
	for _, level := range domain.RequiredApprovals(request.TotalIDR, s.thresholds) {
		step := ApprovalStep{Level: level, Signed: request.SignedAt(level)}
		switch level {
		case domain.ApprovalHead:
			step.SignedBy, step.SignedAt = request.ApprovedByHead, request.ApprovedAtHead
		case domain.ApprovalFinance:
			step.SignedBy, step.SignedAt = request.ApprovedByFinance, request.ApprovedAtFinance
		case domain.ApprovalDirector:
			step.SignedBy, step.SignedAt = request.ApprovedByDirector, request.ApprovedAtDirector
		}
		view.Chain = append(view.Chain, step)
	}
	if level, waiting := domain.NextApproval(request, s.thresholds); waiting {
		view.AwaitingLevel = level
	}
	return view, nil
}

func (s *Service) branchNames(ctx context.Context) (map[string]string, error) {
	branches, err := s.catalog.Branches(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(branches))
	for _, b := range branches {
		names[b.ID] = b.Name
	}
	return names, nil
}

func (s *Service) Requests(ctx context.Context, filter RequestFilter) ([]domain.PurchaseRequest, error) {
	return s.repo.Requests(ctx, filter)
}

func (s *Service) Request(ctx context.Context, requestID string) (RequestView, error) {
	request, err := s.repo.Request(ctx, requestID, false)
	if err != nil {
		return RequestView{}, err
	}
	return s.viewRequest(ctx, request)
}

// RequestInput opens a purchase request.
type RequestInput struct {
	BranchID      string
	RequesterID   *string
	RequesterName string
	DepartmentID  *string
	Priority      string
	RequiredOn    *domain.Date
	Note          *string
}

func (s *Service) CreateRequest(ctx context.Context, in RequestInput, actor Actor) (RequestView, error) {
	priority := strings.ToUpper(in.Priority)
	if priority == "" {
		priority = "NORMAL"
	}
	name := strings.TrimSpace(in.RequesterName)
	if name == "" {
		name = actor.Name
	}

	created, err := s.repo.InsertRequest(ctx, domain.PurchaseRequest{
		ID: s.ids.New(id.PurchaseRequest), PRNumber: s.documentNumber("PR", id.PurchaseRequest),
		BranchID: in.BranchID, RequesterID: in.RequesterID, RequesterName: name,
		DepartmentID: in.DepartmentID, Status: domain.PRDraft, Priority: priority,
		RequiredOn: in.RequiredOn, Note: in.Note,
	})
	if err != nil {
		return RequestView{}, err
	}
	s.record(ctx, "purchasing.request", created.ID, "CREATE", actor, nil)
	return s.viewRequest(ctx, created)
}

// RequestLineInput is one thing being asked for.
type RequestLineInput struct {
	ItemID            *string
	Description       string
	Qty               domain.Quantity
	Unit              string
	EstimatedPriceIDR float64
	Note              *string
}

// AddRequestLine adds a line and re-totals the request. Only a draft can be
// changed: once it is in the approval chain, what people signed must stay put.
func (s *Service) AddRequestLine(ctx context.Context, requestID string, in RequestLineInput, actor Actor) (RequestView, error) {
	var view RequestView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		request, err := s.repo.Request(ctx, requestID, true)
		if err != nil {
			return err
		}
		if request.Status != domain.PRDraft {
			return httpx.Conflict("NOT_DRAFT",
				"That request is %s; a line cannot be added once it has been submitted.",
				strings.ToLower(string(request.Status)))
		}
		if in.Qty <= 0 {
			return httpx.Invalid("A line needs a positive quantity.")
		}

		unit := strings.ToUpper(strings.TrimSpace(in.Unit))
		if unit == "" {
			unit = "PCS"
		}
		if _, err := s.repo.InsertRequestItem(ctx, domain.PurchaseRequestItem{
			ID: s.ids.New(id.LineItem), RequestID: requestID, ItemID: in.ItemID,
			Description: strings.TrimSpace(in.Description), Qty: in.Qty, Unit: unit,
			EstimatedPriceIDR: in.EstimatedPriceIDR, Note: in.Note,
		}); err != nil {
			return err
		}

		request, err = s.retotalRequest(ctx, request)
		if err != nil {
			return err
		}
		view, err = s.viewRequest(ctx, request)
		return err
	})
	return view, err
}

// retotalRequest recomputes a request's total from its lines. The total is
// stored because it decides the approval chain, and a chain that changes when
// somebody edits a price is not a chain.
func (s *Service) retotalRequest(ctx context.Context, request domain.PurchaseRequest) (domain.PurchaseRequest, error) {
	items, err := s.repo.RequestItems(ctx, request.ID)
	if err != nil {
		return domain.PurchaseRequest{}, err
	}
	var total float64
	for _, item := range items {
		total += item.TotalIDR
	}
	request.TotalIDR = total
	return s.repo.SaveRequest(ctx, request)
}

func (s *Service) RemoveRequestLine(ctx context.Context, requestID, lineID string, actor Actor) (RequestView, error) {
	var view RequestView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		request, err := s.repo.Request(ctx, requestID, true)
		if err != nil {
			return err
		}
		if request.Status != domain.PRDraft {
			return httpx.Conflict("NOT_DRAFT",
				"That request is %s and cannot be changed.", strings.ToLower(string(request.Status)))
		}
		if err := s.repo.DeleteRequestItem(ctx, lineID); err != nil {
			return err
		}
		request, err = s.retotalRequest(ctx, request)
		if err != nil {
			return err
		}
		view, err = s.viewRequest(ctx, request)
		return err
	})
	return view, err
}

// SubmitRequest sends a draft into the approval chain.
func (s *Service) SubmitRequest(ctx context.Context, requestID string, actor Actor) (RequestView, error) {
	var view RequestView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		request, err := s.repo.Request(ctx, requestID, true)
		if err != nil {
			return err
		}
		items, err := s.repo.RequestItems(ctx, requestID)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return httpx.Conflict("NO_LINES", "That request has nothing on it.")
		}

		level, waiting := domain.NextApproval(request, s.thresholds)
		if !waiting {
			level = domain.ApprovalHead
		}
		next, err := domain.Transition(domain.PurchaseRequestTransitions,
			request.Status, domain.PendingStatusFor(level))
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That request is %s and cannot be submitted.", strings.ToLower(string(request.Status)))
		}

		request.Status = next
		saved, err := s.repo.SaveRequest(ctx, request)
		if err != nil {
			return err
		}
		view, err = s.viewRequest(ctx, saved)
		return err
	})
	if err != nil {
		return RequestView{}, err
	}
	s.record(ctx, "purchasing.request", requestID, "SUBMIT", actor, nil)
	return view, nil
}

// ApproveRequest signs one level of the chain.
//
// The level signed is whatever the request is currently waiting on, and the
// actor's role has to reach it. That is the whole control: a branch manager
// cannot sign the finance level by asking twice.
func (s *Service) ApproveRequest(ctx context.Context, requestID string, actor Actor) (RequestView, error) {
	var view RequestView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		request, err := s.repo.Request(ctx, requestID, true)
		if err != nil {
			return err
		}

		level, waiting := domain.NextApproval(request, s.thresholds)
		if !waiting {
			return httpx.Conflict("ALREADY_APPROVED", "That request is already fully approved.")
		}
		if !domain.CanApproveAt(actor.Role, level) {
			return httpx.ErrForbidden.WithMessage(
				"That request is waiting on %s approval, which your role cannot give.",
				strings.ToLower(string(level)))
		}

		now := s.clock.Now()
		switch level {
		case domain.ApprovalHead:
			request.ApprovedByHead, request.ApprovedAtHead = &actor.ID, &now
		case domain.ApprovalFinance:
			request.ApprovedByFinance, request.ApprovedAtFinance = &actor.ID, &now
		case domain.ApprovalDirector:
			request.ApprovedByDirector, request.ApprovedAtDirector = &actor.ID, &now
		}

		target := domain.PRApproved
		if nextLevel, stillWaiting := domain.NextApproval(request, s.thresholds); stillWaiting {
			target = domain.PendingStatusFor(nextLevel)
		}
		next, err := domain.Transition(domain.PurchaseRequestTransitions, request.Status, target)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That request is %s and cannot be approved.", strings.ToLower(string(request.Status)))
		}
		request.Status = next

		saved, err := s.repo.SaveRequest(ctx, request)
		if err != nil {
			return err
		}
		view, err = s.viewRequest(ctx, saved)
		return err
	})
	if err != nil {
		return RequestView{}, err
	}
	s.record(ctx, "purchasing.request", requestID, "APPROVE", actor, nil)
	return view, nil
}

// RejectRequest turns a request down at any pending stage.
func (s *Service) RejectRequest(ctx context.Context, requestID, reason string, actor Actor) (RequestView, error) {
	if strings.TrimSpace(reason) == "" {
		return RequestView{}, httpx.Invalid("A rejection needs a reason.")
	}

	var view RequestView
	err := s.db.InTx(ctx, func(ctx context.Context) error {
		request, err := s.repo.Request(ctx, requestID, true)
		if err != nil {
			return err
		}
		next, err := domain.Transition(domain.PurchaseRequestTransitions, request.Status, domain.PRRejected)
		if err != nil {
			return httpx.Conflict("INVALID_TRANSITION",
				"That request is %s and cannot be rejected.", strings.ToLower(string(request.Status)))
		}

		now := s.clock.Now()
		request.Status = next
		request.RejectedBy, request.RejectedAt, request.RejectionReason = &actor.ID, &now, &reason

		saved, err := s.repo.SaveRequest(ctx, request)
		if err != nil {
			return err
		}
		view, err = s.viewRequest(ctx, saved)
		return err
	})
	if err != nil {
		return RequestView{}, err
	}
	s.record(ctx, "purchasing.request", requestID, "REJECT", actor, &reason)
	return view, nil
}
