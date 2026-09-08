package domain

import "testing"

func TestSharesAddUpToAHundred(t *testing.T) {
	// Percentages worked out per row against a running sum is how a report's
	// shares come to 103%. They are computed once, at the end, from one total.
	buckets := ShareOut([]SalesBucket{
		{Key: "a", SalesIDR: 300_000},
		{Key: "b", SalesIDR: 200_000},
		{Key: "c", SalesIDR: 500_000},
	})
	var total float64
	for _, bucket := range buckets {
		total += bucket.Share
	}
	if total != 100 {
		t.Fatalf("shares should add to 100, got %v", total)
	}
	if buckets[2].Share != 50 {
		t.Fatalf("500.000 of a million is 50%%, got %v", buckets[2].Share)
	}

	// Nothing sold is not a division by zero.
	empty := ShareOut([]SalesBucket{{Key: "a"}, {Key: "b"}})
	if empty[0].Share != 0 {
		t.Fatalf("no sales means no shares, got %v", empty[0].Share)
	}
}

func TestRushHourKeepsTheQuietHoursAndIgnoresClosedOnes(t *testing.T) {
	report := SummarizeRushHour([]HourlySale{
		{Hour: 7, Orders: 12, SalesIDR: 900_000},
		{Hour: 12, Orders: 3, SalesIDR: 200_000},
		{Hour: 18, Orders: 20, SalesIDR: 2_400_000},
	})

	// Every hour appears. A chart with holes in it reads as missing data
	// rather than as a quiet Tuesday afternoon.
	if len(report.Hours) != 24 {
		t.Fatalf("all 24 hours appear, got %d", len(report.Hours))
	}
	if report.BusiestHour != 18 {
		t.Fatalf("the evening is busiest, got %d", report.BusiestHour)
	}
	// The quietest *trading* hour is noon. A shop shut at 4am is not having a
	// bad hour, it is closed.
	if report.QuietestHour != 12 {
		t.Fatalf("the quietest trading hour is noon, got %d", report.QuietestHour)
	}
	if report.Hours[3].Label != "03:00" {
		t.Fatalf("hours are labelled by the clock, got %q", report.Hours[3].Label)
	}
}

func TestProfitRanksByContributionNotByMargin(t *testing.T) {
	// A 70% margin on something that sells twice a month matters less than 25%
	// on the thing everybody buys. Ranking by margin quietly recommends
	// dropping the second one.
	ranked := RankProfit([]ProfitLine{
		{ProductID: "rare", SalesIDR: 200_000, CostIDR: 60_000},
		{ProductID: "staple", SalesIDR: 4_000_000, CostIDR: 3_000_000},
	})

	if ranked[0].ProductID != "staple" {
		t.Fatalf("the staple contributes more, got %q first", ranked[0].ProductID)
	}
	// Margin is against what it sold for, not what it cost: confusing the two
	// is how a shop believes it makes 60% on something it makes 37% on.
	if ranked[0].MarginPercent != 25 {
		t.Fatalf("1.000.000 on 4.000.000 is a 25%% margin, got %v", ranked[0].MarginPercent)
	}
	if ranked[1].MarginPercent != 70 {
		t.Fatalf("140.000 on 200.000 is a 70%% margin, got %v", ranked[1].MarginPercent)
	}
}

func TestASupplierWhoIsCheapAndShortScoresBelowOneWhoIsCompleteb(t *testing.T) {
	ranked := RankSuppliers([]SupplierDelivery{
		// Cheap, but a third of it never arrived.
		{SupplierID: "short", SupplierName: "Short", TotalIDR: 1_000_000,
			QtyOrdered: 300, QtyAccepted: 200, LeadDays: 3, OnTime: true},
		// Dearer, complete, on time.
		{SupplierID: "solid", SupplierName: "Solid", TotalIDR: 1_400_000,
			QtyOrdered: 300, QtyAccepted: 300, LeadDays: 5, OnTime: true},
	})

	if ranked[0].SupplierID != "solid" {
		t.Fatalf("stock that never arrived cannot be sold at any price: %v", ranked)
	}
	if ranked[1].FillRate != 66.67 {
		t.Fatalf("200 of 300 is a 66.67%% fill rate, got %v", ranked[1].FillRate)
	}
	if ranked[0].AverageLeadDays != 5 {
		t.Fatalf("lead time is measured, not quoted, got %v", ranked[0].AverageLeadDays)
	}
}

func TestRejectRateIsAgainstWhatWasDeliveredNotWhatWasOrdered(t *testing.T) {
	// A short delivery that was entirely good has a 0% reject rate, not a
	// reject rate inflated by the goods that never came.
	ranked := RankSuppliers([]SupplierDelivery{
		{SupplierID: "s", SupplierName: "S", QtyOrdered: 100, QtyAccepted: 50, QtyRejected: 0},
	})
	if ranked[0].RejectRate != 0 {
		t.Fatalf("nothing was rejected, got %v", ranked[0].RejectRate)
	}
	if ranked[0].FillRate != 50 {
		t.Fatalf("half arrived, got %v", ranked[0].FillRate)
	}

	withRejects := RankSuppliers([]SupplierDelivery{
		{SupplierID: "s", SupplierName: "S", QtyOrdered: 100, QtyAccepted: 90, QtyRejected: 10},
	})
	if withRejects[0].RejectRate != 10 {
		t.Fatalf("10 of 100 delivered is 10%%, got %v", withRejects[0].RejectRate)
	}
}

func TestValuationSaysWhereTheMoneyIs(t *testing.T) {
	report := Value([]ValuationLine{
		{ItemID: "a", QtyOnHand: 10, UnitCost: 480_000},
		{ItemID: "b", QtyOnHand: 100, UnitCost: 18_000},
		{ItemID: "c", QtyOnHand: 5, UnitCost: 4_000},
	})

	if report.TotalIDR != 6_620_000 {
		t.Fatalf("4.800.000 plus 1.800.000 plus 20.000, got %v", report.TotalIDR)
	}
	// Sorted by what it is worth, so the capital is at the top.
	if report.Lines[0].ItemID != "a" {
		t.Fatalf("the most valuable line comes first, got %q", report.Lines[0].ItemID)
	}
	// Three items is fewer than five, so all the money is in the top five.
	if report.Concentration != 100 {
		t.Fatalf("three items are all of the top five, got %v", report.Concentration)
	}
	if report.Lines[0].Share != 72.51 {
		t.Fatalf("4.800.000 of 6.620.000 is 72.51%%, got %v", report.Lines[0].Share)
	}
}
