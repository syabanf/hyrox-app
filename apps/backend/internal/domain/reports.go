package domain

import (
	"math"
	"sort"
	"time"
)

// The arithmetic behind the reports.
//
// These live here rather than in SQL for one reason: a number a manager acts
// on should be produced by something with a test around it. Aggregation is
// where reporting bugs hide, because a wrong total looks exactly like a right
// one until somebody checks it against reality.

// ── Selling ──────────────────────────────────────────────────────────────────

// SalesBucket is one slice of takings: an hour, a day, a payment method.
type SalesBucket struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Orders    int      `json:"orders"`
	Items     Quantity `json:"items"`
	SalesIDR  float64  `json:"salesIdr"`
	CostIDR   float64  `json:"costIdr"`
	ProfitIDR float64  `json:"profitIdr"`
	// Share is this bucket's part of the whole, as a percentage. Computed
	// after every bucket is known, which is why it is not a field a query
	// could fill in.
	Share float64 `json:"share"`
}

// ShareOut fills in each bucket's percentage of the total.
//
// Percentages are computed once, at the end, from the same total — working
// them out per row against a running sum is how a report's shares end up
// adding to 103%.
func ShareOut(buckets []SalesBucket) []SalesBucket {
	var total float64
	for _, bucket := range buckets {
		total += bucket.SalesIDR
	}
	if total <= 0 {
		return buckets
	}
	for i := range buckets {
		buckets[i].Share = math.Round(buckets[i].SalesIDR/total*10000) / 100
	}
	return buckets
}

// RushHour is when a counter is busy.
type RushHour struct {
	// Hours is 24 buckets, always all of them: an hour with no sales is a
	// fact about the shop, and dropping it makes the quiet ones invisible.
	Hours []SalesBucket `json:"hours"`
	// Busiest and quietest are by takings rather than by transaction count.
	// Ten people buying a bottle is not a rush.
	BusiestHour  int `json:"busiestHour"`
	QuietestHour int `json:"quietestHour"`
	// PeakShare is what the busiest hour is worth, which is the number that
	// decides whether a second till is worth staffing.
	PeakShare float64 `json:"peakShare"`
}

// SummarizeRushHour buckets sales by hour of the day.
//
// Every hour appears, including the empty ones. A chart with holes in it reads
// as missing data rather than as a quiet Tuesday afternoon.
func SummarizeRushHour(sales []HourlySale) RushHour {
	hours := make([]SalesBucket, 24)
	for hour := range hours {
		hours[hour] = SalesBucket{
			Key: twoDigits(hour), Label: twoDigits(hour) + ":00",
		}
	}
	for _, sale := range sales {
		if sale.Hour < 0 || sale.Hour > 23 {
			continue
		}
		bucket := &hours[sale.Hour]
		bucket.Orders += sale.Orders
		bucket.SalesIDR += sale.SalesIDR
		bucket.Items += sale.Items
	}

	report := RushHour{Hours: ShareOut(hours)}
	busiest, quietest := 0, -1
	for hour, bucket := range report.Hours {
		if bucket.SalesIDR > report.Hours[busiest].SalesIDR {
			busiest = hour
		}
		// The quietest hour is the quietest *trading* hour. A shop that is
		// shut at 4am is not having a bad hour, it is closed.
		if bucket.Orders > 0 && (quietest < 0 || bucket.SalesIDR < report.Hours[quietest].SalesIDR) {
			quietest = hour
		}
	}
	if quietest < 0 {
		quietest = busiest
	}
	report.BusiestHour, report.QuietestHour = busiest, quietest
	report.PeakShare = report.Hours[busiest].Share
	return report
}

// HourlySale is one hour's takings, as the query returns them.
type HourlySale struct {
	Hour     int
	Orders   int
	Items    Quantity
	SalesIDR float64
}

func twoDigits(v int) string {
	if v < 10 {
		return "0" + string(rune('0'+v))
	}
	return string(rune('0'+v/10)) + string(rune('0'+v%10))
}

// ProfitLine is one product's contribution.
type ProfitLine struct {
	ProductID   string   `json:"productId"`
	ProductName string   `json:"productName"`
	Qty         Quantity `json:"qty"`
	SalesIDR    float64  `json:"salesIdr"`
	CostIDR     float64  `json:"costIdr"`
	ProfitIDR   float64  `json:"profitIdr"`
	// MarginPercent is against what it sold for, not against what it cost.
	// Margin and markup are different numbers and confusing them is how a
	// shop believes it makes 60% on something it makes 37% on.
	MarginPercent float64 `json:"marginPercent"`
}

// RankProfit orders products by what they actually contributed.
//
// By total profit, not by margin: a 70% margin on something that sells twice a
// month matters less than 25% on the thing everybody buys, and ranking by
// margin quietly recommends dropping the second one.
func RankProfit(lines []ProfitLine) []ProfitLine {
	ranked := append([]ProfitLine(nil), lines...)
	for i := range ranked {
		ranked[i].ProfitIDR = round2(ranked[i].SalesIDR - ranked[i].CostIDR)
		if ranked[i].SalesIDR > 0 {
			ranked[i].MarginPercent = math.Round(ranked[i].ProfitIDR/ranked[i].SalesIDR*10000) / 100
		}
	}
	sort.SliceStable(ranked, func(a, b int) bool {
		return ranked[a].ProfitIDR > ranked[b].ProfitIDR
	})
	return ranked
}

// ClosingReport is what a till should have in it at the end of a shift, and
// what it did.
type ClosingReport struct {
	ShiftID     string      `json:"shiftId"`
	ShiftNumber string      `json:"shiftNumber"`
	CashierName string      `json:"cashierName"`
	OpenedAt    time.Time   `json:"openedAt"`
	ClosedAt    *time.Time  `json:"closedAt"`
	Totals      ShiftTotals `json:"totals"`
	// ByMethod is what came in through each tender, which is what a manager
	// reconciles against the card terminal's own printout.
	ByMethod []SalesBucket `json:"byMethod"`
	// Counted, expected and the difference between them: the only three
	// numbers a cash-up is actually about.
	CountedCashIDR  *float64 `json:"countedCashIdr"`
	ExpectedCashIDR float64  `json:"expectedCashIdr"`
	VarianceIDR     float64  `json:"varianceIdr"`
	// Short is a drawer with less in it than it should have. Named because
	// over and short are different conversations.
	Short bool `json:"short"`
}

// ── Buying ───────────────────────────────────────────────────────────────────

// SupplierPerformance is how a supplier actually behaves, as opposed to what
// their price list says.
type SupplierPerformance struct {
	SupplierID   string  `json:"supplierId"`
	SupplierName string  `json:"supplierName"`
	Orders       int     `json:"orders"`
	SpendIDR     float64 `json:"spendIdr"`
	// OnTimeRate and FillRate are the two numbers worth arguing with a
	// supplier about: did it turn up when they said, and was it all there.
	OnTimeRate float64 `json:"onTimeRate"`
	FillRate   float64 `json:"fillRate"`
	// RejectRate is the share of delivered goods that failed inspection.
	RejectRate float64 `json:"rejectRate"`
	// AverageLeadDays is measured, not quoted.
	AverageLeadDays float64 `json:"averageLeadDays"`
	Score           float64 `json:"score"`
}

// SupplierDelivery is one order's outcome, as the query returns it.
type SupplierDelivery struct {
	SupplierID   string
	SupplierName string
	TotalIDR     float64
	QtyOrdered   Quantity
	QtyAccepted  Quantity
	QtyRejected  Quantity
	// LeadDays is order date to delivery date. Negative is impossible and
	// treated as zero rather than dragging an average below reality.
	LeadDays int
	OnTime   bool
}

// RankSuppliers scores suppliers on how they behave.
//
// The score weights fill rate most heavily, then on-time, then quality. Stock
// that never arrived cannot be sold at any price, which is why a supplier who
// is cheap and short is worse than one who is dear and complete.
func RankSuppliers(deliveries []SupplierDelivery) []SupplierPerformance {
	type accumulator struct {
		SupplierPerformance
		onTime   int
		leadDays int
		ordered  Quantity
		accepted Quantity
		rejected Quantity
	}
	byID := map[string]*accumulator{}

	for _, delivery := range deliveries {
		acc, ok := byID[delivery.SupplierID]
		if !ok {
			acc = &accumulator{SupplierPerformance: SupplierPerformance{
				SupplierID: delivery.SupplierID, SupplierName: delivery.SupplierName,
			}}
			byID[delivery.SupplierID] = acc
		}
		acc.Orders++
		acc.SpendIDR += delivery.TotalIDR
		acc.ordered += delivery.QtyOrdered
		acc.accepted += delivery.QtyAccepted
		acc.rejected += delivery.QtyRejected
		if delivery.OnTime {
			acc.onTime++
		}
		if delivery.LeadDays > 0 {
			acc.leadDays += delivery.LeadDays
		}
	}

	out := make([]SupplierPerformance, 0, len(byID))
	for _, acc := range byID {
		performance := acc.SupplierPerformance
		performance.SpendIDR = round2(performance.SpendIDR)
		if acc.Orders > 0 {
			performance.OnTimeRate = percent(float64(acc.onTime), float64(acc.Orders))
			performance.AverageLeadDays = math.Round(float64(acc.leadDays)/float64(acc.Orders)*10) / 10
		}
		if acc.ordered > 0 {
			performance.FillRate = percent(float64(acc.accepted), float64(acc.ordered))
		}
		if delivered := acc.accepted + acc.rejected; delivered > 0 {
			performance.RejectRate = percent(float64(acc.rejected), float64(delivered))
		}
		// Fill rate carries half the weight: goods that never arrived cannot
		// be sold at any price.
		performance.Score = math.Round(
			(performance.FillRate*0.5+performance.OnTimeRate*0.3+
				(100-performance.RejectRate)*0.2)*10) / 10
		out = append(out, performance)
	}

	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Score != out[b].Score {
			return out[a].Score > out[b].Score
		}
		return out[a].SpendIDR > out[b].SpendIDR
	})
	return out
}

func percent(part, whole float64) float64 {
	if whole <= 0 {
		return 0
	}
	return math.Round(part/whole*10000) / 100
}

// ── Stock ────────────────────────────────────────────────────────────────────

// ValuationLine is what one item's stock is worth.
type ValuationLine struct {
	ItemID    string   `json:"itemId"`
	SKU       string   `json:"sku"`
	Name      string   `json:"name"`
	Unit      string   `json:"unit"`
	QtyOnHand Quantity `json:"qtyOnHand"`
	UnitCost  float64  `json:"unitCostIdr"`
	ValueIDR  float64  `json:"valueIdr"`
	Share     float64  `json:"share"`
}

// ValuationReport is the whole shelf, valued.
type ValuationReport struct {
	Lines    []ValuationLine `json:"lines"`
	TotalIDR float64         `json:"totalIdr"`
	Items    int             `json:"items"`
	// Concentration is what share of the money sits in the top five items.
	// A shop with 80% of its capital in five lines has a different problem
	// from one with it spread across two hundred.
	Concentration float64 `json:"concentration"`
}

// Value totals a shelf and says where the money is.
func Value(lines []ValuationLine) ValuationReport {
	report := ValuationReport{Lines: []ValuationLine{}}
	for _, line := range lines {
		line.ValueIDR = round2(float64(line.QtyOnHand) * line.UnitCost)
		report.TotalIDR += line.ValueIDR
		report.Lines = append(report.Lines, line)
	}
	report.TotalIDR = round2(report.TotalIDR)
	report.Items = len(report.Lines)

	sort.SliceStable(report.Lines, func(a, b int) bool {
		return report.Lines[a].ValueIDR > report.Lines[b].ValueIDR
	})
	if report.TotalIDR > 0 {
		var top float64
		for i, line := range report.Lines {
			report.Lines[i].Share = math.Round(line.ValueIDR/report.TotalIDR*10000) / 100
			if i < 5 {
				top += line.ValueIDR
			}
		}
		report.Concentration = math.Round(top/report.TotalIDR*10000) / 100
	}
	return report
}

// StockCardEntry is one line of an item's history at one branch: every
// movement, with the running balance beside it.
type StockCardEntry struct {
	MovementID string    `json:"movementId"`
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	Reference  string    `json:"reference"`
	// In and Out are separated rather than signed, because that is how a
	// stock card is read and added up by hand when somebody disputes it.
	In       Quantity `json:"in"`
	Out      Quantity `json:"out"`
	Balance  Quantity `json:"balance"`
	UnitCost float64  `json:"unitCostIdr"`
	ValueIDR float64  `json:"valueIdr"`
}
