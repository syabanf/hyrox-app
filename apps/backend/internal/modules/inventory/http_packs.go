package inventory

import (
	"net/http"
	"strings"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

func (h *Handler) listUnits(w http.ResponseWriter, r *http.Request) {
	units, err := h.service.Units(r.Context(), httpx.QueryBool(r, "activeOnly", false))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, units)
}

type unitBody struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	SortOrder int    `json:"sortOrder"`
	Active    *bool  `json:"active"`
}

func (u *unitBody) Validate() error {
	if strings.TrimSpace(u.Code) == "" || strings.TrimSpace(u.Name) == "" {
		return httpx.Invalid("A unit needs a code and a name.")
	}
	if u.Kind != "" && !domain.IsValidUnitKind(strings.ToUpper(u.Kind)) {
		return httpx.Invalid("%q is not a kind of unit.", u.Kind)
	}
	return nil
}

func (h *Handler) saveUnit(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[unitBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	unit, err := h.service.SaveUnit(r.Context(), UnitInput{
		Code: body.Code, Name: body.Name,
		Kind:      domain.UnitKind(strings.ToUpper(body.Kind)),
		SortOrder: body.SortOrder, Active: body.Active == nil || *body.Active,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, unit)
}

func (h *Handler) listPacks(w http.ResponseWriter, r *http.Request) {
	packs, err := h.service.ItemPacks(r.Context(), httpx.Param(r, "id"), httpx.Query(r, "branchId"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, packs)
}

type packBody struct {
	UnitCode        string  `json:"unitCode"`
	Factor          float64 `json:"factor"`
	Barcode         *string `json:"barcode"`
	PurchaseDefault bool    `json:"purchaseDefault"`
	SaleDefault     bool    `json:"saleDefault"`
	Active          *bool   `json:"active"`
}

func (p *packBody) Validate() error {
	if strings.TrimSpace(p.UnitCode) == "" {
		return httpx.Invalid("A pack needs a unit.")
	}
	if p.Factor <= 0 {
		return httpx.Invalid("A pack holds a positive number of base units.")
	}
	return nil
}

func (h *Handler) savePack(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[packBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	pack, err := h.service.SavePack(r.Context(), PackInput{
		ItemID: httpx.Param(r, "id"), UnitCode: body.UnitCode, Factor: body.Factor,
		Barcode: body.Barcode, PurchaseDefault: body.PurchaseDefault,
		SaleDefault: body.SaleDefault, Active: body.Active == nil || *body.Active,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, pack)
}

func (h *Handler) deletePack(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeletePack(r.Context(), httpx.Param(r, "packId"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}
