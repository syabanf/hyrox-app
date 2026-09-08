package inventory

import (
	"net/http"
	"strings"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Handler serves the stockroom.
type Handler struct {
	service *Service
	guard   *auth.Guard
}

func NewHandler(service *Service, guard *auth.Guard) *Handler {
	return &Handler{service: service, guard: guard}
}

func (h *Handler) Mount(r *httpx.Router) {
	admin := func(p domain.Permission) httpx.Middleware { return h.guard.RequireAdmin(string(p)) }
	view := admin(domain.PermInventoryView)
	manage := admin(domain.PermInventoryManage)
	count := admin(domain.PermInventoryCount)

	r.Get("/api/admin/inventory/overview", h.overview, view)

	r.Get("/api/admin/inventory/categories", h.listCategories, view)
	r.Post("/api/admin/inventory/categories", h.createCategory, manage)
	r.Put("/api/admin/inventory/categories/{id}", h.updateCategory, manage)
	r.Delete("/api/admin/inventory/categories/{id}", h.deleteCategory, manage)

	r.Get("/api/admin/inventory/items", h.listItems, view)
	r.Post("/api/admin/inventory/items", h.createItem, manage)
	r.Get("/api/admin/inventory/items/{id}", h.getItem, view)
	r.Put("/api/admin/inventory/items/{id}", h.updateItem, manage)
	r.Put("/api/admin/inventory/items/{id}/reorder", h.setReorderPoint, manage)

	// Units and packs: the conversion between what arrives on a pallet and
	// what leaves in a hand.
	// Dated stock: what is on the shelf, and how long it has left.
	r.Get("/api/admin/inventory/batches", h.listBatches, view)
	r.Get("/api/admin/inventory/expiry", h.expiryReport, view)
	// Writing off expired goods is a stock adjustment, so it takes the same
	// grant as one — not the milder one that only counts.
	r.Post("/api/admin/inventory/batches/{id}/write-off", h.writeOffBatch, count)

	r.Get("/api/admin/inventory/units", h.listUnits, view)
	r.Put("/api/admin/inventory/units", h.saveUnit, manage)
	r.Get("/api/admin/inventory/items/{id}/packs", h.listPacks, view)
	r.Put("/api/admin/inventory/items/{id}/packs", h.savePack, manage)
	r.Delete("/api/admin/inventory/items/{id}/packs/{packId}", h.deletePack, manage)

	r.Get("/api/admin/inventory/stock", h.listStock, view)
	r.Get("/api/admin/inventory/movements", h.listMovements, view)
	// Changing a quantity by hand is its own grant, and always carries a reason.
	r.Post("/api/admin/inventory/adjust", h.adjust, count)
	r.Post("/api/admin/inventory/transfer", h.transfer, count)
	r.Get("/api/admin/inventory/transfers", h.listTransfers, view)

	r.Get("/api/admin/inventory/stock-takes", h.listStockTakes, view)
	r.Post("/api/admin/inventory/stock-takes", h.openStockTake, count)
	r.Get("/api/admin/inventory/stock-takes/{id}", h.getStockTake, view)
	r.Post("/api/admin/inventory/stock-takes/{id}/count", h.count, count)
	r.Delete("/api/admin/inventory/stock-takes/{id}/lines/{lineId}", h.removeCount, count)
	r.Post("/api/admin/inventory/stock-takes/{id}/{action}", h.decideStockTake, count)
}

func actorFrom(r *http.Request) Actor {
	principal, _ := auth.Admin(r.Context())
	return Actor{ID: principal.ID, Name: principal.Role}
}

// quantity reads a decimal quantity from a request field.
func quantity(value float64) domain.Quantity { return domain.Quantity(value) }

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.service.Overview(r.Context(), httpx.Query(r, "branchId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, overview)
}

// ── Categories ───────────────────────────────────────────────────────────────

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := h.service.Categories(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, categories)
}

type categoryRequest struct {
	Name      string `json:"name"`
	Code      string `json:"code"`
	Active    *bool  `json:"active"`
	SortOrder int    `json:"sortOrder"`
}

func (c *categoryRequest) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return httpx.Invalid("A category needs a name.")
	}
	if strings.TrimSpace(c.Code) == "" {
		return httpx.Invalid("A category needs a code.")
	}
	return nil
}

func (c categoryRequest) toInput() CategoryInput {
	return CategoryInput{
		Name: strings.TrimSpace(c.Name), Code: strings.TrimSpace(c.Code),
		Active: c.Active == nil || *c.Active, SortOrder: c.SortOrder,
	}
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[categoryRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := h.service.CreateCategory(r.Context(), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, created)
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[categoryRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updated, err := h.service.UpdateCategory(r.Context(), httpx.Param(r, "id"), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, updated)
}

func (h *Handler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteCategory(r.Context(), httpx.Param(r, "id"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}

// ── Items ────────────────────────────────────────────────────────────────────

func (h *Handler) listItems(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.Items(r.Context(), ItemFilter{
		Query:      httpx.Query(r, "query"),
		CategoryID: httpx.Query(r, "categoryId"),
		Kind:       strings.ToUpper(httpx.Query(r, "kind")),
		ActiveOnly: httpx.QueryBool(r, "activeOnly", false),
		Limit:      httpx.QueryInt(r, "limit", 300),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, items)
}

func (h *Handler) getItem(w http.ResponseWriter, r *http.Request) {
	detail, err := h.service.ItemDetail(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, detail)
}

type itemRequest struct {
	SKU         string  `json:"sku"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	CategoryID  *string `json:"categoryId"`
	Unit        string  `json:"unit"`
	Kind        string  `json:"kind"`
	TrackStock  *bool   `json:"trackStock"`
	// Dated goods. Off unless asked for: most of a catalogue has no date on
	// it, and demanding one would make every receipt a form nobody can fill.
	TrackBatches      *bool   `json:"trackBatches"`
	ExpiryWarningDays *int    `json:"expiryWarningDays"`
	Barcode           *string `json:"barcode"`
	ImageURL          *string `json:"imageUrl"`
	Active            *bool   `json:"active"`
}

func (i *itemRequest) Validate() error {
	if strings.TrimSpace(i.SKU) == "" {
		return httpx.Invalid("An item needs a SKU.")
	}
	if strings.TrimSpace(i.Name) == "" {
		return httpx.Invalid("An item needs a name.")
	}
	switch domain.ItemKind(strings.ToUpper(i.Kind)) {
	case domain.ItemRetail, domain.ItemSupply, domain.ItemRaw, "":
	default:
		return httpx.Invalid("%q is not an item kind.", i.Kind)
	}
	return nil
}

func (i itemRequest) toInput() ItemInput {
	kind := domain.ItemKind(strings.ToUpper(i.Kind))
	if kind == "" {
		kind = domain.ItemRetail
	}
	unit := strings.TrimSpace(i.Unit)
	if unit == "" {
		unit = "PCS"
	}
	return ItemInput{
		SKU: i.SKU, Name: i.Name, Description: i.Description, CategoryID: i.CategoryID,
		Unit: unit, Kind: kind,
		TrackStock:   i.TrackStock == nil || *i.TrackStock,
		TrackBatches: i.TrackBatches != nil && *i.TrackBatches,
		ExpiryWarningDays: func() int {
			if i.ExpiryWarningDays == nil {
				return 30
			}
			return *i.ExpiryWarningDays
		}(),
		Barcode: i.Barcode, ImageURL: i.ImageURL,
		Active: i.Active == nil || *i.Active,
	}
}

func (h *Handler) createItem(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[itemRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	created, err := h.service.CreateItem(r.Context(), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, created)
}

func (h *Handler) updateItem(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[itemRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	updated, err := h.service.UpdateItem(r.Context(), httpx.Param(r, "id"), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, updated)
}

type reorderRequest struct {
	BranchID    string   `json:"branchId"`
	QtyMinimum  float64  `json:"qtyMinimum"`
	QtyMaximum  *float64 `json:"qtyMaximum"`
	BinLocation *string  `json:"binLocation"`
	Note        *string  `json:"note"`
}

func (q *reorderRequest) Validate() error {
	if strings.TrimSpace(q.BranchID) == "" {
		return httpx.Invalid("A branch is required.")
	}
	if q.QtyMinimum < 0 {
		return httpx.Invalid("A minimum cannot be negative.")
	}
	return nil
}

func (h *Handler) setReorderPoint(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[reorderRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := ReorderInput{
		QtyMinimum: quantity(body.QtyMinimum), BinLocation: body.BinLocation, Note: body.Note,
	}
	if body.QtyMaximum != nil {
		max := quantity(*body.QtyMaximum)
		in.QtyMaximum = &max
	}
	level, err := h.service.SetReorderPoint(r.Context(), httpx.Param(r, "id"), body.BranchID, in, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, level)
}

// ── Stock ────────────────────────────────────────────────────────────────────

func (h *Handler) listStock(w http.ResponseWriter, r *http.Request) {
	rows, err := h.service.Stock(r.Context(), LevelFilter{
		BranchID:   httpx.Query(r, "branchId"),
		CategoryID: httpx.Query(r, "categoryId"),
		Query:      httpx.Query(r, "query"),
		LowOnly:    httpx.QueryBool(r, "lowOnly", false),
		Limit:      httpx.QueryInt(r, "limit", 500),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, rows)
}

func (h *Handler) listMovements(w http.ResponseWriter, r *http.Request) {
	movements, err := h.service.Movements(r.Context(), MovementFilter{
		ItemID:        httpx.Query(r, "itemId"),
		BranchID:      httpx.Query(r, "branchId"),
		Kind:          strings.ToUpper(httpx.Query(r, "kind")),
		ReferenceType: httpx.Query(r, "referenceType"),
		ReferenceID:   httpx.Query(r, "referenceId"),
		Limit:         httpx.QueryInt(r, "limit", 200),
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, movements)
}

type adjustRequest struct {
	ItemID   string  `json:"itemId"`
	BranchID string  `json:"branchId"`
	Qty      float64 `json:"qty"`
	Reason   string  `json:"reason"`
	// For dated stock being added by hand. Stock leaving does not name a
	// batch: FEFO decides, the same as it does for a sale.
	BatchCode string  `json:"batchCode"`
	ExpiresOn *string `json:"expiresOn"`
	Note      *string `json:"note"`
}

func (a *adjustRequest) Validate() error {
	if strings.TrimSpace(a.ItemID) == "" || strings.TrimSpace(a.BranchID) == "" {
		return httpx.Invalid("An item and a branch are required.")
	}
	if a.Qty == 0 {
		return httpx.Invalid("An adjustment of nothing is not an adjustment.")
	}
	if strings.TrimSpace(a.Reason) == "" {
		return httpx.Invalid("An adjustment needs a reason.")
	}
	if a.ExpiresOn != nil {
		if _, err := domain.ParseDate(*a.ExpiresOn); err != nil {
			return httpx.Invalid("expiresOn must be a date as YYYY-MM-DD.")
		}
	}
	return nil
}

func (h *Handler) adjust(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[adjustRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	movement, err := h.service.Adjust(r.Context(), AdjustInput{
		ItemID: body.ItemID, BranchID: body.BranchID, Qty: quantity(body.Qty),
		Reason: strings.TrimSpace(body.Reason), BatchCode: body.BatchCode,
		ExpiresOn: parseOptionalDate(body.ExpiresOn), Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, movement)
}

type transferRequest struct {
	ItemID       string  `json:"itemId"`
	FromBranchID string  `json:"fromBranchId"`
	ToBranchID   string  `json:"toBranchId"`
	Qty          float64 `json:"qty"`
	Note         *string `json:"note"`
}

func (t *transferRequest) Validate() error {
	if strings.TrimSpace(t.ItemID) == "" {
		return httpx.Invalid("An item is required.")
	}
	if t.FromBranchID == "" || t.ToBranchID == "" {
		return httpx.Invalid("A transfer needs both branches.")
	}
	if t.Qty <= 0 {
		return httpx.Invalid("A transfer needs a positive quantity.")
	}
	return nil
}

func (h *Handler) transfer(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[transferRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	transfer, err := h.service.Transfer(r.Context(), TransferInput{
		ItemID: body.ItemID, FromBranchID: body.FromBranchID, ToBranchID: body.ToBranchID,
		Qty: quantity(body.Qty), Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, transfer)
}

func (h *Handler) listTransfers(w http.ResponseWriter, r *http.Request) {
	transfers, err := h.service.Transfers(r.Context(),
		httpx.Query(r, "itemId"), httpx.QueryInt(r, "limit", 100))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, transfers)
}

// ── Stock takes ──────────────────────────────────────────────────────────────

func (h *Handler) listStockTakes(w http.ResponseWriter, r *http.Request) {
	takes, err := h.service.StockTakes(r.Context(),
		httpx.Query(r, "branchId"), strings.ToUpper(httpx.Query(r, "status")),
		httpx.QueryInt(r, "limit", 100))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, takes)
}

func (h *Handler) getStockTake(w http.ResponseWriter, r *http.Request) {
	take, err := h.service.StockTake(r.Context(), httpx.Param(r, "id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, take)
}

type openStockTakeRequest struct {
	BranchID  string  `json:"branchId"`
	CountedOn string  `json:"countedOn"`
	Note      *string `json:"note"`
}

func (o *openStockTakeRequest) Validate() error {
	if strings.TrimSpace(o.BranchID) == "" {
		return httpx.Invalid("A branch is required.")
	}
	if o.CountedOn != "" {
		if _, err := domain.ParseDate(o.CountedOn); err != nil {
			return httpx.Invalid("countedOn must be a date as YYYY-MM-DD.")
		}
	}
	return nil
}

func (h *Handler) openStockTake(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[openStockTakeRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	countedOn, _ := domain.ParseDate(body.CountedOn)
	take, err := h.service.OpenStockTake(r.Context(), body.BranchID, countedOn, body.Note, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, take)
}

type countRequest struct {
	ItemID     string  `json:"itemId"`
	QtyCounted float64 `json:"qtyCounted"`
	Note       *string `json:"note"`
}

func (c *countRequest) Validate() error {
	if strings.TrimSpace(c.ItemID) == "" {
		return httpx.Invalid("An item is required.")
	}
	if c.QtyCounted < 0 {
		return httpx.Invalid("A counted quantity cannot be negative.")
	}
	return nil
}

func (h *Handler) count(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[countRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	take, err := h.service.Count(r.Context(), httpx.Param(r, "id"), CountInput{
		ItemID: body.ItemID, QtyCounted: quantity(body.QtyCounted), Note: body.Note,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, take)
}

func (h *Handler) removeCount(w http.ResponseWriter, r *http.Request) {
	take, err := h.service.RemoveCount(r.Context(), httpx.Param(r, "id"), httpx.Param(r, "lineId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, take)
}

func (h *Handler) decideStockTake(w http.ResponseWriter, r *http.Request) {
	takeID := httpx.Param(r, "id")
	actor := actorFrom(r)

	var take StockTakeView
	var err error
	switch strings.ToLower(httpx.Param(r, "action")) {
	case "apply":
		take, err = h.service.ApplyStockTake(r.Context(), takeID, actor)
	case "cancel":
		take, err = h.service.CancelStockTake(r.Context(), takeID, actor)
	default:
		err = httpx.Invalid("A stock take is applied or cancelled.")
	}
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, take)
}
