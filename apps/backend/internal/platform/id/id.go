// Package id generates the prefixed, lexicographically sortable identifiers
// used as primary keys across every module.
//
// IDs are TEXT rather than UUID on purpose: the prefix says what a row is when
// it turns up in a log or a URL (mem_..., ses_..., pay_...), and seeded demo
// rows can keep hand-written ids like "mem_demo".
package id

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
	"sync"
	"time"
)

// Crockford base32 without the ambiguous letters I, L, O, U.
const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Prefixes are the canonical short names for each entity type.
const (
	Member       = "mem"
	AdminUser    = "adm"
	Branch       = "brn"
	Gate         = "gat"
	Coach        = "coa"
	ClassType    = "clt"
	Session      = "ses"
	Booking      = "bkg"
	LedgerEntry  = "led"
	Lot          = "lot"
	Payment      = "pay"
	Package      = "pkg"
	Voucher      = "vou"
	Redemption   = "red"
	AccessLog    = "acc"
	Notification = "ntf"
	Campaign     = "cmp"
	Audit        = "aud"
	Scheme       = "sch"
	Payout       = "pyo"
	Activity     = "act"
	Comment      = "cmt"
	Segment      = "seg"
	Effort       = "eff"
	Challenge    = "chl"
	Club         = "clb"
	Gear         = "gea"
	Route        = "rte"
	Exercise     = "exr"
	Workout      = "wkt"
	WorkoutRun   = "wrn"
	RaceEvent    = "rce"
	UserRace     = "urc"
	OTP          = "otp"
	Outbox       = "obx"

	// People on the payroll, as opposed to members and logins.
	Employee         = "emp"
	Department       = "dep"
	Position         = "pos"
	EmploymentStatus = "est"
	Shift            = "shf"
	EmployeeShift    = "esh"
	Attendance       = "att"
	Holiday          = "hol"
	Leave            = "lve"
	Overtime         = "ovt"

	// Stock, and everything that moves it.
	InventoryItem     = "itm"
	InventoryCategory = "ictg"
	StockMovement     = "stm"
	StockTransfer     = "strf"
	StockTake         = "stk"
	StockTakeLine     = "stl"

	// Buying.
	Supplier        = "sup"
	PurchaseRequest = "pr"
	PurchaseOrder   = "po"
	GoodsReceipt    = "grn"
	PurchaseReturn  = "prt"
	LineItem        = "lin"

	// The till.
	POSProduct   = "prd"
	POSCategory  = "pctg"
	POSOrder     = "ord"
	POSPayment   = "pmt"
	POSShift     = "psh"
	ProductPrice = "ppr"

	// Packs: how the same goods are bought by the carton and sold by the piece.
	Unit     = "unt"
	ItemPack = "ipk"

	// Loyalty.
	Tier              = "tir"
	MemberProfile     = "mpr"
	XPRule            = "xpr"
	XPEntry           = "xpe"
	Reward            = "rwd"
	LoyaltyRedemption = "rdm"
)

// Generator hands out identifiers. It is safe for concurrent use.
type Generator interface {
	New(prefix string) string
}

type generator struct {
	mu       sync.Mutex
	lastMs   int64
	sequence uint16
}

// NewGenerator returns the default time-ordered generator.
func NewGenerator() Generator { return &generator{} }

// New returns "<prefix>_<26 chars>": 48 bits of millisecond timestamp, 16 bits
// of per-millisecond sequence, and 64 bits of randomness. Sorting by id sorts
// by creation time, which keeps index locality good on append-heavy tables.
func (g *generator) New(prefix string) string {
	now := time.Now().UnixMilli()

	g.mu.Lock()
	if now == g.lastMs {
		g.sequence++
	} else {
		g.lastMs = now
		g.sequence = 0
	}
	seq := g.sequence
	g.mu.Unlock()

	var raw [16]byte
	// 48-bit big-endian millisecond timestamp, so byte order == time order.
	raw[0] = byte(now >> 40)
	raw[1] = byte(now >> 32)
	raw[2] = byte(now >> 24)
	raw[3] = byte(now >> 16)
	raw[4] = byte(now >> 8)
	raw[5] = byte(now)
	binary.BigEndian.PutUint16(raw[6:8], seq)
	if _, err := rand.Read(raw[8:]); err != nil {
		// crypto/rand cannot fail on any supported platform; if it ever does,
		// the timestamp and sequence still keep ids unique within a process.
		binary.BigEndian.PutUint64(raw[8:], uint64(now)*2654435761)
	}

	var sb strings.Builder
	sb.Grow(len(prefix) + 1 + 26)
	sb.WriteString(prefix)
	sb.WriteByte('_')
	encode(&sb, raw[:])
	return sb.String()
}

// encode writes 16 bytes as 26 base32 characters (130 bits of space, top bits zero).
func encode(sb *strings.Builder, b []byte) {
	var acc uint32
	var bits uint
	for _, c := range b {
		acc = acc<<8 | uint32(c)
		bits += 8
		for bits >= 5 {
			bits -= 5
			sb.WriteByte(alphabet[(acc>>bits)&31])
		}
	}
	if bits > 0 {
		sb.WriteByte(alphabet[(acc<<(5-bits))&31])
	}
}

// Fixed is a deterministic generator for tests and seeding: it returns
// "<prefix>_<n>" with a per-prefix counter.
type Fixed struct {
	mu       sync.Mutex
	counters map[string]int
}

func NewFixed() *Fixed { return &Fixed{counters: map[string]int{}} }

func (f *Fixed) New(prefix string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counters[prefix]++
	return prefix + "_" + itoa(f.counters[prefix])
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
