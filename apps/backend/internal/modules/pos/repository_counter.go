package pos

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/database"
	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// Offers, gift cards, tenders, receipts.

func dateValue(d *domain.Date) any {
	if d == nil || *d == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", string(*d))
	if err != nil {
		return nil
	}
	return parsed
}

func requiredDate(d domain.Date) any { return dateValue(&d) }

func optionalDate(t *time.Time) *domain.Date {
	if t == nil {
		return nil
	}
	d := domain.Date(t.Format("2006-01-02"))
	return &d
}

// ── Promotions ───────────────────────────────────────────────────────────────

const promotionColumns = `id, code, name, description, kind, percent, amount_idr, buy_qty,
	free_qty, bundle_price_idr, min_spend_idr, min_qty, requires_code, channels, exclusive,
	priority, starts_on, ends_on, max_uses, max_uses_per_member, used_count, active`

func scanPromotion(row pgx.Row) (domain.Promotion, error) {
	var p domain.Promotion
	var startsOn, endsOn *time.Time
	err := row.Scan(&p.ID, &p.Code, &p.Name, &p.Description, &p.Kind, &p.Percent, &p.AmountIDR,
		&p.BuyQty, &p.FreeQty, &p.BundlePriceIDR, &p.MinSpendIDR, &p.MinQty, &p.RequiresCode,
		&p.Channels, &p.Exclusive, &p.Priority, &startsOn, &endsOn, &p.MaxUses,
		&p.MaxUsesPerMember, &p.UsedCount, &p.Active)
	if err != nil {
		return domain.Promotion{}, err
	}
	p.StartsOn, p.EndsOn = optionalDate(startsOn), optionalDate(endsOn)
	return p, nil
}

func (r *Repository) Promotions(ctx context.Context, activeOnly bool) ([]domain.Promotion, error) {
	query := `SELECT ` + promotionColumns + ` FROM pos.promotions`
	if activeOnly {
		query += ` WHERE active`
	}
	query += ` ORDER BY priority DESC, name`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("pos: listing promotions: %w", err)
	}
	defer rows.Close()

	out := []domain.Promotion{}
	for rows.Next() {
		p, err := scanPromotion(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning promotion: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) UpsertPromotion(ctx context.Context, p domain.Promotion) (domain.Promotion, error) {
	saved, err := scanPromotion(r.db.QueryRow(ctx, `
		INSERT INTO pos.promotions (id, code, name, description, kind, percent, amount_idr,
			buy_qty, free_qty, bundle_price_idr, min_spend_idr, min_qty, requires_code,
			channels, exclusive, priority, starts_on, ends_on, max_uses, max_uses_per_member,
			active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
			$18, $19, $20, $21)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name,
			description = EXCLUDED.description, kind = EXCLUDED.kind,
			percent = EXCLUDED.percent, amount_idr = EXCLUDED.amount_idr,
			buy_qty = EXCLUDED.buy_qty, free_qty = EXCLUDED.free_qty,
			bundle_price_idr = EXCLUDED.bundle_price_idr,
			min_spend_idr = EXCLUDED.min_spend_idr, min_qty = EXCLUDED.min_qty,
			requires_code = EXCLUDED.requires_code, channels = EXCLUDED.channels,
			exclusive = EXCLUDED.exclusive, priority = EXCLUDED.priority,
			starts_on = EXCLUDED.starts_on, ends_on = EXCLUDED.ends_on,
			max_uses = EXCLUDED.max_uses, max_uses_per_member = EXCLUDED.max_uses_per_member,
			active = EXCLUDED.active, updated_at = now()
		RETURNING `+promotionColumns,
		p.ID, p.Code, p.Name, p.Description, p.Kind, p.Percent, p.AmountIDR, p.BuyQty,
		p.FreeQty, p.BundlePriceIDR, p.MinSpendIDR, p.MinQty, p.RequiresCode, p.Channels,
		p.Exclusive, p.Priority, dateValue(p.StartsOn), dateValue(p.EndsOn),
		p.MaxUses, p.MaxUsesPerMember, p.Active))
	if database.IsCheckViolation(err) {
		return domain.Promotion{}, httpx.Invalid("That offer's window ends before it starts.")
	}
	if err != nil {
		return domain.Promotion{}, fmt.Errorf("pos: saving promotion: %w", err)
	}
	return saved, nil
}

const targetColumns = `id, promotion_id, product_id, category_id, qty`

// PromotionTargets is what every offer applies to, in one round trip: the till
// evaluates all of them on every line, and one query per offer would make
// scanning an item slower with every promotion the shop runs.
func (r *Repository) PromotionTargets(ctx context.Context) (map[string][]domain.PromotionTarget, error) {
	rows, err := r.db.Query(ctx, `SELECT `+targetColumns+` FROM pos.promotion_targets`)
	if err != nil {
		return nil, fmt.Errorf("pos: listing promotion targets: %w", err)
	}
	defer rows.Close()

	out := map[string][]domain.PromotionTarget{}
	for rows.Next() {
		var t domain.PromotionTarget
		if err := rows.Scan(&t.ID, &t.PromotionID, &t.ProductID, &t.CategoryID, &t.Qty); err != nil {
			return nil, fmt.Errorf("pos: scanning promotion target: %w", err)
		}
		out[t.PromotionID] = append(out[t.PromotionID], t)
	}
	return out, rows.Err()
}

func (r *Repository) ReplaceTargets(ctx context.Context, promotionID string, targets []domain.PromotionTarget) error {
	if _, err := r.db.Exec(ctx,
		`DELETE FROM pos.promotion_targets WHERE promotion_id = $1`, promotionID); err != nil {
		return fmt.Errorf("pos: clearing promotion targets: %w", err)
	}
	for _, target := range targets {
		if _, err := r.db.Exec(ctx, `
			INSERT INTO pos.promotion_targets (id, promotion_id, product_id, category_id, qty)
			VALUES ($1, $2, $3, $4, $5)`,
			target.ID, promotionID, target.ProductID, target.CategoryID, target.Qty); err != nil {
			if database.IsCheckViolation(err) {
				return httpx.Invalid("A target is either a product or a category, not both.")
			}
			return fmt.Errorf("pos: inserting promotion target: %w", err)
		}
	}
	return nil
}

// ReplaceOrderPromotions records what an order actually got.
func (r *Repository) ReplaceOrderPromotions(ctx context.Context, orderID string,
	applied []domain.AppliedPromotion, ids []string) error {

	if _, err := r.db.Exec(ctx,
		`DELETE FROM pos.order_promotions WHERE order_id = $1`, orderID); err != nil {
		return fmt.Errorf("pos: clearing order promotions: %w", err)
	}
	for i, promotion := range applied {
		if _, err := r.db.Exec(ctx, `
			INSERT INTO pos.order_promotions (id, order_id, promotion_id, code, name, discount_idr)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			ids[i], orderID, promotion.PromotionID, promotion.Code, promotion.Name,
			promotion.DiscountIDR); err != nil {
			return fmt.Errorf("pos: recording order promotion: %w", err)
		}
	}
	return nil
}

func (r *Repository) OrderPromotions(ctx context.Context, orderID string) ([]domain.AppliedPromotion, error) {
	rows, err := r.db.Query(ctx,
		`SELECT promotion_id, code, name, discount_idr FROM pos.order_promotions
		 WHERE order_id = $1 ORDER BY discount_idr DESC`, orderID)
	if err != nil {
		return nil, fmt.Errorf("pos: listing order promotions: %w", err)
	}
	defer rows.Close()

	out := []domain.AppliedPromotion{}
	for rows.Next() {
		var p domain.AppliedPromotion
		if err := rows.Scan(&p.PromotionID, &p.Code, &p.Name, &p.DiscountIDR); err != nil {
			return nil, fmt.Errorf("pos: scanning order promotion: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountPromotionUse bumps an offer's usage when a sale completes, not when it
// is applied: an abandoned basket should not burn a limited offer.
func (r *Repository) CountPromotionUse(ctx context.Context, promotionID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE pos.promotions SET used_count = used_count + 1, updated_at = now() WHERE id = $1`,
		promotionID)
	if err != nil {
		return fmt.Errorf("pos: counting promotion use: %w", err)
	}
	return nil
}

// MemberPromotionUses is how many times one member has had each offer.
func (r *Repository) MemberPromotionUses(ctx context.Context, memberID string) (map[string]int, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.promotion_id, count(*)
		FROM pos.order_promotions p
		JOIN pos.orders o ON o.id = p.order_id
		WHERE o.member_id = $1 AND o.status = 'COMPLETED'
		GROUP BY p.promotion_id`, memberID)
	if err != nil {
		return nil, fmt.Errorf("pos: counting member promotion uses: %w", err)
	}
	defer rows.Close()

	out := map[string]int{}
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("pos: scanning member promotion use: %w", err)
		}
		out[id] = count
	}
	return out, rows.Err()
}

// ── Gift cards ───────────────────────────────────────────────────────────────

const giftCardColumns = `id, code, barcode, member_id, issued_on, expires_on, initial_idr,
	balance_idr, status, issued_by, note, created_at, updated_at`

func scanGiftCard(row pgx.Row) (domain.GiftCard, error) {
	var c domain.GiftCard
	var issuedOn time.Time
	var expiresOn *time.Time
	err := row.Scan(&c.ID, &c.Code, &c.Barcode, &c.MemberID, &issuedOn, &expiresOn,
		&c.InitialIDR, &c.BalanceIDR, &c.Status, &c.IssuedBy, &c.Note,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return domain.GiftCard{}, err
	}
	c.IssuedOn = domain.Date(issuedOn.Format("2006-01-02"))
	c.ExpiresOn = optionalDate(expiresOn)
	return c, nil
}

// GiftCardFilter narrows the card list.
type GiftCardFilter struct {
	MemberID string
	Status   string
	Query    string
	Limit    int
}

func (r *Repository) GiftCards(ctx context.Context, filter GiftCardFilter) ([]domain.GiftCard, error) {
	query := `SELECT ` + giftCardColumns + ` FROM pos.gift_cards WHERE 1 = 1`
	args := []any{}
	if filter.MemberID != "" {
		args = append(args, filter.MemberID)
		query += fmt.Sprintf(" AND member_id = $%d", len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if filter.Query != "" {
		args = append(args, "%"+filter.Query+"%")
		query += fmt.Sprintf(" AND (code ILIKE $%d OR barcode ILIKE $%d)", len(args), len(args))
	}
	query += ` ORDER BY created_at DESC`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("pos: listing gift cards: %w", err)
	}
	defer rows.Close()

	out := []domain.GiftCard{}
	for rows.Next() {
		c, err := scanGiftCard(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning gift card: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GiftCardByCode resolves what somebody typed or scanned. Either identifier
// finds the card, because a cashier holding a card should not have to know
// which of the two numbers on it the system wants.
func (r *Repository) GiftCardByCode(ctx context.Context, code string, forUpdate bool) (domain.GiftCard, error) {
	query := `SELECT ` + giftCardColumns + ` FROM pos.gift_cards WHERE code = $1 OR barcode = $1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	c, err := scanGiftCard(r.db.QueryRow(ctx, query, code))
	if database.IsNoRows(err) {
		return domain.GiftCard{}, httpx.NotFound("gift card")
	}
	if err != nil {
		return domain.GiftCard{}, fmt.Errorf("pos: reading gift card: %w", err)
	}
	return c, nil
}

func (r *Repository) InsertGiftCard(ctx context.Context, c domain.GiftCard) (domain.GiftCard, error) {
	created, err := scanGiftCard(r.db.QueryRow(ctx, `
		INSERT INTO pos.gift_cards (id, code, barcode, member_id, issued_on, expires_on,
			initial_idr, balance_idr, status, issued_by, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING `+giftCardColumns,
		c.ID, c.Code, c.Barcode, c.MemberID, requiredDate(c.IssuedOn), dateValue(c.ExpiresOn),
		c.InitialIDR, c.BalanceIDR, c.Status, c.IssuedBy, c.Note))
	if database.IsUniqueViolation(err) {
		return domain.GiftCard{}, httpx.Conflict("DUPLICATE", "That card number is already issued.")
	}
	if err != nil {
		return domain.GiftCard{}, fmt.Errorf("pos: issuing gift card: %w", err)
	}
	return created, nil
}

// PostGiftCard writes one movement and the balance it produced, together.
func (r *Repository) PostGiftCard(ctx context.Context, cardID string, posted domain.PostedGiftCard,
	kind string, orderID *string, reason *string, actorID, entryID string) error {

	if _, err := r.db.Exec(ctx, `
		INSERT INTO pos.gift_card_entries (id, card_id, kind, amount_idr, balance_before,
			balance_after, order_id, reason, actor_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		entryID, cardID, kind, posted.AmountIDR, posted.BalanceBefore, posted.BalanceAfter,
		orderID, reason, actorID); err != nil {
		if database.IsCheckViolation(err) {
			return httpx.Invalid("That card movement does not add up.")
		}
		return fmt.Errorf("pos: writing gift card entry: %w", err)
	}
	if _, err := r.db.Exec(ctx,
		`UPDATE pos.gift_cards SET balance_idr = $2, updated_at = now() WHERE id = $1`,
		cardID, posted.BalanceAfter); err != nil {
		return fmt.Errorf("pos: updating gift card balance: %w", err)
	}
	return nil
}

func (r *Repository) SetGiftCardStatus(ctx context.Context, cardID, status string) (domain.GiftCard, error) {
	saved, err := scanGiftCard(r.db.QueryRow(ctx,
		`UPDATE pos.gift_cards SET status = $2, updated_at = now()
		 WHERE id = $1 RETURNING `+giftCardColumns, cardID, status))
	if database.IsNoRows(err) {
		return domain.GiftCard{}, httpx.NotFound("gift card")
	}
	if err != nil {
		return domain.GiftCard{}, fmt.Errorf("pos: setting gift card status: %w", err)
	}
	return saved, nil
}

// GiftCardEntry is one movement on a card.
type GiftCardEntry struct {
	ID            string    `json:"id"`
	CardID        string    `json:"cardId"`
	Kind          string    `json:"kind"`
	AmountIDR     float64   `json:"amountIdr"`
	BalanceBefore float64   `json:"balanceBefore"`
	BalanceAfter  float64   `json:"balanceAfter"`
	OrderID       *string   `json:"orderId"`
	Reason        *string   `json:"reason"`
	ActorID       *string   `json:"actorId"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (r *Repository) GiftCardEntries(ctx context.Context, cardID string) ([]GiftCardEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, card_id, kind, amount_idr, balance_before, balance_after, order_id,
		       reason, actor_id, created_at
		FROM pos.gift_card_entries WHERE card_id = $1 ORDER BY created_at DESC`, cardID)
	if err != nil {
		return nil, fmt.Errorf("pos: listing gift card entries: %w", err)
	}
	defer rows.Close()

	out := []GiftCardEntry{}
	for rows.Next() {
		var e GiftCardEntry
		if err := rows.Scan(&e.ID, &e.CardID, &e.Kind, &e.AmountIDR, &e.BalanceBefore,
			&e.BalanceAfter, &e.OrderID, &e.Reason, &e.ActorID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("pos: scanning gift card entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── Payment methods ──────────────────────────────────────────────────────────

const methodColumns = `id, code, name, kind, gives_change, needs_reference, counts_in_drawer,
	sort_order, active`

func scanMethod(row pgx.Row) (domain.PaymentMethod, error) {
	var m domain.PaymentMethod
	err := row.Scan(&m.ID, &m.Code, &m.Name, &m.Kind, &m.GivesChange, &m.NeedsReference,
		&m.CountsInDrawer, &m.SortOrder, &m.Active)
	return m, err
}

func (r *Repository) PaymentMethods(ctx context.Context, activeOnly bool) ([]domain.PaymentMethod, error) {
	query := `SELECT ` + methodColumns + ` FROM pos.payment_methods`
	if activeOnly {
		query += ` WHERE active`
	}
	query += ` ORDER BY sort_order, name`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("pos: listing payment methods: %w", err)
	}
	defer rows.Close()

	out := []domain.PaymentMethod{}
	for rows.Next() {
		m, err := scanMethod(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning payment method: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MethodsByCode is the lookup the settlement rules take.
func (r *Repository) MethodsByCode(ctx context.Context) (map[string]domain.PaymentMethod, error) {
	methods, err := r.PaymentMethods(ctx, false)
	if err != nil {
		return nil, err
	}
	out := map[string]domain.PaymentMethod{}
	for _, method := range methods {
		out[method.Code] = method
	}
	return out, nil
}

func (r *Repository) UpsertMethod(ctx context.Context, m domain.PaymentMethod) (domain.PaymentMethod, error) {
	saved, err := scanMethod(r.db.QueryRow(ctx, `
		INSERT INTO pos.payment_methods (id, code, name, kind, gives_change, needs_reference,
			counts_in_drawer, sort_order, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (code) DO UPDATE SET name = EXCLUDED.name, kind = EXCLUDED.kind,
			gives_change = EXCLUDED.gives_change, needs_reference = EXCLUDED.needs_reference,
			counts_in_drawer = EXCLUDED.counts_in_drawer, sort_order = EXCLUDED.sort_order,
			active = EXCLUDED.active, updated_at = now()
		RETURNING `+methodColumns,
		m.ID, m.Code, m.Name, m.Kind, m.GivesChange, m.NeedsReference, m.CountsInDrawer,
		m.SortOrder, m.Active))
	if database.IsCheckViolation(err) {
		return domain.PaymentMethod{}, httpx.Invalid(
			"Change comes out of the drawer, so a method that gives it has to be counted in it.")
	}
	if err != nil {
		return domain.PaymentMethod{}, fmt.Errorf("pos: saving payment method: %w", err)
	}
	return saved, nil
}

// ── Receipts ─────────────────────────────────────────────────────────────────

// ReceiptSettings is what a printed receipt says, per branch.
type ReceiptSettings struct {
	BranchID     string  `json:"branchId"`
	Header       string  `json:"header"`
	Footer       string  `json:"footer"`
	BusinessName string  `json:"businessName"`
	Address      string  `json:"address"`
	Phone        *string `json:"phone"`
	TaxNumber    *string `json:"taxNumber"`
	PaperWidth   int     `json:"paperWidth"`
	ShowLogo     bool    `json:"showLogo"`
	ShowCashier  bool    `json:"showCashier"`
	AutoPrint    bool    `json:"autoPrint"`
}

const settingsColumns = `branch_id, header, footer, business_name, address, phone, tax_number,
	paper_width, show_logo, show_cashier, auto_print`

func scanSettings(row pgx.Row) (ReceiptSettings, error) {
	var s ReceiptSettings
	err := row.Scan(&s.BranchID, &s.Header, &s.Footer, &s.BusinessName, &s.Address,
		&s.Phone, &s.TaxNumber, &s.PaperWidth, &s.ShowLogo, &s.ShowCashier, &s.AutoPrint)
	return s, err
}

// Settings returns a branch's receipt setup, or sensible defaults when nobody
// has configured one — a shop should be able to print before it has been to
// the settings screen.
func (r *Repository) Settings(ctx context.Context, branchID string) (ReceiptSettings, error) {
	s, err := scanSettings(r.db.QueryRow(ctx,
		`SELECT `+settingsColumns+` FROM pos.receipt_settings WHERE branch_id = $1`, branchID))
	if database.IsNoRows(err) {
		return ReceiptSettings{
			BranchID: branchID, PaperWidth: 58, ShowLogo: true,
			ShowCashier: true, AutoPrint: true,
		}, nil
	}
	if err != nil {
		return ReceiptSettings{}, fmt.Errorf("pos: reading receipt settings: %w", err)
	}
	return s, nil
}

func (r *Repository) SaveSettings(ctx context.Context, s ReceiptSettings) (ReceiptSettings, error) {
	saved, err := scanSettings(r.db.QueryRow(ctx, `
		INSERT INTO pos.receipt_settings (branch_id, header, footer, business_name, address,
			phone, tax_number, paper_width, show_logo, show_cashier, auto_print)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (branch_id) DO UPDATE SET header = EXCLUDED.header,
			footer = EXCLUDED.footer, business_name = EXCLUDED.business_name,
			address = EXCLUDED.address, phone = EXCLUDED.phone,
			tax_number = EXCLUDED.tax_number, paper_width = EXCLUDED.paper_width,
			show_logo = EXCLUDED.show_logo, show_cashier = EXCLUDED.show_cashier,
			auto_print = EXCLUDED.auto_print, updated_at = now()
		RETURNING `+settingsColumns,
		s.BranchID, s.Header, s.Footer, s.BusinessName, s.Address, s.Phone, s.TaxNumber,
		s.PaperWidth, s.ShowLogo, s.ShowCashier, s.AutoPrint))
	if database.IsCheckViolation(err) {
		return ReceiptSettings{}, httpx.Invalid("Thermal paper is 58mm or 80mm.")
	}
	if err != nil {
		return ReceiptSettings{}, fmt.Errorf("pos: saving receipt settings: %w", err)
	}
	return saved, nil
}

// PrintJob is one document waiting for a printer.
type PrintJob struct {
	ID        string     `json:"id"`
	BranchID  string     `json:"branchId"`
	Kind      string     `json:"kind"`
	OrderID   *string    `json:"orderId"`
	ShiftID   *string    `json:"shiftId"`
	Payload   string     `json:"payload"`
	Status    string     `json:"status"`
	Attempts  int        `json:"attempts"`
	Error     *string    `json:"error"`
	ClaimedAt *time.Time `json:"claimedAt"`
	PrintedAt *time.Time `json:"printedAt"`
	CreatedAt time.Time  `json:"createdAt"`
}

const printJobColumns = `id, branch_id, kind, order_id, shift_id, payload, status, attempts,
	error, claimed_at, printed_at, created_at`

func scanPrintJob(row pgx.Row) (PrintJob, error) {
	var j PrintJob
	err := row.Scan(&j.ID, &j.BranchID, &j.Kind, &j.OrderID, &j.ShiftID, &j.Payload,
		&j.Status, &j.Attempts, &j.Error, &j.ClaimedAt, &j.PrintedAt, &j.CreatedAt)
	return j, err
}

func (r *Repository) InsertPrintJob(ctx context.Context, j PrintJob) (PrintJob, error) {
	created, err := scanPrintJob(r.db.QueryRow(ctx, `
		INSERT INTO pos.print_jobs (id, branch_id, kind, order_id, shift_id, payload, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+printJobColumns,
		j.ID, j.BranchID, j.Kind, j.OrderID, j.ShiftID, j.Payload, j.Status))
	if err != nil {
		return PrintJob{}, fmt.Errorf("pos: queueing print job: %w", err)
	}
	return created, nil
}

// ClaimPrintJobs hands the next few jobs to a printer agent and marks them as
// taken, so two agents on one counter do not print the same receipt twice.
func (r *Repository) ClaimPrintJobs(ctx context.Context, branchID string, limit int) ([]PrintJob, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE pos.print_jobs SET status = 'PRINTING', claimed_at = now(),
			attempts = attempts + 1, updated_at = now()
		WHERE id IN (
			SELECT id FROM pos.print_jobs
			WHERE branch_id = $1 AND status = 'QUEUED'
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		RETURNING `+printJobColumns, branchID, limit)
	if err != nil {
		return nil, fmt.Errorf("pos: claiming print jobs: %w", err)
	}
	defer rows.Close()

	out := []PrintJob{}
	for rows.Next() {
		j, err := scanPrintJob(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning print job: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (r *Repository) FinishPrintJob(ctx context.Context, jobID, status string, failure *string) (PrintJob, error) {
	saved, err := scanPrintJob(r.db.QueryRow(ctx, `
		UPDATE pos.print_jobs SET status = $2, error = $3,
			printed_at = CASE WHEN $2 = 'PRINTED' THEN now() ELSE printed_at END,
			updated_at = now()
		WHERE id = $1 RETURNING `+printJobColumns, jobID, status, failure))
	if database.IsNoRows(err) {
		return PrintJob{}, httpx.NotFound("print job")
	}
	if err != nil {
		return PrintJob{}, fmt.Errorf("pos: finishing print job: %w", err)
	}
	return saved, nil
}

func (r *Repository) PrintJobs(ctx context.Context, branchID, status string, limit int) ([]PrintJob, error) {
	query := `SELECT ` + printJobColumns + ` FROM pos.print_jobs WHERE 1 = 1`
	args := []any{}
	if branchID != "" {
		args = append(args, branchID)
		query += fmt.Sprintf(" AND branch_id = $%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("pos: listing print jobs: %w", err)
	}
	defer rows.Close()

	out := []PrintJob{}
	for rows.Next() {
		j, err := scanPrintJob(rows)
		if err != nil {
			return nil, fmt.Errorf("pos: scanning print job: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ReceiptSend is a receipt on its way to a phone.
type ReceiptSend struct {
	ID          string     `json:"id"`
	OrderID     string     `json:"orderId"`
	Channel     string     `json:"channel"`
	Destination string     `json:"destination"`
	Body        string     `json:"body"`
	Status      string     `json:"status"`
	Error       *string    `json:"error"`
	SentAt      *time.Time `json:"sentAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

func (r *Repository) InsertReceiptSend(ctx context.Context, s ReceiptSend) (ReceiptSend, error) {
	var created ReceiptSend
	err := r.db.QueryRow(ctx, `
		INSERT INTO pos.receipt_sends (id, order_id, channel, destination, body, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, order_id, channel, destination, body, status, error, sent_at, created_at`,
		s.ID, s.OrderID, s.Channel, s.Destination, s.Body, s.Status).
		Scan(&created.ID, &created.OrderID, &created.Channel, &created.Destination,
			&created.Body, &created.Status, &created.Error, &created.SentAt, &created.CreatedAt)
	if database.IsForeignKeyViolation(err) {
		return ReceiptSend{}, httpx.NotFound("order")
	}
	if err != nil {
		return ReceiptSend{}, fmt.Errorf("pos: queueing receipt send: %w", err)
	}
	return created, nil
}
