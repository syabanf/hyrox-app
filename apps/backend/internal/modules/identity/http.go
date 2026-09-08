package identity

import (
	"net/http"
	"strings"
	"time"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/auth"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Handler serves the identity module's HTTP surface.
type Handler struct {
	service *Service
	guard   *auth.Guard
	// demoRoster exposes the staff list on the login screen. It is a demo
	// affordance: with it off, the roster requires authentication.
	demoRoster bool
}

func NewHandler(service *Service, guard *auth.Guard, demoRoster bool) *Handler {
	return &Handler{service: service, guard: guard, demoRoster: demoRoster}
}

func (h *Handler) Mount(r *httpx.Router) {
	// Member sign-in and registration.
	r.Post("/api/auth/otp/request", h.requestOTP)
	r.Post("/api/auth/otp/verify", h.verifyOTP)
	r.Post("/api/auth/register", h.register)

	// Staff sign-in. The roster is only public in demo mode, where the login
	// screen is a role picker; otherwise listing staff needs a session.
	if h.demoRoster {
		r.Get("/api/admin/auth/users", h.listAdminUsers)
	} else {
		r.Get("/api/admin/auth/users", h.listAdminUsers, h.guard.RequireAdmin(string(domain.PermConfigView)))
	}
	r.Post("/api/admin/auth/login", h.adminLogin)
	// What the login screen is allowed to offer. Public, because it is read
	// before anybody has a token.
	r.Get("/api/admin/auth/mode", h.authMode)
	// Changing your own password needs a session, not a permission: everybody
	// has one to change.
	r.Post("/api/admin/auth/password", h.changeOwnPassword, h.guard.RequireAnyAdmin)

	// Member self-service.
	r.Get("/api/me/profile", h.getProfile, h.guard.RequireMember)
	r.Patch("/api/me", h.updateProfile, h.guard.RequireMember)

	// Staff-managed membership records.
	admin := func(p domain.Permission) httpx.Middleware { return h.guard.RequireAdmin(string(p)) }
	r.Post("/api/admin/members", h.createMember, admin(domain.PermMembersManage))
	r.Patch("/api/admin/members/{id}", h.updateMember, admin(domain.PermMembersManage))

	r.Post("/api/admin/users", h.createAdminUser, admin(domain.PermUsersManage))
	r.Patch("/api/admin/users/{id}", h.updateAdminUser, admin(domain.PermUsersManage))
	r.Delete("/api/admin/users/{id}", h.deleteAdminUser, admin(domain.PermUsersManage))
	// A supervisor's PIN for authorising at a till. Managing logins is the
	// grant, because a PIN is a credential like any other.
	r.Put("/api/admin/users/{id}/supervisor-pin", h.setSupervisorPIN,
		admin(domain.PermUsersManage))
	// Handing somebody a password, for a new starter or a lockout.
	r.Put("/api/admin/users/{id}/password", h.setAdminPassword,
		admin(domain.PermUsersManage))
}

func actorFrom(r *http.Request) Actor {
	principal, _ := auth.Admin(r.Context())
	return Actor{ID: principal.ID, Name: principal.Role}
}

// ── Sign-in ──────────────────────────────────────────────────────────────────

type otpRequest struct {
	Identifier string `json:"identifier"`
}

func (o *otpRequest) Validate() error {
	if len(strings.TrimSpace(o.Identifier)) < 3 {
		return httpx.Invalid("Enter an email address or phone number.")
	}
	return nil
}

func (h *Handler) requestOTP(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[otpRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	challenge, err := h.service.RequestOTP(r.Context(), body.Identifier)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, challenge)
}

type otpVerifyRequest struct {
	ChallengeID string `json:"challengeId"`
	Code        string `json:"code"`
}

func (o *otpVerifyRequest) Validate() error {
	if o.ChallengeID == "" {
		return httpx.Invalid("A challenge id is required.")
	}
	if len(o.Code) < 4 || len(o.Code) > 8 {
		return httpx.Invalid("Enter the code from your message.")
	}
	return nil
}

func (h *Handler) verifyOTP(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[otpVerifyRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	session, err := h.service.VerifyOTP(r.Context(), body.ChallengeID, body.Code)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, session)
}

type registerRequest struct {
	FullName          string                   `json:"fullName"`
	Email             string                   `json:"email"`
	Phone             string                   `json:"phone"`
	DateOfBirth       *time.Time               `json:"dateOfBirth"`
	Gender            *string                  `json:"gender"`
	EmergencyContact  *domain.EmergencyContact `json:"emergencyContact"`
	PreferredBranchID *string                  `json:"preferredBranchId"`
	WaiverAccepted    bool                     `json:"waiverAccepted"`
	TermsAccepted     bool                     `json:"termsAccepted"`
}

func (rq *registerRequest) Validate() error {
	if len(strings.TrimSpace(rq.FullName)) < 2 {
		return httpx.Invalid("Enter your full name.")
	}
	if !strings.Contains(rq.Email, "@") {
		return httpx.Invalid("Enter a valid email address.")
	}
	if len(strings.TrimSpace(rq.Phone)) < 6 {
		return httpx.Invalid("Enter a valid phone number.")
	}
	if rq.Gender != nil && !isGender(*rq.Gender) {
		return httpx.Invalid("Gender must be MALE, FEMALE or OTHER.")
	}
	if rq.EmergencyContact != nil {
		c := rq.EmergencyContact
		if strings.TrimSpace(c.Name) == "" || len(strings.TrimSpace(c.Phone)) < 6 || strings.TrimSpace(c.Relation) == "" {
			return httpx.Invalid("An emergency contact needs a name, phone number and relation.")
		}
	}
	if !rq.WaiverAccepted || !rq.TermsAccepted {
		return httpx.Invalid("The waiver and terms must both be accepted.")
	}
	return nil
}

func isGender(value string) bool {
	switch domain.Gender(value) {
	case domain.GenderMale, domain.GenderFemale, domain.GenderOther:
		return true
	}
	return false
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[registerRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	var gender *domain.Gender
	if body.Gender != nil {
		g := domain.Gender(*body.Gender)
		gender = &g
	}
	session, err := h.service.Register(r.Context(), RegistrationInput{
		FullName:          body.FullName,
		Email:             body.Email,
		Phone:             body.Phone,
		DateOfBirth:       body.DateOfBirth,
		Gender:            gender,
		EmergencyContact:  body.EmergencyContact,
		PreferredBranchID: body.PreferredBranchID,
		WaiverAccepted:    body.WaiverAccepted,
		TermsAccepted:     body.TermsAccepted,
	})
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, session)
}

func (h *Handler) listAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.service.AdminUsers(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, users)
}

type adminLoginRequest struct {
	// UserID is the demo role picker; email and password are the real login.
	UserID   string `json:"userId"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *adminLoginRequest) Validate() error {
	if a.UserID == "" && strings.TrimSpace(a.Email) == "" {
		return httpx.Invalid("Enter your email address and password.")
	}
	if a.UserID == "" && a.Password == "" {
		return httpx.Invalid("Enter your password.")
	}
	return nil
}

func (h *Handler) adminLogin(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[adminLoginRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	session, err := h.service.AdminLogin(r.Context(), body.UserID, body.Email, body.Password)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, session)
}

func (h *Handler) authMode(w http.ResponseWriter, r *http.Request) {
	mode, err := h.service.AuthMode(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, mode)
}

// ── Passwords ────────────────────────────────────────────────────────────────

type setPasswordBody struct {
	Password string `json:"password"`
}

func (h *Handler) setAdminPassword(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[setPasswordBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.SetAdminPassword(r.Context(), httpx.Param(r, "id"), body.Password, actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"set": true})
}

type changePasswordBody struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (h *Handler) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[changePasswordBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	principal, _ := auth.Admin(r.Context())
	if err := h.service.ChangeOwnPassword(r.Context(), principal.ID, body.CurrentPassword, body.NewPassword); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"changed": true})
}

// ── Member profile ───────────────────────────────────────────────────────────

func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	member, err := h.service.Member(r.Context(), auth.MemberID(r.Context()))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, member)
}

// profilePatch models "field absent" and "field set to null" separately: the
// pointer-to-pointer fields let a member clear their emergency contact without
// clearing it every time they change their name.
type profilePatch struct {
	FullName          *string                   `json:"fullName"`
	Email             *string                   `json:"email"`
	Phone             *string                   `json:"phone"`
	EmergencyContact  **domain.EmergencyContact `json:"emergencyContact"`
	PreferredBranchID **string                  `json:"preferredBranchId"`
	AvatarURL         **string                  `json:"avatarUrl"`
}

func (p *profilePatch) Validate() error {
	if p.FullName != nil && len(strings.TrimSpace(*p.FullName)) < 2 {
		return httpx.Invalid("Enter your full name.")
	}
	if p.Email != nil && !strings.Contains(*p.Email, "@") {
		return httpx.Invalid("Enter a valid email address.")
	}
	if p.Phone != nil && len(strings.TrimSpace(*p.Phone)) < 6 {
		return httpx.Invalid("Enter a valid phone number.")
	}
	// Avatars are data URLs; a very large one would bloat every profile read.
	if p.AvatarURL != nil && *p.AvatarURL != nil && len(**p.AvatarURL) > 300_000 {
		return httpx.Invalid("That image is too large.")
	}
	return nil
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[profilePatch](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := ProfileInput{
		FullName: body.FullName,
		Email:    body.Email,
		Phone:    body.Phone,
	}
	if body.EmergencyContact != nil {
		in.SetContact = true
		in.EmergencyContact = *body.EmergencyContact
	}
	if body.PreferredBranchID != nil {
		in.SetBranch = true
		in.PreferredBranchID = *body.PreferredBranchID
	}
	if body.AvatarURL != nil {
		in.SetAvatar = true
		in.AvatarURL = *body.AvatarURL
	}

	member, err := h.service.UpdateProfile(r.Context(), auth.MemberID(r.Context()), in)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, member)
}

// ── Staff-managed members ────────────────────────────────────────────────────

type createMemberRequest struct {
	FullName          string  `json:"fullName"`
	Email             string  `json:"email"`
	Phone             string  `json:"phone"`
	PreferredBranchID *string `json:"preferredBranchId"`
	Notes             *string `json:"notes"`
}

func (c *createMemberRequest) Validate() error {
	if len(strings.TrimSpace(c.FullName)) < 2 {
		return httpx.Invalid("A member name of at least 2 characters is required.")
	}
	if !strings.Contains(c.Email, "@") {
		return httpx.Invalid("A valid email address is required.")
	}
	if len(strings.TrimSpace(c.Phone)) < 6 {
		return httpx.Invalid("A valid phone number is required.")
	}
	return nil
}

func (h *Handler) createMember(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[createMemberRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	member, err := h.service.CreateMemberAsAdmin(r.Context(), AdminMemberInput{
		FullName:          body.FullName,
		Email:             body.Email,
		Phone:             body.Phone,
		PreferredBranchID: body.PreferredBranchID,
		Notes:             body.Notes,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, member)
}

type memberPatch struct {
	Status            *string  `json:"status"`
	Notes             **string `json:"notes"`
	PreferredBranchID **string `json:"preferredBranchId"`
	Reason            *string  `json:"reason"`
}

func (m *memberPatch) Validate() error {
	if m.Status != nil && !domain.IsValidMemberStatus(*m.Status) {
		return httpx.Invalid("Status must be ACTIVE, SUSPENDED, INACTIVE or ARCHIVED.")
	}
	return nil
}

func (h *Handler) updateMember(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[memberPatch](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := AdminUpdateInput{Reason: body.Reason}
	if body.Status != nil {
		status := domain.MemberStatus(*body.Status)
		in.Status = &status
	}
	if body.Notes != nil {
		in.SetNotes = true
		in.Notes = *body.Notes
	}
	if body.PreferredBranchID != nil {
		in.SetBranch = true
		in.PreferredBranchID = *body.PreferredBranchID
	}

	member, err := h.service.UpdateMemberAsAdmin(r.Context(), httpx.Param(r, "id"), in, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, member)
}

// ── Staff accounts ───────────────────────────────────────────────────────────

type adminUserRequest struct {
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Role     string  `json:"role"`
	BranchID *string `json:"branchId"`
}

func (a *adminUserRequest) Validate() error {
	if len(strings.TrimSpace(a.Name)) < 2 {
		return httpx.Invalid("A name of at least 2 characters is required.")
	}
	if !strings.Contains(a.Email, "@") {
		return httpx.Invalid("A valid email address is required.")
	}
	if !domain.IsAdminRole(a.Role) {
		return httpx.Invalid("That role is not recognized.")
	}
	return nil
}

func (h *Handler) createAdminUser(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[adminUserRequest](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	user, err := h.service.CreateAdminUser(r.Context(), AdminUserInput{
		Name: body.Name, Email: body.Email,
		Role: domain.AdminRole(body.Role), BranchID: body.BranchID,
	}, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.Created(w, user)
}

type adminUserPatch struct {
	Name     *string  `json:"name"`
	Email    *string  `json:"email"`
	Role     *string  `json:"role"`
	BranchID **string `json:"branchId"`
}

func (a *adminUserPatch) Validate() error {
	if a.Role != nil && !domain.IsAdminRole(*a.Role) {
		return httpx.Invalid("That role is not recognized.")
	}
	if a.Email != nil && !strings.Contains(*a.Email, "@") {
		return httpx.Invalid("A valid email address is required.")
	}
	return nil
}

func (h *Handler) updateAdminUser(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[adminUserPatch](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	in := AdminUserInput{}
	if body.Name != nil {
		in.Name = *body.Name
	}
	if body.Email != nil {
		in.Email = *body.Email
	}
	if body.Role != nil {
		in.Role = domain.AdminRole(*body.Role)
	}
	setBranch := body.BranchID != nil
	if setBranch {
		in.BranchID = *body.BranchID
	}

	user, err := h.service.UpdateAdminUser(r.Context(), httpx.Param(r, "id"), in, setBranch, actorFrom(r))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, user)
}

func (h *Handler) deleteAdminUser(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteAdminUser(r.Context(), httpx.Param(r, "id"), actorFrom(r)); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"ok": true})
}

type supervisorPINBody struct {
	// Empty takes the PIN away, which is how somebody stops being able to
	// authorise at a till without their login changing.
	PIN string `json:"pin"`
}

func (h *Handler) setSupervisorPIN(w http.ResponseWriter, r *http.Request) {
	body, err := httpx.Decode[supervisorPINBody](r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.service.SetSupervisorPIN(r.Context(), httpx.Param(r, "id"), body.PIN); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, map[string]bool{"set": body.PIN != ""})
}
