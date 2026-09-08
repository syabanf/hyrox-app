package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/syabanf/nuhabit-backend/internal/domain"
	"github.com/syabanf/nuhabit-backend/internal/platform/database"
	"github.com/syabanf/nuhabit-backend/internal/platform/httpx"
)

// Repository persists the credit ledger, expiry lots, payments and vouchers.
type Repository struct {
	db *database.DB
}

func NewRepository(db *database.DB) *Repository { return &Repository{db: db} }

// ── Ledger ───────────────────────────────────────────────────────────────────

const entryColumns = `id, member_id, type, amount, description, source_type, source_id,
	reverses_entry_id, actor_id, reason, created_at`

func scanEntry(row pgx.Row) (domain.CreditLedgerEntry, error) {
	var e domain.CreditLedgerEntry
	err := row.Scan(&e.ID, &e.MemberID, &e.Type, &e.Amount, &e.Description, &e.SourceType,
		&e.SourceID, &e.ReversesEntryID, &e.ActorID, &e.Reason, &e.CreatedAt)
	return e, err
}

func (r *Repository) Entries(ctx context.Context, memberID string) ([]domain.CreditLedgerEntry, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+entryColumns+` FROM wallet.credit_ledger_entries WHERE member_id = $1 ORDER BY created_at DESC, id DESC`,
		memberID)
	if err != nil {
		return nil, fmt.Errorf("wallet: listing ledger entries: %w", err)
	}
	defer rows.Close()

	entries := []domain.CreditLedgerEntry{}
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("wallet: scanning ledger entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (r *Repository) Entry(ctx context.Context, id string) (domain.CreditLedgerEntry, error) {
	e, err := scanEntry(r.db.QueryRow(ctx, `SELECT `+entryColumns+` FROM wallet.credit_ledger_entries WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.CreditLedgerEntry{}, httpx.NotFound("ledger entry")
	}
	if err != nil {
		return domain.CreditLedgerEntry{}, fmt.Errorf("wallet: reading ledger entry: %w", err)
	}
	return e, nil
}

// InsertEntry appends to the ledger. The table refuses updates and deletes, so
// this is the only way credits ever move.
func (r *Repository) InsertEntry(ctx context.Context, e domain.CreditLedgerEntry) (domain.CreditLedgerEntry, error) {
	created, err := scanEntry(r.db.QueryRow(ctx, `
		INSERT INTO wallet.credit_ledger_entries
			(id, member_id, type, amount, description, source_type, source_id, reverses_entry_id, actor_id, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+entryColumns,
		e.ID, e.MemberID, e.Type, e.Amount, e.Description, e.SourceType, e.SourceID,
		e.ReversesEntryID, e.ActorID, e.Reason, e.CreatedAt))
	if database.IsUniqueViolation(err, "ledger_reversal_idx") {
		return domain.CreditLedgerEntry{}, httpx.Conflict("ALREADY_REVERSED", "That entry has already been reversed.")
	}
	if err != nil {
		return domain.CreditLedgerEntry{}, fmt.Errorf("wallet: inserting ledger entry: %w", err)
	}
	return created, nil
}

// Balance sums the ledger in the database rather than loading every row: this
// runs on every booking and every gate scan.
func (r *Repository) Balance(ctx context.Context, memberID string) (int, error) {
	var balance int
	err := r.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM wallet.credit_ledger_entries WHERE member_id = $1`, memberID).
		Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("wallet: computing balance: %w", err)
	}
	return balance, nil
}

// Balances loads several balances at once, for list views.
func (r *Repository) Balances(ctx context.Context, memberIDs []string) (map[string]int, error) {
	if len(memberIDs) == 0 {
		return map[string]int{}, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT member_id, COALESCE(SUM(amount), 0) FROM wallet.credit_ledger_entries
		WHERE member_id = ANY($1) GROUP BY member_id`, memberIDs)
	if err != nil {
		return nil, fmt.Errorf("wallet: computing balances: %w", err)
	}
	defer rows.Close()

	balances := make(map[string]int, len(memberIDs))
	for rows.Next() {
		var memberID string
		var balance int
		if err := rows.Scan(&memberID, &balance); err != nil {
			return nil, fmt.Errorf("wallet: scanning balance: %w", err)
		}
		balances[memberID] = balance
	}
	return balances, rows.Err()
}

// OutstandingCredits is the studio's total liability: credits sold and not yet
// consumed, across every member who still holds any.
func (r *Repository) OutstandingCredits(ctx context.Context) (int, error) {
	var total int
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(balance), 0) FROM (
			SELECT SUM(amount) AS balance FROM wallet.credit_ledger_entries GROUP BY member_id
		) balances WHERE balance > 0`).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("wallet: computing outstanding credits: %w", err)
	}
	return total, nil
}

// HasReversal reports whether an entry was already compensated.
func (r *Repository) HasReversal(ctx context.Context, entryID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM wallet.credit_ledger_entries WHERE reverses_entry_id = $1)`, entryID).
		Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("wallet: checking reversal: %w", err)
	}
	return exists, nil
}

// EntryBySource finds the entry a payment produced, so a refund knows what to
// reverse.
func (r *Repository) EntryBySource(ctx context.Context, sourceType domain.LedgerSourceType, sourceID string, entryType domain.LedgerEntryType) (domain.CreditLedgerEntry, error) {
	e, err := scanEntry(r.db.QueryRow(ctx, `
		SELECT `+entryColumns+` FROM wallet.credit_ledger_entries
		WHERE source_type = $1 AND source_id = $2 AND type = $3
		ORDER BY created_at LIMIT 1`, sourceType, sourceID, entryType))
	if database.IsNoRows(err) {
		return domain.CreditLedgerEntry{}, httpx.NotFound("ledger entry")
	}
	if err != nil {
		return domain.CreditLedgerEntry{}, fmt.Errorf("wallet: reading entry by source: %w", err)
	}
	return e, nil
}

// ── Lots ─────────────────────────────────────────────────────────────────────

const lotColumns = `id, member_id, ledger_entry_id, package_id, credits, expires_at, created_at`

func scanLot(row pgx.Row) (domain.TopUpLot, error) {
	var l domain.TopUpLot
	err := row.Scan(&l.ID, &l.MemberID, &l.LedgerEntryID, &l.PackageID, &l.Credits, &l.ExpiresAt, &l.CreatedAt)
	return l, err
}

func (r *Repository) Lots(ctx context.Context, memberID string) ([]domain.TopUpLot, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+lotColumns+` FROM wallet.top_up_lots WHERE member_id = $1 ORDER BY expires_at`, memberID)
	if err != nil {
		return nil, fmt.Errorf("wallet: listing lots: %w", err)
	}
	defer rows.Close()

	lots := []domain.TopUpLot{}
	for rows.Next() {
		l, err := scanLot(rows)
		if err != nil {
			return nil, fmt.Errorf("wallet: scanning lot: %w", err)
		}
		lots = append(lots, l)
	}
	return lots, rows.Err()
}

func (r *Repository) InsertLot(ctx context.Context, l domain.TopUpLot) (domain.TopUpLot, error) {
	created, err := scanLot(r.db.QueryRow(ctx, `
		INSERT INTO wallet.top_up_lots (id, member_id, ledger_entry_id, package_id, credits, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+lotColumns,
		l.ID, l.MemberID, l.LedgerEntryID, l.PackageID, l.Credits, l.ExpiresAt, l.CreatedAt))
	if err != nil {
		return domain.TopUpLot{}, fmt.Errorf("wallet: inserting lot: %w", err)
	}
	return created, nil
}

// MembersWithExpiredLots finds who the expiry sweep needs to visit, so the
// sweep never scans the whole membership.
func (r *Repository) MembersWithExpiredLots(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := r.db.Query(ctx,
		`SELECT DISTINCT member_id FROM wallet.top_up_lots WHERE expires_at <= $1`, now)
	if err != nil {
		return nil, fmt.Errorf("wallet: finding expired lots: %w", err)
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var memberID string
		if err := rows.Scan(&memberID); err != nil {
			return nil, fmt.Errorf("wallet: scanning member id: %w", err)
		}
		members = append(members, memberID)
	}
	return members, rows.Err()
}

// ── Payments ─────────────────────────────────────────────────────────────────

const paymentColumns = `id, member_id, package_id, credits, amount_idr, discount_idr, total_idr,
	voucher_code, channel, status, created_at, paid_at, refunded_at`

func scanPayment(row pgx.Row) (domain.Payment, error) {
	var p domain.Payment
	err := row.Scan(&p.ID, &p.MemberID, &p.PackageID, &p.Credits, &p.AmountIDR, &p.DiscountIDR,
		&p.TotalIDR, &p.VoucherCode, &p.Channel, &p.Status, &p.CreatedAt, &p.PaidAt, &p.RefundedAt)
	return p, err
}

// PaymentFilter narrows the admin payment list.
type PaymentFilter struct {
	MemberID string
	Status   string
	Limit    int
}

func (r *Repository) Payments(ctx context.Context, filter PaymentFilter) ([]domain.Payment, error) {
	query := `SELECT ` + paymentColumns + ` FROM wallet.payments WHERE 1 = 1`
	args := []any{}
	if filter.MemberID != "" {
		args = append(args, filter.MemberID)
		query += fmt.Sprintf(` AND member_id = $%d`, len(args))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	query += ` ORDER BY created_at DESC`
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(args))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("wallet: listing payments: %w", err)
	}
	defer rows.Close()

	payments := []domain.Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("wallet: scanning payment: %w", err)
		}
		payments = append(payments, p)
	}
	return payments, rows.Err()
}

func (r *Repository) Payment(ctx context.Context, id string) (domain.Payment, error) {
	p, err := scanPayment(r.db.QueryRow(ctx, `SELECT `+paymentColumns+` FROM wallet.payments WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.Payment{}, httpx.NotFound("payment")
	}
	if err != nil {
		return domain.Payment{}, fmt.Errorf("wallet: reading payment: %w", err)
	}
	return p, nil
}

// LockPayment reads a payment FOR UPDATE, so two concurrent callbacks cannot
// both settle it and credit the member twice.
func (r *Repository) LockPayment(ctx context.Context, id string) (domain.Payment, error) {
	p, err := scanPayment(r.db.QueryRow(ctx,
		`SELECT `+paymentColumns+` FROM wallet.payments WHERE id = $1 FOR UPDATE`, id))
	if database.IsNoRows(err) {
		return domain.Payment{}, httpx.NotFound("payment")
	}
	if err != nil {
		return domain.Payment{}, fmt.Errorf("wallet: locking payment: %w", err)
	}
	return p, nil
}

func (r *Repository) InsertPayment(ctx context.Context, p domain.Payment, externalID *string) (domain.Payment, error) {
	created, err := scanPayment(r.db.QueryRow(ctx, `
		INSERT INTO wallet.payments (id, member_id, package_id, credits, amount_idr, discount_idr,
			total_idr, voucher_code, channel, status, external_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+paymentColumns,
		p.ID, p.MemberID, p.PackageID, p.Credits, p.AmountIDR, p.DiscountIDR, p.TotalIDR,
		p.VoucherCode, p.Channel, p.Status, externalID, p.CreatedAt))
	if err != nil {
		return domain.Payment{}, fmt.Errorf("wallet: inserting payment: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdatePaymentStatus(ctx context.Context, id string, status domain.PaymentStatus, paidAt, refundedAt *time.Time) (domain.Payment, error) {
	updated, err := scanPayment(r.db.QueryRow(ctx, `
		UPDATE wallet.payments SET status = $2, paid_at = COALESCE($3, paid_at), refunded_at = COALESCE($4, refunded_at)
		WHERE id = $1 RETURNING `+paymentColumns,
		id, status, paidAt, refundedAt))
	if database.IsNoRows(err) {
		return domain.Payment{}, httpx.NotFound("payment")
	}
	if err != nil {
		return domain.Payment{}, fmt.Errorf("wallet: updating payment: %w", err)
	}
	return updated, nil
}

// CountPurchases counts a member's paid purchases of one package, for the
// per-member purchase limit.
func (r *Repository) CountPurchases(ctx context.Context, memberID, packageID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM wallet.payments
		WHERE member_id = $1 AND package_id = $2 AND status IN ('PENDING', 'PAID')`,
		memberID, packageID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("wallet: counting purchases: %w", err)
	}
	return count, nil
}

// CountMemberPurchases counts a member's completed purchases across every
// package, which is how "new member" is decided for welcome vouchers.
func (r *Repository) CountMemberPurchases(ctx context.Context, memberID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM wallet.payments WHERE member_id = $1 AND status = 'PAID'`, memberID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("wallet: counting member purchases: %w", err)
	}
	return count, nil
}

func (r *Repository) CountPaymentsForPackage(ctx context.Context, packageID string) (int, error) {
	var count int
	if err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM wallet.payments WHERE package_id = $1`, packageID).Scan(&count); err != nil {
		return 0, fmt.Errorf("wallet: counting package payments: %w", err)
	}
	return count, nil
}

// PackageSales aggregates paid revenue per package for the admin list.
func (r *Repository) PackageSales(ctx context.Context) (map[string]PackageSale, error) {
	rows, err := r.db.Query(ctx, `
		SELECT package_id, count(*), COALESCE(SUM(total_idr), 0)
		FROM wallet.payments WHERE status = 'PAID' GROUP BY package_id`)
	if err != nil {
		return nil, fmt.Errorf("wallet: aggregating package sales: %w", err)
	}
	defer rows.Close()

	sales := map[string]PackageSale{}
	for rows.Next() {
		var packageID string
		var sale PackageSale
		if err := rows.Scan(&packageID, &sale.PurchaseCount, &sale.RevenueIDR); err != nil {
			return nil, fmt.Errorf("wallet: scanning package sale: %w", err)
		}
		sales[packageID] = sale
	}
	return sales, rows.Err()
}

// PackageSale is how one package has sold.
type PackageSale struct {
	PurchaseCount int
	RevenueIDR    int64
}

// ── Vouchers ─────────────────────────────────────────────────────────────────

const voucherColumns = `id, code, type, value, starts_at, ends_at, usage_limit, per_member_limit,
	eligible_segment, applicable_package_ids, status, created_at`

func scanVoucher(row pgx.Row) (domain.Voucher, error) {
	var v domain.Voucher
	var packages []byte
	if err := row.Scan(&v.ID, &v.Code, &v.Type, &v.Value, &v.StartsAt, &v.EndsAt, &v.UsageLimit,
		&v.PerMemberLimit, &v.EligibleSegment, &packages, &v.Status, &v.CreatedAt); err != nil {
		return domain.Voucher{}, err
	}
	if len(packages) > 0 {
		if err := json.Unmarshal(packages, &v.ApplicablePackageIDs); err != nil {
			return domain.Voucher{}, fmt.Errorf("wallet: decoding voucher packages: %w", err)
		}
	}
	return v, nil
}

func (r *Repository) Vouchers(ctx context.Context) ([]domain.Voucher, error) {
	rows, err := r.db.Query(ctx, `SELECT `+voucherColumns+` FROM wallet.vouchers ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("wallet: listing vouchers: %w", err)
	}
	defer rows.Close()

	vouchers := []domain.Voucher{}
	for rows.Next() {
		v, err := scanVoucher(rows)
		if err != nil {
			return nil, fmt.Errorf("wallet: scanning voucher: %w", err)
		}
		vouchers = append(vouchers, v)
	}
	return vouchers, rows.Err()
}

// LiveVouchers are the codes a member could actually use right now, which is
// what the promo rail on the home screen shows.
func (r *Repository) LiveVouchers(ctx context.Context, now time.Time) ([]domain.Voucher, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+voucherColumns+` FROM wallet.vouchers
		WHERE status = 'ACTIVE' AND starts_at <= $1 AND ends_at >= $1
		ORDER BY ends_at`, now)
	if err != nil {
		return nil, fmt.Errorf("wallet: listing live vouchers: %w", err)
	}
	defer rows.Close()

	vouchers := []domain.Voucher{}
	for rows.Next() {
		v, err := scanVoucher(rows)
		if err != nil {
			return nil, fmt.Errorf("wallet: scanning voucher: %w", err)
		}
		vouchers = append(vouchers, v)
	}
	return vouchers, rows.Err()
}

func (r *Repository) Voucher(ctx context.Context, id string) (domain.Voucher, error) {
	v, err := scanVoucher(r.db.QueryRow(ctx, `SELECT `+voucherColumns+` FROM wallet.vouchers WHERE id = $1`, id))
	if database.IsNoRows(err) {
		return domain.Voucher{}, httpx.NotFound("voucher")
	}
	if err != nil {
		return domain.Voucher{}, fmt.Errorf("wallet: reading voucher: %w", err)
	}
	return v, nil
}

func (r *Repository) VoucherByCode(ctx context.Context, code string) (domain.Voucher, error) {
	v, err := scanVoucher(r.db.QueryRow(ctx,
		`SELECT `+voucherColumns+` FROM wallet.vouchers WHERE upper(code) = upper($1)`, strings.TrimSpace(code)))
	if database.IsNoRows(err) {
		return domain.Voucher{}, httpx.NotFound("voucher")
	}
	if err != nil {
		return domain.Voucher{}, fmt.Errorf("wallet: reading voucher by code: %w", err)
	}
	return v, nil
}

func (r *Repository) InsertVoucher(ctx context.Context, v domain.Voucher) (domain.Voucher, error) {
	packages, err := marshalIDs(v.ApplicablePackageIDs)
	if err != nil {
		return domain.Voucher{}, err
	}
	created, err := scanVoucher(r.db.QueryRow(ctx, `
		INSERT INTO wallet.vouchers (id, code, type, value, starts_at, ends_at, usage_limit,
			per_member_limit, eligible_segment, applicable_package_ids, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING `+voucherColumns,
		v.ID, v.Code, v.Type, v.Value, v.StartsAt, v.EndsAt, v.UsageLimit,
		v.PerMemberLimit, v.EligibleSegment, packages, v.Status, v.CreatedAt))
	if database.IsUniqueViolation(err) {
		return domain.Voucher{}, httpx.Conflict("DUPLICATE", "That voucher code already exists.")
	}
	if err != nil {
		return domain.Voucher{}, fmt.Errorf("wallet: inserting voucher: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateVoucher(ctx context.Context, v domain.Voucher) (domain.Voucher, error) {
	packages, err := marshalIDs(v.ApplicablePackageIDs)
	if err != nil {
		return domain.Voucher{}, err
	}
	updated, err := scanVoucher(r.db.QueryRow(ctx, `
		UPDATE wallet.vouchers SET code = $2, type = $3, value = $4, starts_at = $5, ends_at = $6,
			usage_limit = $7, per_member_limit = $8, eligible_segment = $9,
			applicable_package_ids = $10, status = $11, updated_at = now()
		WHERE id = $1 RETURNING `+voucherColumns,
		v.ID, v.Code, v.Type, v.Value, v.StartsAt, v.EndsAt, v.UsageLimit,
		v.PerMemberLimit, v.EligibleSegment, packages, v.Status))
	if database.IsNoRows(err) {
		return domain.Voucher{}, httpx.NotFound("voucher")
	}
	if database.IsUniqueViolation(err) {
		return domain.Voucher{}, httpx.Conflict("DUPLICATE", "That voucher code already exists.")
	}
	if err != nil {
		return domain.Voucher{}, fmt.Errorf("wallet: updating voucher: %w", err)
	}
	return updated, nil
}

func (r *Repository) DeleteVoucher(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM wallet.vouchers WHERE id = $1`, id)
	if database.IsForeignKeyViolation(err) {
		return httpx.ErrInUse.WithMessage("This voucher has been redeemed. Disable it instead.")
	}
	if err != nil {
		return fmt.Errorf("wallet: deleting voucher: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return httpx.NotFound("voucher")
	}
	return nil
}

// InsertRedemption records one use of a voucher.
func (r *Repository) InsertRedemption(ctx context.Context, red domain.VoucherRedemption) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO wallet.voucher_redemptions (id, voucher_id, member_id, payment_id, discount_idr, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		red.ID, red.VoucherID, red.MemberID, red.PaymentID, red.DiscountIDR, red.CreatedAt)
	if database.IsUniqueViolation(err) {
		// The payment already carries a discount; redeeming twice would double it.
		return nil
	}
	if err != nil {
		return fmt.Errorf("wallet: inserting redemption: %w", err)
	}
	return nil
}

// RedemptionCounts returns total and per-member usage, which the voucher rules
// check before allowing another redemption.
func (r *Repository) RedemptionCounts(ctx context.Context, voucherID, memberID string) (total int, byMember int, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE member_id = $2)
		FROM wallet.voucher_redemptions WHERE voucher_id = $1`, voucherID, memberID).
		Scan(&total, &byMember)
	if err != nil {
		return 0, 0, fmt.Errorf("wallet: counting redemptions: %w", err)
	}
	return total, byMember, nil
}

// RedemptionTotals counts redemptions per voucher for the admin list.
func (r *Repository) RedemptionTotals(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.Query(ctx,
		`SELECT voucher_id, count(*) FROM wallet.voucher_redemptions GROUP BY voucher_id`)
	if err != nil {
		return nil, fmt.Errorf("wallet: counting redemptions: %w", err)
	}
	defer rows.Close()

	totals := map[string]int{}
	for rows.Next() {
		var voucherID string
		var count int
		if err := rows.Scan(&voucherID, &count); err != nil {
			return nil, fmt.Errorf("wallet: scanning redemption total: %w", err)
		}
		totals[voucherID] = count
	}
	return totals, rows.Err()
}

func marshalIDs(ids []string) ([]byte, error) {
	if ids == nil {
		return nil, nil
	}
	body, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("wallet: encoding id list: %w", err)
	}
	return body, nil
}
