package seed

import (
	"context"
	"fmt"
	"time"
)

// The supply chain, end to end: request, order, delivery, receipt, return,
// credit note, payment.
//
// These tables only make sense together. A goods receipt with no order behind
// it, or a payable with no invoice, is not a smaller demo — it is a broken
// one, because every screen in Purchasing is a join. So the fixtures below
// describe whole orders and the seeder derives the rest: the receipt lines
// come from the order lines, the return comes from what was rejected at QC,
// the credit note comes from the return, and the payment applies it.
//
// The five orders are chosen to put one of each state on the screen at once:
// one settled, one part-received and overdue, one still in transit, one that
// went wrong and generated a credit, and one still being typed.

type poLine struct {
	item, desc, unit string
	qty              int
	price            int64
	// received is what actually turned up, and rejected what failed QC. Zero
	// received means nothing has arrived yet.
	received, rejected int
	batch              string
	expiresInDays      int
}

type termSeed struct {
	label   string
	percent int
	dueIn   int // days from the order date; negative is already overdue
	status  string
}

type poSeed struct {
	id, number, supplier, branch, status string
	daysAgo, expectedIn                  int
	note                                 string
	lines                                []poLine
	terms                                []termSeed
}

var purchaseOrders = []poSeed{
	{
		id: "po_seed_1", number: "PO-2609-001", supplier: "sup_nutrition",
		branch: "brn_senopati", status: "RECEIVED", daysAgo: 45, expectedIn: 7,
		note: "Monthly retail restock.",
		lines: []poLine{
			{item: "itm_whey", desc: "Whey Protein 1kg", unit: "PCS", qty: 24, price: 480_000, received: 24, batch: "WHY-2609", expiresInDays: 420},
			{item: "itm_bar", desc: "Protein Bar (Chocolate)", unit: "PCS", qty: 200, price: 18_000, received: 200, batch: "BAR-2609", expiresInDays: 210},
			{item: "itm_iso", desc: "Isotonic Drink 500ml", unit: "PCS", qty: 120, price: 12_000, received: 120, batch: "ISO-2609", expiresInDays: 180},
		},
		terms: []termSeed{
			{label: "50% deposit", percent: 50, dueIn: 0, status: "PAID"},
			{label: "Balance on delivery", percent: 50, dueIn: 14, status: "PAID"},
		},
	},
	{
		id: "po_seed_2", number: "PO-2609-002", supplier: "sup_apparel",
		branch: "brn_senopati", status: "PARTIALLY_RECEIVED", daysAgo: 30, expectedIn: 21,
		note: "Q4 apparel drop. Tanks back-ordered.",
		lines: []poLine{
			{item: "itm_tee", desc: "NuHabit Training Tee", unit: "PCS", qty: 60, price: 85_000, received: 60},
			{item: "itm_tank", desc: "NuHabit Tank Top", unit: "PCS", qty: 60, price: 78_000, received: 20},
		},
		terms: []termSeed{
			{label: "50% deposit", percent: 50, dueIn: 0, status: "PAID"},
			// Due a fortnight ago and still open: the row the payables screen
			// is built to put at the top.
			{label: "Balance NET30", percent: 50, dueIn: 16, status: "PENDING"},
		},
	},
	{
		id: "po_seed_3", number: "PO-2609-003", supplier: "sup_equipment",
		branch: "brn_pik", status: "SENT", daysAgo: 8, expectedIn: 30,
		note: "PIK studio second rig.",
		lines: []poLine{
			{item: "itm_grips", desc: "Training Grips", unit: "PAIR", qty: 40, price: 95_000},
			{item: "itm_bottle", desc: "Steel Bottle 750ml", unit: "PCS", qty: 50, price: 62_000},
		},
		terms: []termSeed{
			{label: "NET45", percent: 100, dueIn: 45, status: "PENDING"},
		},
	},
	{
		id: "po_seed_4", number: "PO-2609-004", supplier: "sup_supplies",
		branch: "brn_senopati", status: "RECEIVED", daysAgo: 18, expectedIn: 5,
		note: "Cleaning supplies. Part of the shipment arrived damaged.",
		lines: []poLine{
			{item: "itm_wipes", desc: "Equipment Wipes (500)", unit: "TUB", qty: 30, price: 145_000, received: 24, rejected: 6},
			{item: "itm_towel", desc: "Gym Towel", unit: "PCS", qty: 100, price: 22_000, received: 100},
		},
		terms: []termSeed{
			{label: "COD", percent: 100, dueIn: 5, status: "PAID"},
		},
	},
	{
		id: "po_seed_5", number: "PO-2609-005", supplier: "sup_nutrition",
		branch: "brn_pik", status: "DRAFT", daysAgo: 1, expectedIn: 14,
		note: "PIK opening stock — awaiting approval.",
		lines: []poLine{
			{item: "itm_bar", desc: "Protein Bar (Chocolate)", unit: "PCS", qty: 150, price: 18_000},
			{item: "itm_shaker", desc: "Protein Shaker", unit: "PCS", qty: 40, price: 35_000},
		},
	},
}

const purchaseTaxPercent = 11

func (s *Seeder) seedPurchasingActivity(ctx context.Context) (int, error) {
	if err := s.seedPurchaseRequests(ctx); err != nil {
		return 0, err
	}
	for _, po := range purchaseOrders {
		if err := s.seedPurchaseOrder(ctx, po); err != nil {
			return 0, err
		}
	}
	if err := s.seedPurchaseReturn(ctx); err != nil {
		return 0, err
	}
	if err := s.seedVendorPayments(ctx); err != nil {
		return 0, err
	}
	return len(purchaseOrders), nil
}

// seedPurchaseRequests writes the paperwork that comes before an order.
//
// One request per outcome the approval chain can reach — approved and
// converted, still waiting on Finance, and refused — because an approvals
// inbox with nothing in it cannot be told from one that is broken.
func (s *Seeder) seedPurchaseRequests(ctx context.Context) error {
	now := s.clock.Now()
	reqs := []struct {
		id, number, branch, requester, status, priority string
		daysAgo                                         int
		converted                                       string
		reason                                          string
		items                                           []poLine
	}{
		{
			id: "pr_seed_1", number: "PR-2609-001", branch: "brn_senopati",
			requester: "Dewi Front Desk", status: "CONVERTED", priority: "NORMAL",
			daysAgo: 50, converted: "po_seed_1",
			items: []poLine{
				{item: "itm_whey", desc: "Whey Protein 1kg", unit: "PCS", qty: 24, price: 480_000},
				{item: "itm_bar", desc: "Protein Bar (Chocolate)", unit: "PCS", qty: 200, price: 18_000},
			},
		},
		{
			id: "pr_seed_2", number: "PR-2609-002", branch: "brn_pik",
			requester: "Rangga Branch Manager", status: "PENDING_FINANCE", priority: "HIGH",
			daysAgo: 4,
			items: []poLine{
				{item: "itm_grips", desc: "Training Grips", unit: "PAIR", qty: 40, price: 95_000},
			},
		},
		{
			id: "pr_seed_3", number: "PR-2609-003", branch: "brn_senopati",
			requester: "Kevin Pratama", status: "REJECTED", priority: "LOW", daysAgo: 12,
			reason: "Covered by the existing equipment order — resubmit next quarter.",
			items: []poLine{
				{item: "itm_bottle", desc: "Steel Bottle 750ml", unit: "PCS", qty: 200, price: 62_000},
			},
		},
	}

	for _, r := range reqs {
		at := now.AddDate(0, 0, -r.daysAgo)
		var total int64
		for _, it := range r.items {
			total += int64(it.qty) * it.price
		}

		// The approval columns are filled in as far as the status got. A
		// request sitting with Finance has the head's approval on it and
		// nothing else, which is what the queue is sorted by.
		var headBy, headAt, finBy, finAt, dirBy, dirAt any
		var rejBy, rejAt, reason, converted any
		switch r.status {
		case "CONVERTED", "APPROVED":
			headBy, headAt = "adm_hq", at.AddDate(0, 0, 1)
			finBy, finAt = "adm_finance", at.AddDate(0, 0, 2)
			dirBy, dirAt = "adm_super", at.AddDate(0, 0, 2)
			if r.converted != "" {
				converted = r.converted
			}
		case "PENDING_FINANCE":
			headBy, headAt = "adm_hq", at.AddDate(0, 0, 1)
		case "REJECTED":
			headBy, headAt = "adm_hq", at.AddDate(0, 0, 1)
			rejBy, rejAt, reason = "adm_finance", at.AddDate(0, 0, 3), r.reason
		}

		if _, err := s.db.Exec(ctx, `
			INSERT INTO purchasing.purchase_requests
				(id, pr_number, branch_id, requester_name, status, priority, total_idr,
				 required_on, note, approved_by_head, approved_at_head,
				 approved_by_finance, approved_at_finance, approved_by_director,
				 approved_at_director, rejected_by, rejected_at, rejection_reason,
				 converted_po_id, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$20)
			ON CONFLICT (id) DO NOTHING`,
			r.id, r.number, r.branch, r.requester, r.status, r.priority, total,
			at.AddDate(0, 0, 21), "", headBy, headAt, finBy, finAt, dirBy, dirAt,
			rejBy, rejAt, reason, converted, at); err != nil {
			return fmt.Errorf("seed: purchase request %s: %w", r.id, err)
		}

		for i, it := range r.items {
			if _, err := s.db.Exec(ctx, `
				INSERT INTO purchasing.purchase_request_items
					(id, request_id, item_id, description, qty, unit, estimated_price_idr)
				VALUES ($1,$2,$3,$4,$5,$6,$7)
				ON CONFLICT (id) DO NOTHING`,
				fmt.Sprintf("%s_i%d", r.id, i+1), r.id, it.item, it.desc,
				it.qty, it.unit, it.price); err != nil {
				return fmt.Errorf("seed: request item %s#%d: %w", r.id, i+1, err)
			}
		}
	}
	return nil
}

// seedPurchaseOrder writes one order and everything downstream of it that its
// status implies: the delivery that brought the goods, the receipt that
// booked them in, and the instalments the studio owes for them.
func (s *Seeder) seedPurchaseOrder(ctx context.Context, po poSeed) error {
	now := s.clock.Now()
	ordered := now.AddDate(0, 0, -po.daysAgo)

	var subtotal int64
	for _, l := range po.lines {
		subtotal += int64(l.qty) * l.price
	}
	tax := subtotal * purchaseTaxPercent / 100
	total := subtotal + tax

	var approvedBy, approvedAt, sentAt any
	if po.status != "DRAFT" {
		approvedBy, approvedAt = "adm_finance", ordered.AddDate(0, 0, 1)
		sentAt = ordered.AddDate(0, 0, 1)
	}
	var request any
	if po.id == "po_seed_1" {
		request = "pr_seed_1"
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO purchasing.purchase_orders
			(id, po_number, supplier_id, branch_id, request_id, status, ordered_on,
			 expected_on, subtotal_idr, discount_idr, tax_percent, tax_idr, total_idr,
			 ship_to, note, approved_by, approved_at, sent_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,0,$10,$11,$12,$13,$14,$15,$16,$17,$18,$18)
		ON CONFLICT (id) DO NOTHING`,
		po.id, po.number, po.supplier, po.branch, request, po.status, ordered,
		ordered.AddDate(0, 0, po.expectedIn), subtotal, purchaseTaxPercent, tax, total,
		po.branch, po.note, approvedBy, approvedAt, sentAt, ordered); err != nil {
		return fmt.Errorf("seed: purchase order %s: %w", po.id, err)
	}

	for i, l := range po.lines {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO purchasing.purchase_order_items
				(id, order_id, item_id, description, qty_ordered, qty_received, unit,
				 unit_price_idr, discount_idr)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,0)
			ON CONFLICT (id) DO NOTHING`,
			lineID(po.id, i), po.id, l.item, l.desc, l.qty, l.received, l.unit,
			l.price); err != nil {
			return fmt.Errorf("seed: order item %s#%d: %w", po.id, i+1, err)
		}
	}

	for i, t := range po.terms {
		due := ordered.AddDate(0, 0, t.dueIn)
		amount := total * int64(t.percent) / 100
		paid := int64(0)
		if t.status == "PAID" {
			paid = amount
		}
		if _, err := s.db.Exec(ctx, `
			INSERT INTO purchasing.payment_terms
				(id, order_id, sequence, label, due_on, percent, amount_idr, status,
				 paid_idr, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)
			ON CONFLICT (id) DO NOTHING`,
			termID(po.id, i), po.id, i+1, t.label, due, t.percent, amount,
			t.status, paid, ordered); err != nil {
			return fmt.Errorf("seed: payment term %s#%d: %w", po.id, i+1, err)
		}
	}

	return s.seedReceipt(ctx, po, ordered)
}

// seedReceipt books in whatever arrived.
//
// Nothing arrives on a DRAFT or SENT order, and an order where nothing
// arrived gets no delivery note and no receipt — the absence is the state,
// not an omission.
func (s *Seeder) seedReceipt(ctx context.Context, po poSeed, ordered time.Time) error {
	var arrived []int
	for i, l := range po.lines {
		if l.received > 0 || l.rejected > 0 {
			arrived = append(arrived, i)
		}
	}
	if len(arrived) == 0 {
		return nil
	}

	on := ordered.AddDate(0, 0, po.expectedIn)
	deliveryID := "dlv_" + po.id
	receiptID := "grn_" + po.id
	note := "DN-" + po.number

	if _, err := s.db.Exec(ctx, `
		INSERT INTO purchasing.deliveries
			(id, delivery_number, order_id, supplier_id, branch_id, delivery_note_number,
			 driver_name, vehicle, arrived_on, arrived_at, received_by, received_by_name,
			 status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::date,$9,'adm_desk','Dewi Front Desk','INSPECTED',$9,$9)
		ON CONFLICT (id) DO NOTHING`,
		deliveryID, "DLV-"+po.number[3:], po.id, po.supplier, po.branch, note,
		"Pak Yusuf", "B 9021 KXA", on); err != nil {
		return fmt.Errorf("seed: delivery %s: %w", po.id, err)
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO purchasing.goods_receipts
			(id, grn_number, order_id, supplier_id, branch_id, received_on, received_by,
			 received_by_name, delivery_note_number, status, posted_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,'adm_desk','Dewi Front Desk',$7,'POSTED',$8,$8,$8)
		ON CONFLICT (id) DO NOTHING`,
		receiptID, "GRN-"+po.number[3:], po.id, po.supplier, po.branch, on,
		note, on); err != nil {
		return fmt.Errorf("seed: goods receipt %s: %w", po.id, err)
	}

	for _, i := range arrived {
		l := po.lines[i]
		qc := "ACCEPTED"
		switch {
		case l.received == 0:
			qc = "REJECTED"
		case l.rejected > 0:
			qc = "PARTIALLY_REJECTED"
		}

		var expires any
		var batch any
		if l.batch != "" {
			batch = l.batch
			expires = on.AddDate(0, 0, l.expiresInDays)
		}

		if _, err := s.db.Exec(ctx, `
			INSERT INTO purchasing.delivery_items
				(id, delivery_id, order_item_id, item_id, qty_delivered, unit,
				 batch_number, expires_on)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (id) DO NOTHING`,
			fmt.Sprintf("dli_%s_%d", po.id, i+1), deliveryID, lineID(po.id, i),
			l.item, l.received+l.rejected, l.unit, batch, expires); err != nil {
			return fmt.Errorf("seed: delivery item %s#%d: %w", po.id, i+1, err)
		}

		if _, err := s.db.Exec(ctx, `
			INSERT INTO purchasing.goods_receipt_items
				(id, receipt_id, order_item_id, item_id, qty_accepted, qty_rejected,
				 unit_price_idr, qc_status, batch_number, expires_on, note)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (id) DO NOTHING`,
			receiptItemID(po.id, i), receiptID, lineID(po.id, i), l.item,
			l.received, l.rejected, l.price, qc, batch, expires,
			rejectNote(l.rejected)); err != nil {
			return fmt.Errorf("seed: receipt item %s#%d: %w", po.id, i+1, err)
		}
	}
	return nil
}

func rejectNote(rejected int) any {
	if rejected == 0 {
		return nil
	}
	return "Seals broken in transit."
}

func lineID(po string, i int) string        { return fmt.Sprintf("poi_%s_%d", po, i+1) }
func termID(po string, i int) string        { return fmt.Sprintf("pmt_%s_%d", po, i+1) }
func receiptItemID(po string, i int) string { return fmt.Sprintf("gri_%s_%d", po, i+1) }

// seedPurchaseReturn sends the damaged wipes back and takes the credit note
// the supplier issued for them.
//
// This is the one path in Purchasing that touches three tables nobody
// exercises by accident, and the reason the payables screen has a "settled by
// credit" column at all.
func (s *Seeder) seedPurchaseReturn(ctx context.Context) error {
	now := s.clock.Now()
	returnedOn := now.AddDate(0, 0, -10)
	const qty, price = 6, int64(145_000)
	amount := int64(qty) * price

	if _, err := s.db.Exec(ctx, `
		INSERT INTO purchasing.purchase_returns
			(id, return_number, receipt_id, supplier_id, branch_id, returned_on,
			 reason_type, reason_note, status, total_idr, posted_at, created_at, updated_at)
		VALUES ('prt_seed_1','RTN-2609-001','grn_po_seed_4','sup_supplies','brn_senopati',
		        $1,'DAMAGED','Six tubs arrived with broken seals.','POSTED',$2,$3,$3,$3)
		ON CONFLICT (id) DO NOTHING`, returnedOn, amount, returnedOn); err != nil {
		return fmt.Errorf("seed: purchase return: %w", err)
	}

	if _, err := s.db.Exec(ctx, `
		INSERT INTO purchasing.purchase_return_items
			(id, return_id, receipt_item_id, item_id, qty, unit_price_idr, note)
		VALUES ('prti_seed_1','prt_seed_1',$1,'itm_wipes',$2,$3,'Broken seals')
		ON CONFLICT (id) DO NOTHING`,
		receiptItemID("po_seed_4", 0), qty, price); err != nil {
		return fmt.Errorf("seed: purchase return item: %w", err)
	}

	// The credit note. Partly applied: the rest is still sitting with the
	// supplier, which is the balance the credit-notes screen is there to show.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO purchasing.vendor_credits
			(id, credit_number, supplier_id, return_id, issued_on, amount_idr,
			 applied_idr, status, reason, expires_on, created_at, updated_at)
		VALUES ('vcr_seed_1','CN-2609-001','sup_supplies','prt_seed_1',$1::date,$2,0,
		        'OPEN','Damaged goods returned',$3,$1,$1)
		ON CONFLICT (id) DO NOTHING`,
		returnedOn.AddDate(0, 0, 2), amount, returnedOn.AddDate(0, 6, 0)); err != nil {
		return fmt.Errorf("seed: vendor credit: %w", err)
	}

	// A second credit with nothing against it, so the screen shows both an
	// applied and an unapplied balance.
	if _, err := s.db.Exec(ctx, `
		INSERT INTO purchasing.vendor_credits
			(id, credit_number, supplier_id, issued_on, amount_idr, applied_idr,
			 status, reason, expires_on, created_at, updated_at)
		VALUES ('vcr_seed_2','CN-2609-002','sup_apparel',$1::date,450000,0,'OPEN',
		        'Goodwill credit for the late tank delivery',$2,$1,$1)
		ON CONFLICT (id) DO NOTHING`,
		now.AddDate(0, 0, -5), now.AddDate(0, 6, 0)); err != nil {
		return fmt.Errorf("seed: goodwill credit: %w", err)
	}
	return nil
}

// seedVendorPayments settles the terms marked PAID, and applies the credit
// note to the one it belongs to.
//
// Driven off the terms rather than listed separately: a payment that does not
// match the instalment it paid is exactly the discrepancy Finance opens this
// screen to find, and seeding one would be seeding a bug.
func (s *Seeder) seedVendorPayments(ctx context.Context) error {
	rows, err := s.db.Query(ctx, `
		SELECT t.id, t.order_id, o.supplier_id, t.due_on, t.amount_idr::bigint, t.sequence
		FROM purchasing.payment_terms t
		JOIN purchasing.purchase_orders o ON o.id = t.order_id
		WHERE t.status = 'PAID'
		ORDER BY t.order_id, t.sequence`)
	if err != nil {
		return fmt.Errorf("seed: paid terms: %w", err)
	}
	defer rows.Close()

	type payment struct {
		termID, orderID, supplier string
		paidOn                    time.Time
		amount                    int64
		sequence                  int
	}
	var payments []payment
	for rows.Next() {
		var p payment
		if err := rows.Scan(&p.termID, &p.orderID, &p.supplier, &p.paidOn,
			&p.amount, &p.sequence); err != nil {
			return fmt.Errorf("seed: scanning paid term: %w", err)
		}
		payments = append(payments, p)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("seed: reading paid terms: %w", err)
	}

	for i, p := range payments {
		id := fmt.Sprintf("vpy_seed_%02d", i+1)

		// The wipes order was settled partly with the credit note from the
		// damaged tubs. Everything else was a plain transfer.
		var credit int64
		if p.orderID == "po_seed_4" {
			credit = 870_000
		}

		if _, err := s.db.Exec(ctx, `
			INSERT INTO purchasing.vendor_payments
				(id, payment_number, supplier_id, order_id, term_id, paid_on, amount_idr,
				 credit_idr, method, reference, status, posted_at, posted_by,
				 created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'POSTED',$11,'adm_finance',$11,$11)
			ON CONFLICT (id) DO NOTHING`,
			id, fmt.Sprintf("VP-2609-%03d", i+1), p.supplier, p.orderID, p.termID,
			p.paidOn, p.amount, credit, method(credit), "TRF/"+id, p.paidOn); err != nil {
			return fmt.Errorf("seed: vendor payment %s: %w", id, err)
		}

		if credit == 0 {
			continue
		}
		if _, err := s.db.Exec(ctx, `
			INSERT INTO purchasing.credit_applications
				(id, credit_id, payment_id, amount_idr, applied_at)
			VALUES ($1,'vcr_seed_1',$2,$3,$4)
			ON CONFLICT (credit_id, payment_id) DO NOTHING`,
			"vca_"+id, id, credit, p.paidOn); err != nil {
			return fmt.Errorf("seed: credit application %s: %w", id, err)
		}
		if _, err := s.db.Exec(ctx, `
			UPDATE purchasing.vendor_credits
			SET applied_idr = $1,
			    status = CASE WHEN $1 >= amount_idr THEN 'APPLIED' ELSE 'PARTIALLY_APPLIED' END
			WHERE id = 'vcr_seed_1'`, credit); err != nil {
			return fmt.Errorf("seed: credit balance: %w", err)
		}
	}
	return nil
}

// method names how the money moved. A payment carrying a credit note is not a
// transfer, and calling it one is how a reconciliation goes missing.
func method(credit int64) string {
	if credit > 0 {
		return "CREDIT_NOTE"
	}
	return "TRANSFER"
}
