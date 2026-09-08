package pos

import (
	"fmt"
	"strings"
	"time"

	"github.com/syabanf/hyrox-app/apps/backend/internal/domain"
)

// Rendering a receipt.
//
// Plain text at a fixed width, because that is what a thermal printer takes:
// no proportional font, no wrapping it can do for you, and a line longer than
// the paper is a line that comes out cut in half. The width comes from the
// branch's settings, since 58mm and 80mm are the two that exist in practice
// and they hold very different numbers of characters.

const (
	narrowWidth = 32 // 58mm paper
	wideWidth   = 48 // 80mm paper
)

func columnsFor(paperWidth int) int {
	if paperWidth >= 80 {
		return wideWidth
	}
	return narrowWidth
}

// renderReceipt lays a sale out for the printer.
func renderReceipt(order OrderView, settings ReceiptSettings,
	promotions []domain.AppliedPromotion, studio *time.Location) string {

	width := columnsFor(settings.PaperWidth)
	var b strings.Builder

	centre := func(text string) {
		for _, part := range wrap(text, width) {
			padding := (width - len([]rune(part))) / 2
			if padding < 0 {
				padding = 0
			}
			b.WriteString(strings.Repeat(" ", padding) + part + "\n")
		}
	}
	rule := func() { b.WriteString(strings.Repeat("-", width) + "\n") }
	// A label on the left and a figure on the right, with the gap between
	// them. The figure never wraps: a total split across two lines is a total
	// nobody can read.
	row := func(label, value string) {
		gap := width - len([]rune(label)) - len([]rune(value))
		if gap < 1 {
			label = trimTo(label, width-len([]rune(value))-1)
			gap = 1
		}
		b.WriteString(label + strings.Repeat(" ", gap) + value + "\n")
	}

	if settings.BusinessName != "" {
		centre(strings.ToUpper(settings.BusinessName))
	}
	if settings.Address != "" {
		centre(settings.Address)
	}
	if settings.Phone != nil && *settings.Phone != "" {
		centre(*settings.Phone)
	}
	if settings.TaxNumber != nil && *settings.TaxNumber != "" {
		centre("NPWP " + *settings.TaxNumber)
	}
	if settings.Header != "" {
		centre(settings.Header)
	}
	rule()

	row(order.OrderNumber, order.OpenedAt.In(studio).Format("02/01/06 15:04"))
	if settings.ShowCashier {
		row("Cashier", trimTo(order.CashierName, width-10))
	}
	if order.MemberName != nil && *order.MemberName != "" {
		row("Member", trimTo(*order.MemberName, width-9))
	}
	rule()

	for _, item := range order.Items {
		b.WriteString(trimTo(item.ProductName, width) + "\n")
		// The quantity line reads as the arithmetic it is: how many, at what,
		// coming to what.
		quantity := fmt.Sprintf("  %s %s x %s",
			trimFloat(float64(item.Qty)), strings.ToLower(item.PackUnit),
			money(item.UnitPriceIDR))
		row(quantity, money(item.LineTotalIDR))
	}
	rule()

	row("Subtotal", money(order.SubtotalIDR))
	for _, promotion := range promotions {
		row(trimTo(promotion.Name, width-14), "-"+money(promotion.DiscountIDR))
	}
	if order.TierDiscountIDR > 0 {
		row("Member discount", "-"+money(order.TierDiscountIDR))
	}
	if order.DiscountIDR > 0 {
		row("Discount", "-"+money(order.DiscountIDR))
	}
	if order.TaxIDR > 0 {
		row("Tax", money(order.TaxIDR))
	}
	rule()
	row("TOTAL", money(order.TotalIDR))

	for _, payment := range order.Payments {
		row(string(payment.Method), money(payment.AmountIDR))
	}
	if order.ChangeIDR > 0 {
		row("Change", money(order.ChangeIDR))
	}

	if order.XPEarned > 0 {
		rule()
		centre(fmt.Sprintf("%d points earned", order.XPEarned))
	}
	if settings.Footer != "" {
		rule()
		centre(settings.Footer)
	}
	// Thermal printers need blank lines to clear the tear bar.
	b.WriteString("\n\n\n")
	return b.String()
}

// money is a rupiah figure without the symbol: the currency is on the header
// and repeating it on every line wastes a third of narrow paper.
func money(v float64) string {
	whole := int64(v + 0.5)
	sign := ""
	if whole < 0 {
		sign, whole = "-", -whole
	}
	digits := fmt.Sprintf("%d", whole)

	var out []byte
	for i, c := range []byte(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, c)
	}
	return sign + string(out)
}

func trimFloat(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", v), "0"), ".")
}

func trimTo(text string, width int) string {
	runes := []rune(text)
	if width <= 0 {
		return ""
	}
	if len(runes) <= width {
		return text
	}
	return string(runes[:width])
}

// wrap breaks text on word boundaries, so a long shop name does not come out
// cut mid-word.
func wrap(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	lines := []string{}
	current := ""
	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if len([]rune(candidate)) > width && current != "" {
			lines = append(lines, current)
			current = word
			continue
		}
		current = candidate
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}
