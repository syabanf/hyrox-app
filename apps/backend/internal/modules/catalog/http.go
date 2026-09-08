package catalog

import (
	"context"
	"net/http"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// PackageStat is how a package has actually sold. It comes from the wallet
// module, which owns payments.
type PackageStat struct {
	PurchaseCount int   `json:"purchaseCount"`
	RevenueIDR    int64 `json:"revenueIdr"`
}

// Usage is the port catalog declares for the facts it needs from other
// modules: whether a row may be deleted, and how a package has sold.
//
// Catalog never reads another module's tables to answer these. Wiring supplies
// an adapter today; a remote client would satisfy the same interface.
type Usage interface {
	SessionsForClassType(ctx context.Context, classTypeID string) (int, error)
	UpcomingSessionsForCoach(ctx context.Context, coachID string) (int, error)
	PaymentsForPackage(ctx context.Context, packageID string) (int, error)
	PackageStats(ctx context.Context) (map[string]PackageStat, error)
}

// Handler serves the catalog module's HTTP surface.
type Handler struct {
	service *Service
	usage   Usage
	guard   *auth.Guard
}

func NewHandler(service *Service, usage Usage, guard *auth.Guard) *Handler {
	return &Handler{service: service, usage: usage, guard: guard}
}

// Mount registers every catalog route. Public reads are what the member app
// needs to render a schedule and a shop; everything else is staff-only and
// permission-checked.
func (h *Handler) Mount(r *httpx.Router) {
	// Public catalog: branches, bookable class types, purchasable packages.
	r.Get("/api/branches", h.listBranches)
	r.Get("/api/class-types", h.listPublicClassTypes)
	r.Get("/api/packages", h.listPublicPackages)

	admin := func(permission domain.Permission) httpx.Middleware {
		return h.guard.RequireAdmin(string(permission))
	}

	r.Get("/api/admin/branches", h.listBranches, admin(domain.PermConfigView))
	r.Post("/api/admin/branches", h.createBranch, admin(domain.PermBranchesManage))
	r.Patch("/api/admin/branches/{id}", h.updateBranch, admin(domain.PermBranchesManage))
	r.Delete("/api/admin/branches/{id}", h.deleteBranch, admin(domain.PermBranchesManage))

	r.Get("/api/admin/gates", h.listGates, admin(domain.PermAccessView))
	r.Post("/api/admin/gates", h.createGate, admin(domain.PermGatesManage))
	r.Patch("/api/admin/gates/{id}", h.updateGate, admin(domain.PermGatesManage))
	r.Delete("/api/admin/gates/{id}", h.deleteGate, admin(domain.PermGatesManage))

	r.Get("/api/admin/coaches", h.listCoaches, admin(domain.PermOperationsView))
	r.Post("/api/admin/coaches", h.createCoach, admin(domain.PermCoachesManage))
	r.Patch("/api/admin/coaches/{id}", h.updateCoach, admin(domain.PermCoachesManage))
	r.Delete("/api/admin/coaches/{id}", h.deleteCoach, admin(domain.PermCoachesManage))

	r.Get("/api/admin/class-types", h.listAdminClassTypes, admin(domain.PermOperationsView))
	r.Post("/api/admin/class-types", h.createClassType, admin(domain.PermClassTypesManage))
	r.Patch("/api/admin/class-types/{id}", h.updateClassType, admin(domain.PermClassTypesManage))
	r.Delete("/api/admin/class-types/{id}", h.deleteClassType, admin(domain.PermClassTypesManage))

	r.Get("/api/admin/packages", h.listAdminPackages, admin(domain.PermCommercialView))
	r.Post("/api/admin/packages", h.createPackage, admin(domain.PermPackagesManage))
	r.Patch("/api/admin/packages/{id}", h.updatePackage, admin(domain.PermPackagesManage))
	r.Delete("/api/admin/packages/{id}", h.deletePackage, admin(domain.PermPackagesManage))

	r.Get("/api/admin/rules", h.getRules, admin(domain.PermConfigView))
	r.Put("/api/admin/rules", h.updateRules, admin(domain.PermRulesUpdate))

	r.Get("/api/admin/exercises", h.listExercises, admin(domain.PermOperationsView))
	r.Patch("/api/admin/exercises/{id}", h.updateExercise, admin(domain.PermClassTypesManage))
	r.Get("/api/exercises", h.exerciseLibrary, h.guard.RequireMember)
}

// actorFrom identifies the staff member behind an administrative action.
func actorFrom(r *http.Request) Actor {
	principal, _ := auth.Admin(r.Context())
	return Actor{ID: principal.ID, Name: principal.Role}
}

// ── Branches ─────────────────────────────────────────────────────────────────

func (h *Handler) listBranches(w http.ResponseWriter, r *http.Request) {
	branches, err := h.service.Branches(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, branches)
}

type branchRequest struct {
	Name           string  `json:"name"`
	Address        string  `json:"address"`
	Timezone       string  `json:"timezone"`
	OperatingHours string  `json:"operatingHours"`
	Status         string  `json:"status"`
	ManagerName    *string `json:"managerName"`
}

func (b *branchRequest) Validate() error {
	if len(strings.TrimSpace(b.Name)) < 2 {
		return httpx.Invalid("A branch name of at least 2 characters is required.")
	}
	if len(strings.TrimSpace(b.Address)) < 3 {
		return httpx.Invalid("A branch address is required.")
	}
	if b.Status != "" && b.Status != string(domain.StatusActive) && b.Status != string(domain.StatusInactive) {
		return httpx.Invalid("Status must be ACTIVE or INACTIVE.")
	}
	return nil
}

func (b branchRequest) toInput() BranchInput {
	return BranchInput{
		Name:           b.Name,
		Address:        b.Address,
		Timezone:       b.Timezone,
		OperatingHours: b.OperatingHours,
		Status:         domain.ActiveStatus(b.Status),
		ManagerName:    b.ManagerName,
	}
}

func (h *Handler) createBranch(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[branchRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	branch, err := h.service.CreateBranch(r.Context(), body.toInput(), actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, branch)
}

// branchPatch is a true partial update: an absent field is left alone, which
// is what PATCH means.
type branchPatch struct {
	Name           *string `json:"name"`
	Address        *string `json:"address"`
	Timezone       *string `json:"timezone"`
	OperatingHours *string `json:"operatingHours"`
	Status         *string `json:"status"`
	ManagerName    *string `json:"managerName"`
}

func (h *Handler) updateBranch(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[branchPatch](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := BranchInput{ManagerName: body.ManagerName}
	in.Name = deref(body.Name)
	in.Address = deref(body.Address)
	in.Timezone = deref(body.Timezone)
	in.OperatingHours = deref(body.OperatingHours)
	in.Status = domain.ActiveStatus(deref(body.Status))

	branch, err := h.service.UpdateBranch(r.Context(), httpx.Param(r, "id"), in, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, branch)
}

func (h *Handler) deleteBranch(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteBranch(r.Context(), httpx.Param(r, "id"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, okResponse{OK: true})
}

// ── Gates ────────────────────────────────────────────────────────────────────

func (h *Handler) listGates(w http.ResponseWriter, r *http.Request) {
	gates, err := h.service.Gates(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, gates)
}

type gateRequest struct {
	Name     string `json:"name"`
	BranchID string `json:"branchId"`
	Status   string `json:"status"`
}

func (g *gateRequest) Validate() error {
	if len(strings.TrimSpace(g.Name)) < 2 {
		return httpx.Invalid("A gate name of at least 2 characters is required.")
	}
	if g.BranchID == "" {
		return httpx.Invalid("A branch is required.")
	}
	if g.Status != "" && g.Status != string(domain.GateOnline) && g.Status != string(domain.GateOffline) {
		return httpx.Invalid("Status must be ONLINE or OFFLINE.")
	}
	return nil
}

func (h *Handler) createGate(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[gateRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	gate, err := h.service.CreateGate(r.Context(), GateInput{
		Name: body.Name, BranchID: body.BranchID, Status: domain.GateStatus(body.Status),
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, gate)
}

type gatePatch struct {
	Name     *string `json:"name"`
	BranchID *string `json:"branchId"`
	Status   *string `json:"status"`
}

func (h *Handler) updateGate(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[gatePatch](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	gate, err := h.service.UpdateGate(r.Context(), httpx.Param(r, "id"), GateInput{
		Name:     deref(body.Name),
		BranchID: deref(body.BranchID),
		Status:   domain.GateStatus(deref(body.Status)),
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, gate)
}

func (h *Handler) deleteGate(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteGate(r.Context(), httpx.Param(r, "id"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, okResponse{OK: true})
}

// ── Coaches ──────────────────────────────────────────────────────────────────

func (h *Handler) listCoaches(w http.ResponseWriter, r *http.Request) {
	coaches, err := h.service.Coaches(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, coaches)
}

type coachRequest struct {
	Name           string `json:"name"`
	Bio            string `json:"bio"`
	Specialization string `json:"specialization"`
	BranchID       string `json:"branchId"`
	Status         string `json:"status"`
}

func (c *coachRequest) Validate() error {
	if len(strings.TrimSpace(c.Name)) < 2 {
		return httpx.Invalid("A coach name of at least 2 characters is required.")
	}
	if c.BranchID == "" {
		return httpx.Invalid("A branch is required.")
	}
	return nil
}

func (h *Handler) createCoach(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[coachRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	coach, err := h.service.CreateCoach(r.Context(), CoachInput{
		Name: body.Name, Bio: body.Bio, Specialization: body.Specialization,
		BranchID: body.BranchID, Status: domain.ActiveStatus(body.Status),
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, coach)
}

type coachPatch struct {
	Name           *string `json:"name"`
	Bio            *string `json:"bio"`
	Specialization *string `json:"specialization"`
	BranchID       *string `json:"branchId"`
	Status         *string `json:"status"`
}

func (h *Handler) updateCoach(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[coachPatch](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	coach, err := h.service.UpdateCoach(r.Context(), httpx.Param(r, "id"), CoachInput{
		Name:           deref(body.Name),
		Bio:            deref(body.Bio),
		Specialization: deref(body.Specialization),
		BranchID:       deref(body.BranchID),
		Status:         domain.ActiveStatus(deref(body.Status)),
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, coach)
}

func (h *Handler) deleteCoach(w http.ResponseWriter, r *http.Request) {
	coachID := httpx.Param(r, "id")
	upcoming, err := h.usage.UpcomingSessionsForCoach(r.Context(), coachID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.DeleteCoach(r.Context(), coachID, upcoming, actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, okResponse{OK: true})
}

// ── Class types ──────────────────────────────────────────────────────────────

func (h *Handler) listPublicClassTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.service.ClassTypes(r.Context(), true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, types)
}

func (h *Handler) listAdminClassTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.service.ClassTypes(r.Context(), false)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, types)
}

type classTypeRequest struct {
	Name               string `json:"name"`
	Description        string `json:"description"`
	DefaultDurationMin int    `json:"defaultDurationMin"`
	DefaultCreditCost  int    `json:"defaultCreditCost"`
	DefaultCapacity    int    `json:"defaultCapacity"`
	Active             *bool  `json:"active"`
}

func (c *classTypeRequest) Validate() error {
	if len(strings.TrimSpace(c.Name)) < 2 {
		return httpx.Invalid("A class type name of at least 2 characters is required.")
	}
	if c.DefaultDurationMin <= 0 || c.DefaultCreditCost <= 0 || c.DefaultCapacity <= 0 {
		return httpx.Invalid("Duration, credit cost and capacity must all be greater than zero.")
	}
	return nil
}

func (h *Handler) createClassType(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[classTypeRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	classType, err := h.service.CreateClassType(r.Context(), ClassTypeInput{
		Name: body.Name, Description: body.Description,
		DefaultDurationMin: body.DefaultDurationMin,
		DefaultCreditCost:  body.DefaultCreditCost,
		DefaultCapacity:    body.DefaultCapacity,
		Active:             body.Active,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, classType)
}

type classTypePatch struct {
	Name               *string `json:"name"`
	Description        *string `json:"description"`
	DefaultDurationMin *int    `json:"defaultDurationMin"`
	DefaultCreditCost  *int    `json:"defaultCreditCost"`
	DefaultCapacity    *int    `json:"defaultCapacity"`
	Active             *bool   `json:"active"`
}

func (h *Handler) updateClassType(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[classTypePatch](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	classType, err := h.service.UpdateClassType(r.Context(), httpx.Param(r, "id"), ClassTypeInput{
		Name:               deref(body.Name),
		Description:        deref(body.Description),
		DefaultDurationMin: derefInt(body.DefaultDurationMin),
		DefaultCreditCost:  derefInt(body.DefaultCreditCost),
		DefaultCapacity:    derefInt(body.DefaultCapacity),
		Active:             body.Active,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, classType)
}

func (h *Handler) deleteClassType(w http.ResponseWriter, r *http.Request) {
	classTypeID := httpx.Param(r, "id")
	sessions, err := h.usage.SessionsForClassType(r.Context(), classTypeID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.DeleteClassType(r.Context(), classTypeID, sessions, actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, okResponse{OK: true})
}

// ── Packages ─────────────────────────────────────────────────────────────────

// publicPackageView adds readable coverage names, so the member app can say
// which classes a restricted package unlocks.
type publicPackageView struct {
	domain.CreditPackage
	CoverageNames []string `json:"coverageNames"`
}

func (h *Handler) listPublicPackages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	packages, err := h.service.Packages(ctx, true)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	names, err := h.classTypeNames(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	views := make([]publicPackageView, 0, len(packages))
	for _, p := range packages {
		views = append(views, publicPackageView{CreditPackage: p, CoverageNames: coverageNames(p, names)})
	}
	httpx.OK(w, views)
}

// packageStatsView is the admin list: the package plus how it has sold.
type packageStatsView struct {
	Package       domain.CreditPackage `json:"pkg"`
	PurchaseCount int                  `json:"purchaseCount"`
	RevenueIDR    int64                `json:"revenueIdr"`
	CoverageNames []string             `json:"coverageNames"`
}

func (h *Handler) listAdminPackages(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	packages, err := h.service.Packages(ctx, false)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	stats, err := h.usage.PackageStats(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	names, err := h.classTypeNames(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	views := make([]packageStatsView, 0, len(packages))
	for _, p := range packages {
		stat := stats[p.ID]
		views = append(views, packageStatsView{
			Package:       p,
			PurchaseCount: stat.PurchaseCount,
			RevenueIDR:    stat.RevenueIDR,
			CoverageNames: coverageNames(p, names),
		})
	}
	httpx.OK(w, views)
}

func (h *Handler) classTypeNames(ctx context.Context) (map[string]string, error) {
	types, err := h.service.ClassTypes(ctx, false)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(types))
	for _, t := range types {
		names[t.ID] = t.Name
	}
	return names, nil
}

// coverageNames turns a coverage id list into names; nil stays nil, which the
// clients read as "every class".
func coverageNames(p domain.CreditPackage, names map[string]string) []string {
	if p.ApplicableClassTypeIDs == nil {
		return nil
	}
	out := make([]string, 0, len(p.ApplicableClassTypeIDs))
	for _, id := range p.ApplicableClassTypeIDs {
		if name, ok := names[id]; ok {
			out = append(out, name)
		}
	}
	return out
}

type packageRequest struct {
	Name                   string   `json:"name"`
	Credits                int      `json:"credits"`
	PriceIDR               int64    `json:"priceIdr"`
	ValidityDays           int      `json:"validityDays"`
	BranchID               *string  `json:"branchId"`
	PurchaseLimitPerMember *int     `json:"purchaseLimitPerMember"`
	ApplicableClassTypeIDs []string `json:"applicableClassTypeIds"`
	Status                 string   `json:"status"`
}

func (p *packageRequest) Validate() error {
	if len(strings.TrimSpace(p.Name)) < 2 {
		return httpx.Invalid("A package name of at least 2 characters is required.")
	}
	if p.Credits <= 0 {
		return httpx.Invalid("Credits must be greater than zero.")
	}
	if p.PriceIDR <= 0 {
		return httpx.Invalid("Price must be greater than zero.")
	}
	if p.ValidityDays <= 0 {
		return httpx.Invalid("Validity days must be greater than zero.")
	}
	return nil
}

func (h *Handler) createPackage(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[packageRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	pkg, err := h.service.CreatePackage(r.Context(), PackageInput{
		Name: body.Name, Credits: body.Credits, PriceIDR: body.PriceIDR,
		ValidityDays: body.ValidityDays, BranchID: body.BranchID,
		PurchaseLimitPerMember: body.PurchaseLimitPerMember,
		ApplicableClassTypeIDs: body.ApplicableClassTypeIDs,
		Status:                 domain.PackageStatus(body.Status),
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, pkg)
}

type packagePatch struct {
	Name                   *string   `json:"name"`
	Credits                *int      `json:"credits"`
	PriceIDR               *int64    `json:"priceIdr"`
	ValidityDays           *int      `json:"validityDays"`
	BranchID               *string   `json:"branchId"`
	PurchaseLimitPerMember *int      `json:"purchaseLimitPerMember"`
	ApplicableClassTypeIDs *[]string `json:"applicableClassTypeIds"`
	Status                 *string   `json:"status"`
}

func (h *Handler) updatePackage(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[packagePatch](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := PackageInput{
		Name:                   deref(body.Name),
		Credits:                derefInt(body.Credits),
		ValidityDays:           derefInt(body.ValidityDays),
		BranchID:               body.BranchID,
		PurchaseLimitPerMember: body.PurchaseLimitPerMember,
		Status:                 domain.PackageStatus(deref(body.Status)),
	}
	if body.PriceIDR != nil {
		in.PriceIDR = *body.PriceIDR
	}
	setCoverage := body.ApplicableClassTypeIDs != nil
	if setCoverage {
		in.ApplicableClassTypeIDs = *body.ApplicableClassTypeIDs
	}

	pkg, err := h.service.UpdatePackage(r.Context(), httpx.Param(r, "id"), in, setCoverage, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, pkg)
}

func (h *Handler) deletePackage(w http.ResponseWriter, r *http.Request) {
	packageID := httpx.Param(r, "id")
	payments, err := h.usage.PaymentsForPackage(r.Context(), packageID)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.DeletePackage(r.Context(), packageID, payments, actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, okResponse{OK: true})
}

// ── Rules ────────────────────────────────────────────────────────────────────

func (h *Handler) getRules(w http.ResponseWriter, r *http.Request) {
	view, err := h.service.RulesView(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, view)
}

func (h *Handler) updateRules(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[domain.RulesOverride](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := validateRulesPatch(body); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	rules, err := h.service.UpdateRules(r.Context(), body, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, rules)
}

// validateRulesPatch keeps configuration inside sane bounds: a QR that lives
// for an hour or a negative grace window would quietly break the gate.
func validateRulesPatch(p domain.RulesOverride) error {
	if p.QRTTLSeconds != nil && (*p.QRTTLSeconds < 10 || *p.QRTTLSeconds > 300) {
		return httpx.Invalid("QR lifetime must be between 10 and 300 seconds.")
	}
	if p.DefaultCreditExpiryDays != nil && *p.DefaultCreditExpiryDays <= 0 {
		return httpx.Invalid("Credit expiry days must be greater than zero.")
	}
	if p.CancellationDeadlineHours != nil && *p.CancellationDeadlineHours < 0 {
		return httpx.Invalid("Cancellation deadline cannot be negative.")
	}
	if p.ReEntryGraceMinutes != nil && *p.ReEntryGraceMinutes < 0 {
		return httpx.Invalid("Re-entry grace cannot be negative.")
	}
	if p.AntiPassbackMinutes != nil && *p.AntiPassbackMinutes < 0 {
		return httpx.Invalid("Anti-passback window cannot be negative.")
	}
	// The grace window sits inside the anti-passback window; inverting them
	// would make re-entry unreachable.
	if p.ReEntryGraceMinutes != nil && p.AntiPassbackMinutes != nil &&
		*p.ReEntryGraceMinutes > *p.AntiPassbackMinutes {
		return httpx.Invalid("Re-entry grace must not exceed the anti-passback window.")
	}
	if p.LateCancellationPolicy != nil && *p.LateCancellationPolicy != domain.PolicyForfeit && *p.LateCancellationPolicy != domain.PolicyFree {
		return httpx.Invalid("Late cancellation policy must be FORFEIT or FREE.")
	}
	if p.NoShowPolicy != nil && *p.NoShowPolicy != domain.PolicyForfeit && *p.NoShowPolicy != domain.PolicyFree {
		return httpx.Invalid("No-show policy must be FORFEIT or FREE.")
	}
	if p.ExpiryReminderDays != nil && *p.ExpiryReminderDays < 1 {
		return httpx.Invalid("Expiry reminder days must be at least 1.")
	}
	return nil
}

// ── Exercises ────────────────────────────────────────────────────────────────

func (h *Handler) listExercises(w http.ResponseWriter, r *http.Request) {
	exercises, err := h.service.Exercises(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, exercises)
}

// exerciseLibraryView is what the member Guides tab and workout generator read.
type exerciseLibraryView struct {
	Exercises     []domain.Exercise         `json:"exercises"`
	Substitutions []domain.SubstitutionRule `json:"substitutions"`
}

func (h *Handler) exerciseLibrary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	exercises, err := h.service.Exercises(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	substitutions, err := h.service.Substitutions(ctx)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, exerciseLibraryView{Exercises: exercises, Substitutions: substitutions})
}

type exercisePatch struct {
	Name       *string `json:"name"`
	Difficulty *int    `json:"difficulty"`
	VideoURL   *string `json:"videoUrl"`
}

func (h *Handler) updateExercise(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[exercisePatch](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if body.Difficulty != nil && (*body.Difficulty < 1 || *body.Difficulty > 3) {
		httpx.Fail(w, r, httpx.Invalid("Difficulty must be 1, 2 or 3."))
		return
	}
	exercise, err := h.service.UpdateExercise(r.Context(), httpx.Param(r, "id"),
		deref(body.Name), derefInt(body.Difficulty), body.VideoURL, true, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, exercise)
}

type okResponse struct {
	OK bool `json:"ok"`
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func derefInt(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
