// Package identity owns accounts: studio members, staff users, and the
// sign-in flows that turn a phone number or a role card into a bearer token.
package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/audit"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/auth"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/clock"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/config"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/id"
)

// WaiverVersion is the liability waiver members accept at registration. Bump
// it when the wording changes: past acceptances stay pinned to the version
// they actually agreed to.
const WaiverVersion = "2026-01"

// maxOTPAttempts caps guesses per challenge, so a six-digit code cannot be
// brute-forced.
const maxOTPAttempts = 5

// Service implements the identity use cases.
type Service struct {
	repo      *Repository
	ids       id.Generator
	clock     clock.Clock
	issuer    *auth.Issuer
	passwords *auth.Passwords
	auditor   audit.Recorder
	cfg       config.Auth
}

func NewService(repo *Repository, ids id.Generator, c clock.Clock, issuer *auth.Issuer, auditor audit.Recorder, cfg config.Auth) *Service {
	return &Service{
		repo:      repo,
		ids:       ids,
		clock:     c,
		issuer:    issuer,
		passwords: auth.NewPasswords(cfg.PasswordIterations),
		auditor:   auditor,
		cfg:       cfg,
	}
}

// ── Member sign-in ───────────────────────────────────────────────────────────

// OTPChallenge is what the client needs to complete a sign-in.
type OTPChallenge struct {
	ChallengeID  string `json:"challengeId"`
	MemberExists bool   `json:"memberExists"`
	Hint         string `json:"hint"`
	// Code is only populated in demo mode, so the demo can show the code
	// on screen. It is never set when real delivery is configured.
	Code string `json:"code,omitempty"`
}

// RequestOTP issues a one-time code for an email or phone number.
//
// The response never reveals whether the identifier is registered beyond the
// memberExists flag the demo relies on; in production that flag should be
// dropped so the endpoint cannot be used to enumerate members.
func (s *Service) RequestOTP(ctx context.Context, identifier string) (OTPChallenge, error) {
	identifier = strings.TrimSpace(identifier)
	if len(identifier) < 3 {
		return OTPChallenge{}, httpx.Invalid("Enter an email address or phone number.")
	}

	var memberID *string
	member, err := s.repo.MemberByIdentifier(ctx, identifier)
	if err == nil {
		memberID = &member.ID
	}

	code, err := s.generateCode()
	if err != nil {
		return OTPChallenge{}, err
	}
	challenge := Challenge{
		ID:         s.ids.New(id.OTP),
		Identifier: identifier,
		CodeHash:   s.hashCode(identifier, code),
		MemberID:   memberID,
		ExpiresAt:  s.clock.Now().Add(s.cfg.OTPTTL),
	}
	if err := s.repo.InsertChallenge(ctx, challenge); err != nil {
		return OTPChallenge{}, err
	}

	out := OTPChallenge{
		ChallengeID:  challenge.ID,
		MemberExists: memberID != nil,
		Hint:         fmt.Sprintf("We sent a %d-digit code, valid for %d minutes.", s.cfg.OTPLength, int(s.cfg.OTPTTL.Minutes())),
	}
	if s.cfg.DemoOTP {
		// Demo mode has no SMS provider, so the code comes back in the
		// response instead of a text message.
		out.Code = code
		out.Hint = fmt.Sprintf("Demo mode: your code is %s.", code)
	}
	return out, nil
}

// Session is a signed-in member.
type Session struct {
	Token   string        `json:"token"`
	Member  domain.Member `json:"member"`
	Expires time.Time     `json:"expiresAt"`
}

// VerifyOTP exchanges a challenge and code for a member token.
func (s *Service) VerifyOTP(ctx context.Context, challengeID, code string) (Session, error) {
	challenge, err := s.repo.Challenge(ctx, challengeID)
	if err != nil {
		return Session{}, err
	}
	now := s.clock.Now()

	switch {
	case challenge.ConsumedAt != nil:
		return Session{}, httpx.ErrBadRequest.WithMessage("That code has already been used. Request a new one.")
	case now.After(challenge.ExpiresAt):
		return Session{}, httpx.ErrBadRequest.WithMessage("That code has expired. Request a new one.")
	case challenge.Attempts >= maxOTPAttempts:
		return Session{}, httpx.ErrRateLimited.WithMessage("Too many incorrect codes. Request a new one.")
	}

	if !s.codeMatches(challenge, code) {
		if err := s.repo.RecordAttempt(ctx, challenge.ID); err != nil {
			return Session{}, err
		}
		return Session{}, httpx.ErrUnauthorized.WithMessage("That code is not correct.")
	}

	// Burning the challenge before issuing the token closes the window where
	// two concurrent requests could both redeem the same code.
	consumed, err := s.repo.ConsumeChallenge(ctx, challenge.ID, now)
	if err != nil {
		return Session{}, err
	}
	if !consumed {
		return Session{}, httpx.ErrBadRequest.WithMessage("That code has already been used. Request a new one.")
	}

	if challenge.MemberID == nil {
		return Session{}, httpx.NotFound("member").
			WithMessage("No membership found for that contact. Create one to continue.")
	}
	member, err := s.repo.Member(ctx, *challenge.MemberID)
	if err != nil {
		return Session{}, err
	}
	if member.Status == domain.MemberArchived {
		return Session{}, httpx.ErrForbidden.WithMessage("That membership is closed.")
	}
	return s.sessionFor(member)
}

func (s *Service) sessionFor(member domain.Member) (Session, error) {
	token, expires, err := s.issuer.Issue(auth.Principal{
		ID:   member.ID,
		Kind: auth.KindMember,
	}, s.clock.Now())
	if err != nil {
		return Session{}, err
	}
	return Session{Token: token, Member: member, Expires: expires}, nil
}

// generateCode draws a numeric code from a cryptographic source.
func (s *Service) generateCode() (string, error) {
	digits := s.cfg.OTPLength
	if digits < 4 {
		digits = 6
	}
	code := make([]byte, digits)
	for i := range code {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("identity: generating code: %w", err)
		}
		code[i] = byte('0' + n.Int64())
	}
	return string(code), nil
}

// hashCode stores codes keyed by the identifier, so a leaked database yields
// no usable sign-in codes.
func (s *Service) hashCode(identifier, code string) string {
	mac := hmac.New(sha256.New, s.cfg.Secret)
	mac.Write([]byte(strings.ToLower(identifier) + ":" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) codeMatches(challenge Challenge, code string) bool {
	code = strings.TrimSpace(code)
	if code == "" {
		return false
	}
	// Demo mode accepts any well-formed code so a walkthrough never stalls on
	// an SMS that is not coming. It is refused in production by config.
	if s.cfg.DemoOTP && len(code) >= 4 && len(code) <= 8 {
		return true
	}
	expected := s.hashCode(challenge.Identifier, code)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(challenge.CodeHash)) == 1
}

// ── Registration ─────────────────────────────────────────────────────────────

// RegistrationInput is a self-service sign-up.
type RegistrationInput struct {
	FullName          string
	Email             string
	Phone             string
	DateOfBirth       *time.Time
	Gender            *domain.Gender
	EmergencyContact  *domain.EmergencyContact
	PreferredBranchID *string
	WaiverAccepted    bool
	TermsAccepted     bool
}

// Register creates a member and signs them in.
func (s *Service) Register(ctx context.Context, in RegistrationInput) (Session, error) {
	if !in.WaiverAccepted || !in.TermsAccepted {
		return Session{}, httpx.Invalid("The waiver and terms must both be accepted.")
	}
	now := s.clock.Now()
	member := domain.Member{
		ID:                s.ids.New(id.Member),
		FullName:          strings.TrimSpace(in.FullName),
		Email:             strings.ToLower(strings.TrimSpace(in.Email)),
		Phone:             strings.TrimSpace(in.Phone),
		DateOfBirth:       in.DateOfBirth,
		Gender:            in.Gender,
		EmergencyContact:  in.EmergencyContact,
		PreferredBranchID: in.PreferredBranchID,
		Status:            domain.MemberActive,
		// The accepted version is recorded, not just the fact of acceptance,
		// so a later revision is not treated as already agreed.
		WaiverVersion:    strPtr(WaiverVersion),
		WaiverAcceptedAt: &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	created, err := s.repo.InsertMember(ctx, member)
	if err != nil {
		return Session{}, err
	}
	if err := s.auditor.Record(ctx, audit.Event{
		EntityType: "member", EntityID: created.ID, Action: "register",
		NewValue: audit.Str(created.FullName), ActorID: created.ID, ActorName: created.FullName,
	}); err != nil {
		return Session{}, err
	}
	return s.sessionFor(created)
}

// ── Member profile ───────────────────────────────────────────────────────────

func (s *Service) Member(ctx context.Context, id string) (domain.Member, error) {
	return s.repo.Member(ctx, id)
}

func (s *Service) Members(ctx context.Context, filter MemberFilter) ([]domain.Member, error) {
	return s.repo.Members(ctx, filter)
}

func (s *Service) MembersByIDs(ctx context.Context, ids []string) (map[string]domain.Member, error) {
	return s.repo.MembersByIDs(ctx, ids)
}

func (s *Service) MemberCounts(ctx context.Context) (map[string]int, error) {
	return s.repo.CountMembers(ctx)
}

// ProfileInput is a member editing their own details.
type ProfileInput struct {
	FullName          *string
	Email             *string
	Phone             *string
	EmergencyContact  *domain.EmergencyContact
	SetContact        bool
	PreferredBranchID *string
	SetBranch         bool
	AvatarURL         *string
	SetAvatar         bool
}

func (s *Service) UpdateProfile(ctx context.Context, memberID string, in ProfileInput) (domain.Member, error) {
	member, err := s.repo.Member(ctx, memberID)
	if err != nil {
		return domain.Member{}, err
	}
	if in.FullName != nil {
		member.FullName = strings.TrimSpace(*in.FullName)
	}
	if in.Email != nil {
		member.Email = strings.ToLower(strings.TrimSpace(*in.Email))
	}
	if in.Phone != nil {
		member.Phone = strings.TrimSpace(*in.Phone)
	}
	if in.SetContact {
		member.EmergencyContact = in.EmergencyContact
	}
	if in.SetBranch {
		member.PreferredBranchID = in.PreferredBranchID
	}
	if in.SetAvatar {
		member.AvatarURL = in.AvatarURL
	}
	member.UpdatedAt = s.clock.Now()
	return s.repo.UpdateMember(ctx, member)
}

// AdminMemberInput is the front desk creating a membership.
type AdminMemberInput struct {
	FullName          string
	Email             string
	Phone             string
	PreferredBranchID *string
	Notes             *string
}

func (s *Service) CreateMemberAsAdmin(ctx context.Context, in AdminMemberInput, actor Actor) (domain.Member, error) {
	now := s.clock.Now()
	member := domain.Member{
		ID:                s.ids.New(id.Member),
		FullName:          strings.TrimSpace(in.FullName),
		Email:             strings.ToLower(strings.TrimSpace(in.Email)),
		Phone:             strings.TrimSpace(in.Phone),
		PreferredBranchID: in.PreferredBranchID,
		Notes:             in.Notes,
		Status:            domain.MemberActive,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	created, err := s.repo.InsertMember(ctx, member)
	if err != nil {
		return domain.Member{}, err
	}
	if err := s.auditor.Record(ctx, audit.Event{
		EntityType: "member", EntityID: created.ID, Action: "create",
		NewValue: audit.Str(created.FullName), ActorID: actor.ID, ActorName: actor.Name,
	}); err != nil {
		return domain.Member{}, err
	}
	return created, nil
}

// AdminUpdateInput is a staff edit of a membership.
type AdminUpdateInput struct {
	Status            *domain.MemberStatus
	Notes             *string
	SetNotes          bool
	PreferredBranchID *string
	SetBranch         bool
	Reason            *string
}

// UpdateMemberAsAdmin applies staff edits. A status change goes through the
// member state machine and is always audited, because suspending or archiving
// someone is an action a studio needs to be able to account for.
func (s *Service) UpdateMemberAsAdmin(ctx context.Context, memberID string, in AdminUpdateInput, actor Actor) (domain.Member, error) {
	member, err := s.repo.Member(ctx, memberID)
	if err != nil {
		return domain.Member{}, err
	}
	previousStatus := member.Status

	if in.Status != nil && *in.Status != member.Status {
		next, err := domain.Transition(domain.MemberTransitions, member.Status, *in.Status)
		if err != nil {
			return domain.Member{}, httpx.Conflict("INVALID_TRANSITION",
				"A %s member cannot become %s.", member.Status, *in.Status)
		}
		member.Status = next
	}
	if in.SetNotes {
		member.Notes = in.Notes
	}
	if in.SetBranch {
		member.PreferredBranchID = in.PreferredBranchID
	}
	member.UpdatedAt = s.clock.Now()

	updated, err := s.repo.UpdateMember(ctx, member)
	if err != nil {
		return domain.Member{}, err
	}
	if updated.Status != previousStatus {
		if err := s.auditor.Record(ctx, audit.Event{
			EntityType:    "member",
			EntityID:      memberID,
			Action:        "status_change",
			PreviousValue: audit.Str(string(previousStatus)),
			NewValue:      audit.Str(string(updated.Status)),
			ActorID:       actor.ID,
			ActorName:     actor.Name,
			Reason:        in.Reason,
		}); err != nil {
			return domain.Member{}, err
		}
	}
	return updated, nil
}

// ── Staff accounts ───────────────────────────────────────────────────────────

// Actor identifies the staff member behind an action.
type Actor struct {
	ID   string
	Name string
}

// AdminSession is a signed-in staff user plus what they may do.
type AdminSession struct {
	Token       string              `json:"token"`
	User        domain.AdminUser    `json:"user"`
	Permissions []domain.Permission `json:"permissions"`
	Expires     time.Time           `json:"expiresAt"`
	// MustChangePassword is set when the password was handed over by somebody
	// else. The panel keeps the user on the change-password screen until they
	// have replaced it.
	MustChangePassword bool `json:"mustChangePassword"`
}

func (s *Service) AdminUsers(ctx context.Context) ([]domain.AdminUser, error) {
	return s.repo.AdminUsers(ctx)
}

// AdminLogin signs a staff user in.
//
// The normal path is an email address and a password. Demo mode adds a second
// path — a user id and nothing else — which is what the role-picker cards on
// the login screen send; it is refused outright anywhere demo mode is off, so
// a production deployment has exactly one way in.
func (s *Service) AdminLogin(ctx context.Context, userID, email, password string) (AdminSession, error) {
	if userID != "" {
		if !s.cfg.DemoOTP {
			return AdminSession{}, httpx.ErrUnauthorized.
				WithMessage("Sign in with your email address and password.")
		}
		user, err := s.repo.AdminUser(ctx, userID)
		if err != nil {
			return AdminSession{}, err
		}
		return s.adminSession(ctx, user, false)
	}

	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return AdminSession{}, httpx.Invalid("Enter your email address and password.")
	}

	creds, err := s.repo.CredentialsByEmail(ctx, email)
	if err != nil {
		if httpx.IsNotFound(err) {
			// A distinct "no such account" would turn the login form into a
			// staff directory, so an unknown email fails exactly like a wrong
			// password. The hash is still computed, so the two paths take a
			// comparable amount of time.
			s.passwords.Matches(password, decoyHash)
			return AdminSession{}, errBadCredentials
		}
		return AdminSession{}, err
	}

	now := s.clock.Now()
	if creds.LockedUntil != nil && now.Before(*creds.LockedUntil) {
		return AdminSession{}, httpx.ErrRateLimited.WithMessage(
			"Too many incorrect passwords. Try again in %d minutes, or ask for a reset.",
			int(creds.LockedUntil.Sub(now).Minutes())+1)
	}

	if creds.PasswordHash == "" || !s.passwords.Matches(password, creds.PasswordHash) {
		if err := s.recordFailure(ctx, creds, now); err != nil {
			return AdminSession{}, err
		}
		return AdminSession{}, errBadCredentials
	}

	// The cost of hashing goes up over the years; people whose password
	// predates the rise get moved up on the way past, without being asked.
	if s.passwords.NeedsRehash(creds.PasswordHash) {
		if hash, err := s.passwords.Hash(password); err == nil {
			_ = s.repo.SetPassword(ctx, creds.ID, hash, creds.MustChangePassword, now)
		}
	}
	if err := s.repo.RecordLogin(ctx, creds.ID, now); err != nil {
		return AdminSession{}, err
	}
	return s.adminSession(ctx, creds.AdminUser, creds.MustChangePassword)
}

// errBadCredentials is the single answer to a wrong email and a wrong
// password alike.
var errBadCredentials = httpx.ErrUnauthorized.WithMessage("That email address and password do not match.")

// decoyHash gives the unknown-account path the same work to do as the real
// one. The password it encodes is random and nobody knows it.
var decoyHash = "pbkdf2-sha256$" + strconv.Itoa(auth.DefaultPasswordIterations) +
	"$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func (s *Service) recordFailure(ctx context.Context, creds Credentials, now time.Time) error {
	attempts := s.cfg.LoginAttempts
	if attempts < 1 {
		attempts = 10
	}
	// The lock is applied on the attempt that reaches the limit, so the count
	// and the door never disagree.
	var lockUntil *time.Time
	if creds.FailedLogins+1 >= attempts {
		until := now.Add(s.cfg.LockoutFor)
		lockUntil = &until
	}
	_, err := s.repo.RecordFailedLogin(ctx, creds.ID, lockUntil)
	return err
}

func (s *Service) adminSession(ctx context.Context, user domain.AdminUser, mustChange bool) (AdminSession, error) {
	branchID := ""
	if user.BranchID != nil {
		branchID = *user.BranchID
	}
	token, expires, err := s.issuer.Issue(auth.Principal{
		ID:       user.ID,
		Kind:     auth.KindAdmin,
		Role:     string(user.Role),
		BranchID: branchID,
	}, s.clock.Now())
	if err != nil {
		return AdminSession{}, err
	}
	if err := s.auditor.Record(ctx, audit.Event{
		EntityType: "admin_user", EntityID: user.ID, Action: "login",
		NewValue: audit.Str(string(user.Role)), ActorID: user.ID, ActorName: user.Name,
	}); err != nil {
		return AdminSession{}, err
	}
	return AdminSession{
		Token:              token,
		User:               user,
		Permissions:        domain.PermissionsFor(user.Role),
		Expires:            expires,
		MustChangePassword: mustChange,
	}, nil
}

// AuthMode tells the login screen what it is allowed to offer.
type AuthMode struct {
	// DemoRoster is true when the staff list is public and the role cards can
	// sign somebody in without a password.
	DemoRoster bool `json:"demoRoster"`
	// AccountsWithoutPassword is how many staff cannot sign in yet.
	AccountsWithoutPassword int `json:"accountsWithoutPassword"`
	// MinPasswordLength is published so the form and the server agree.
	MinPasswordLength int `json:"minPasswordLength"`
}

func (s *Service) AuthMode(ctx context.Context) (AuthMode, error) {
	mode := AuthMode{DemoRoster: s.cfg.DemoOTP, MinPasswordLength: minPasswordLength}
	if s.cfg.DemoOTP {
		count, err := s.repo.AccountsWithoutPassword(ctx)
		if err != nil {
			return AuthMode{}, err
		}
		mode.AccountsWithoutPassword = count
	}
	return mode, nil
}

// ── Passwords ────────────────────────────────────────────────────────────────

// minPasswordLength is a floor, not a policy. Composition rules ("one capital,
// one symbol") push people towards Passw0rd! and a sticky note; length is the
// part that actually costs an attacker something.
const minPasswordLength = 10

func checkPasswordPolicy(password string) error {
	if len([]rune(password)) < minPasswordLength {
		return httpx.Invalid("A password is at least %d characters.", minPasswordLength)
	}
	if len(password) > 200 {
		return httpx.Invalid("That password is longer than 200 characters.")
	}
	if strings.TrimSpace(password) == "" {
		return httpx.Invalid("A password cannot be only spaces.")
	}
	return nil
}

// SetAdminPassword is somebody with user management handing out a password —
// a new starter, or a reset for a person locked out.
//
// The password is marked for replacement at first use, because whoever typed
// it in knows it, and a credential two people know is not a credential.
func (s *Service) SetAdminPassword(ctx context.Context, userID, password string, actor Actor) error {
	user, err := s.repo.AdminUser(ctx, userID)
	if err != nil {
		return err
	}
	if err := checkPasswordPolicy(password); err != nil {
		return err
	}
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return err
	}
	if err := s.repo.SetPassword(ctx, userID, hash, true, s.clock.Now()); err != nil {
		return err
	}
	return s.auditor.Record(ctx, audit.Event{
		EntityType: "admin_user", EntityID: userID, Action: "password_reset",
		NewValue: audit.Str(user.Email), ActorID: actor.ID, ActorName: actor.Name,
	})
}

// ChangeOwnPassword is a signed-in user replacing their own.
//
// The current password is required even though the session already proves who
// they are: it is what stops a walked-away laptop from becoming a permanent
// takeover. An account whose password was handed to it is the one exception —
// there is no current password worth proving.
func (s *Service) ChangeOwnPassword(ctx context.Context, userID, current, next string) error {
	creds, err := s.repo.Credentials(ctx, userID)
	if err != nil {
		return err
	}
	if creds.PasswordHash != "" && !s.passwords.Matches(current, creds.PasswordHash) {
		return httpx.ErrUnauthorized.WithMessage("That is not your current password.")
	}
	if err := checkPasswordPolicy(next); err != nil {
		return err
	}
	if creds.PasswordHash != "" && s.passwords.Matches(next, creds.PasswordHash) {
		return httpx.Invalid("Choose a password you have not used here before.")
	}
	hash, err := s.passwords.Hash(next)
	if err != nil {
		return err
	}
	if err := s.repo.SetPassword(ctx, userID, hash, false, s.clock.Now()); err != nil {
		return err
	}
	return s.auditor.Record(ctx, audit.Event{
		EntityType: "admin_user", EntityID: userID, Action: "password_change",
		ActorID: userID, ActorName: creds.Name,
	})
}

// AdminUserInput creates or edits a staff account.
type AdminUserInput struct {
	Name     string
	Email    string
	Role     domain.AdminRole
	BranchID *string
}

func (s *Service) CreateAdminUser(ctx context.Context, in AdminUserInput, actor Actor) (domain.AdminUser, error) {
	user := domain.AdminUser{
		ID:       s.ids.New(id.AdminUser),
		Name:     strings.TrimSpace(in.Name),
		Email:    strings.ToLower(strings.TrimSpace(in.Email)),
		Role:     in.Role,
		BranchID: in.BranchID,
	}
	created, err := s.repo.InsertAdminUser(ctx, user)
	if err != nil {
		return domain.AdminUser{}, err
	}
	return created, s.auditor.Record(ctx, audit.Event{
		EntityType: "admin_user", EntityID: created.ID, Action: "create",
		NewValue: audit.Str(string(created.Role)), ActorID: actor.ID, ActorName: actor.Name,
	})
}

func (s *Service) UpdateAdminUser(ctx context.Context, userID string, in AdminUserInput, setBranch bool, actor Actor) (domain.AdminUser, error) {
	user, err := s.repo.AdminUser(ctx, userID)
	if err != nil {
		return domain.AdminUser{}, err
	}
	previousRole := user.Role

	if in.Name != "" {
		user.Name = strings.TrimSpace(in.Name)
	}
	if in.Email != "" {
		user.Email = strings.ToLower(strings.TrimSpace(in.Email))
	}
	if in.Role != "" {
		user.Role = in.Role
	}
	if setBranch {
		user.BranchID = in.BranchID
	}

	// Demoting the last Super Admin would lock everyone out of user and rules
	// management, so it is refused here as well as on delete.
	if previousRole == domain.RoleSuperAdmin && user.Role != domain.RoleSuperAdmin {
		count, err := s.repo.CountRole(ctx, domain.RoleSuperAdmin)
		if err != nil {
			return domain.AdminUser{}, err
		}
		if count <= 1 {
			return domain.AdminUser{}, httpx.ErrInUse.WithMessage("At least one Super Admin must remain.")
		}
	}

	updated, err := s.repo.UpdateAdminUser(ctx, user)
	if err != nil {
		return domain.AdminUser{}, err
	}
	return updated, s.auditor.Record(ctx, audit.Event{
		EntityType: "admin_user", EntityID: userID, Action: "update",
		PreviousValue: audit.Str(string(previousRole)), NewValue: audit.Str(string(updated.Role)),
		ActorID: actor.ID, ActorName: actor.Name,
	})
}

func (s *Service) DeleteAdminUser(ctx context.Context, userID string, actor Actor) error {
	if userID == actor.ID {
		return httpx.ErrInUse.WithMessage("You cannot delete the account you are signed in with.")
	}
	user, err := s.repo.AdminUser(ctx, userID)
	if err != nil {
		return err
	}
	if user.Role == domain.RoleSuperAdmin {
		count, err := s.repo.CountRole(ctx, domain.RoleSuperAdmin)
		if err != nil {
			return err
		}
		if count <= 1 {
			return httpx.ErrInUse.WithMessage("At least one Super Admin must remain.")
		}
	}
	if err := s.repo.DeleteAdminUser(ctx, userID); err != nil {
		return err
	}
	return s.auditor.Record(ctx, audit.Event{
		EntityType: "admin_user", EntityID: userID, Action: "delete",
		PreviousValue: audit.Str(user.Email), ActorID: actor.ID, ActorName: actor.Name,
	})
}

// PurgeExpiredChallenges is called by the background maintenance loop.
func (s *Service) PurgeExpiredChallenges(ctx context.Context) (int64, error) {
	return s.repo.PurgeExpiredChallenges(ctx, s.clock.Now().Add(-24*time.Hour))
}

func strPtr(value string) *string { return &value }

// ── Supervisor PINs ──────────────────────────────────────────────────────────

// SetSupervisorPIN gives somebody a PIN for authorising at a till, or takes it
// away when the PIN is empty.
//
// Four digits minimum and six maximum: shorter is guessable over a cashier's
// shoulder in one glance, and longer is a password somebody will write down.
func (s *Service) SetSupervisorPIN(ctx context.Context, userID, pin string) error {
	user, err := s.repo.AdminUser(ctx, userID)
	if err != nil {
		return err
	}

	if strings.TrimSpace(pin) == "" {
		return s.repo.SetSupervisorPIN(ctx, userID, nil)
	}
	if len(pin) < 4 || len(pin) > 6 {
		return httpx.Invalid("A supervisor PIN is four to six digits.")
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return httpx.Invalid("A supervisor PIN is digits only.")
		}
	}
	// A PIN on an account that cannot authorise anything is a PIN that will be
	// typed at a till and silently refused.
	if !domain.HasPermission(user.Role, domain.PermPOSVoid) {
		return httpx.Conflict("CANNOT_AUTHORISE",
			"%s cannot authorise at a till, so a PIN would do nothing.",
			strings.ToLower(strings.ReplaceAll(string(user.Role), "_", " ")))
	}

	hash := s.issuer.PINHash(pin)
	return s.repo.SetSupervisorPIN(ctx, userID, &hash)
}

// Supervisor is who a PIN turned out to belong to.
type Supervisor struct {
	ID   string
	Name string
	Role domain.AdminRole
}

// VerifyPIN works out which manager typed a PIN, if any of them did.
//
// It checks every candidate rather than stopping at the first match, so the
// time taken does not depend on where in the list the right one is. The
// permission is checked here too: a PIN is only an answer to "is somebody who
// may do this standing here".
func (s *Service) VerifyPIN(ctx context.Context, pin string, permission domain.Permission) (Supervisor, bool, error) {
	if strings.TrimSpace(pin) == "" {
		return Supervisor{}, false, nil
	}

	candidates, err := s.repo.SupervisorsWithPIN(ctx)
	if err != nil {
		return Supervisor{}, false, err
	}

	var found Supervisor
	matched := false
	for _, candidate := range candidates {
		if !s.issuer.PINMatches(pin, candidate.PINHash) {
			continue
		}
		if !domain.HasPermission(candidate.Role, permission) {
			continue
		}
		if !matched {
			found = Supervisor{ID: candidate.ID, Name: candidate.Name, Role: candidate.Role}
			matched = true
		}
	}
	return found, matched, nil
}
