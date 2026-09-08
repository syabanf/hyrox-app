/**
 * The four ERP modules, in TypeScript.
 *
 * The Go backend owns every rule here — stock arithmetic, approval chains,
 * loyalty maths, till settlement. What lives in this file is the shape of the
 * data and the labels the panel puts on it, so a page can be typed without
 * reimplementing anything.
 */

import type { CalendarDate } from './hris';

// ── Gudang: stock ────────────────────────────────────────────────────────────

export const ITEM_KINDS = ['RETAIL', 'SUPPLY', 'RAW'] as const;
export type ItemKind = (typeof ITEM_KINDS)[number];

export interface InventoryCategory {
  id: string;
  name: string;
  code: string;
  active: boolean;
  sortOrder: number;
  createdAt: string;
  updatedAt: string;
}

export interface InventoryItem {
  id: string;
  sku: string;
  name: string;
  description: string;
  categoryId: string | null;
  unit: string;
  kind: ItemKind;
  /** Weighted average of what the stock on hand cost — not a sale price. */
  unitCostIdr: number;
  trackStock: boolean;
  barcode: string | null;
  imageUrl: string | null;
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

/** One item at one branch. "Do we have this" is not a question until it says where. */
export interface StockLevel {
  itemId: string;
  branchId: string;
  qtyOnHand: number;
  /** Ordered from a supplier but not yet arrived; reorder maths counts it. */
  qtyOnOrder: number;
  qtyMinimum: number;
  qtyMaximum: number | null;
  binLocation: string | null;
  lastMovementAt: string | null;
  note: string | null;
}

export const MOVEMENT_KINDS = [
  'IN',
  'OUT',
  'ADJUSTMENT',
  'TRANSFER_IN',
  'TRANSFER_OUT',
  'RETURN',
] as const;
export type MovementKind = (typeof MOVEMENT_KINDS)[number];

/** One change to one quantity, kept forever. */
export interface StockMovement {
  id: string;
  itemId: string;
  branchId: string;
  kind: MovementKind;
  /** Signed: what was added to the level, negative when taken away. */
  qty: number;
  qtyBefore: number;
  qtyAfter: number;
  unitCostIdr: number;
  totalCostIdr: number;
  referenceType: string | null;
  referenceId: string | null;
  referenceNumber: string | null;
  reason: string | null;
  note: string | null;
  actorId: string | null;
  actorName: string | null;
  createdAt: string;
}

export const STOCK_TAKE_STATUSES = ['DRAFT', 'APPLIED', 'CANCELLED'] as const;
export type StockTakeStatus = (typeof STOCK_TAKE_STATUSES)[number];

export interface StockTake {
  id: string;
  takeNumber: string;
  branchId: string;
  status: StockTakeStatus;
  countedOn: CalendarDate;
  note: string | null;
  appliedBy: string | null;
  appliedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface StockTakeLine {
  id: string;
  stockTakeId: string;
  itemId: string;
  /** Frozen when the item was counted, not whatever the system says now. */
  qtyExpected: number;
  qtyCounted: number;
  note: string | null;
}

export interface StockTransfer {
  id: string;
  transferNumber: string;
  itemId: string;
  fromBranchId: string;
  toBranchId: string;
  qty: number;
  note: string | null;
  actorId: string | null;
  actorName: string | null;
  createdAt: string;
}

// ── Purchasing ───────────────────────────────────────────────────────────────

export const SUPPLIER_STATUSES = ['DRAFT', 'ACTIVE', 'PROBATION', 'INACTIVE', 'BLOCKED'] as const;
export type SupplierStatus = (typeof SUPPLIER_STATUSES)[number];

export const PAYMENT_TERMS = ['COD', 'NET7', 'NET14', 'NET30', 'NET45', 'NET60'] as const;
export type PaymentTerms = (typeof PAYMENT_TERMS)[number];

export interface Supplier {
  id: string;
  code: string;
  name: string;
  contactName: string | null;
  contactPhone: string | null;
  email: string | null;
  address: string | null;
  city: string | null;
  taxNumber: string | null;
  paymentTerms: PaymentTerms;
  bankName: string | null;
  bankAccount: string | null;
  bankHolder: string | null;
  category: string | null;
  status: SupplierStatus;
  note: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface SupplierPrice {
  id: string;
  supplierId: string;
  itemId: string;
  unitPriceIdr: number;
  minOrderQty: number;
  leadTimeDays: number;
  effectiveFrom: CalendarDate;
  active: boolean;
  createdAt: string;
}

export const APPROVAL_LEVELS = ['HEAD', 'FINANCE', 'DIRECTOR'] as const;
export type ApprovalLevel = (typeof APPROVAL_LEVELS)[number];

export const PR_STATUSES = [
  'DRAFT',
  'PENDING_HEAD',
  'PENDING_FINANCE',
  'PENDING_DIRECTOR',
  'APPROVED',
  'REJECTED',
  'CONVERTED',
] as const;
export type PurchaseRequestStatus = (typeof PR_STATUSES)[number];

export interface PurchaseRequest {
  id: string;
  prNumber: string;
  branchId: string;
  requesterId: string | null;
  requesterName: string;
  departmentId: string | null;
  status: PurchaseRequestStatus;
  priority: string;
  totalIdr: number;
  requiredOn: CalendarDate | null;
  note: string | null;
  approvedByHead: string | null;
  approvedAtHead: string | null;
  approvedByFinance: string | null;
  approvedAtFinance: string | null;
  approvedByDirector: string | null;
  approvedAtDirector: string | null;
  rejectedBy: string | null;
  rejectedAt: string | null;
  rejectionReason: string | null;
  convertedPoId: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface PurchaseRequestItem {
  id: string;
  requestId: string;
  itemId: string | null;
  description: string;
  qty: number;
  unit: string;
  estimatedPriceIdr: number;
  totalIdr: number;
  note: string | null;
}

export const PO_STATUSES = [
  'DRAFT',
  'APPROVED',
  'SENT',
  'PARTIALLY_RECEIVED',
  'RECEIVED',
  'CANCELLED',
] as const;
export type PurchaseOrderStatus = (typeof PO_STATUSES)[number];

export interface PurchaseOrder {
  id: string;
  poNumber: string;
  supplierId: string;
  branchId: string;
  requestId: string | null;
  status: PurchaseOrderStatus;
  orderedOn: CalendarDate;
  expectedOn: CalendarDate | null;
  subtotalIdr: number;
  discountIdr: number;
  taxPercent: number;
  taxIdr: number;
  totalIdr: number;
  terms: string | null;
  shipTo: string | null;
  note: string | null;
  approvedBy: string | null;
  approvedAt: string | null;
  sentAt: string | null;
  cancelledBy: string | null;
  cancelledAt: string | null;
  cancellationReason: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface PurchaseOrderItem {
  id: string;
  orderId: string;
  itemId: string;
  description: string;
  /** In the pack the buyer ordered — ten cartons, not 240 pieces. */
  qtyOrdered: number;
  qtyReceived: number;
  unit: string;
  /** Base units inside one of them, so the receipt can convert once. */
  packFactor: number;
  qtyOrderedBase: number;
  qtyReceivedBase: number;
  unitPriceIdr: number;
  discountIdr: number;
  subtotalIdr: number;
  note: string | null;
}

export const GRN_STATUSES = ['DRAFT', 'POSTED', 'CANCELLED'] as const;
export type GoodsReceiptStatus = (typeof GRN_STATUSES)[number];

export interface GoodsReceipt {
  id: string;
  grnNumber: string;
  orderId: string;
  supplierId: string;
  branchId: string;
  receivedOn: CalendarDate;
  receivedBy: string | null;
  receivedByName: string | null;
  deliveryNoteNumber: string | null;
  status: GoodsReceiptStatus;
  note: string | null;
  postedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export const QC_STATUSES = ['ACCEPTED', 'PARTIALLY_REJECTED', 'REJECTED'] as const;
export type QCStatus = (typeof QC_STATUSES)[number];

export interface GoodsReceiptItem {
  id: string;
  receiptId: string;
  orderItemId: string;
  itemId: string;
  /** Entered stock. */
  qtyAccepted: number;
  /** Failed inspection: recorded, and never entered stock. */
  qtyRejected: number;
  qtyReturned: number;
  unitPriceIdr: number;
  qcStatus: QCStatus;
  batchNumber: string | null;
  expiresOn: CalendarDate | null;
  note: string | null;
}

export const RETURN_REASONS = [
  'DAMAGED',
  'WRONG_ITEM',
  'EXPIRED',
  'OVERSTOCK',
  'SPEC_MISMATCH',
  'OTHER',
] as const;
export type ReturnReason = (typeof RETURN_REASONS)[number];

export interface PurchaseReturn {
  id: string;
  returnNumber: string;
  receiptId: string;
  supplierId: string;
  branchId: string;
  returnedOn: CalendarDate;
  reasonType: ReturnReason;
  reasonNote: string | null;
  status: GoodsReceiptStatus;
  totalIdr: number;
  postedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface PurchaseReturnItem {
  id: string;
  returnId: string;
  receiptItemId: string;
  itemId: string;
  qty: number;
  unitPriceIdr: number;
  note: string | null;
}

// ── CRM: loyalty ─────────────────────────────────────────────────────────────

export interface LoyaltyTier {
  id: string;
  code: string;
  name: string;
  rank: number;
  minLifetimeXp: number;
  minSpendIdr: number;
  xpMultiplier: number;
  discountPercent: number;
  benefits: string[];
  colour: string;
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface LoyaltyProfile {
  id: string;
  memberId: string;
  memberCode: string;
  tierCode: string;
  /** Spendable, and falls when it is spent. */
  currentXp: number;
  /** Only ever grows, and is what decides the tier. */
  lifetimeXp: number;
  spentXp: number;
  lifetimeSpendIdr: number;
  joinedAt: string;
  lastActivityAt: string | null;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export const XP_CHANNELS = ['POS', 'BOOKING', 'CLASS', 'PAYMENT', 'MANUAL', 'CAMPAIGN'] as const;
export type XPChannel = (typeof XP_CHANNELS)[number];

export const XP_MODES = ['FIXED', 'PER_ITEM', 'PER_AMOUNT', 'MULTIPLIER', 'PERCENTAGE'] as const;
export type XPMode = (typeof XP_MODES)[number];

export interface XPRule {
  id: string;
  code: string;
  name: string;
  sourceChannel: XPChannel;
  sourceType: string;
  sourceId: string | null;
  branchId: string | null;
  xpMode: XPMode;
  xpValue: number;
  amountStep: number;
  minAmount: number;
  maxXpPerEvent: number | null;
  tierMultiplierEnabled: boolean;
  priority: number;
  startsAt: string | null;
  endsAt: string | null;
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

export const XP_DIRECTIONS = ['EARN', 'SPEND', 'ADJUST', 'REVERSE', 'EXPIRE'] as const;
export type XPDirection = (typeof XP_DIRECTIONS)[number];

export interface XPEntry {
  id: string;
  memberId: string;
  direction: XPDirection;
  sourceChannel: XPChannel;
  sourceType: string;
  sourceId: string | null;
  branchId: string | null;
  xpDelta: number;
  balanceBefore: number;
  balanceAfter: number;
  lifetimeBefore: number;
  lifetimeAfter: number;
  ruleId: string | null;
  referenceType: string | null;
  referenceId: string | null;
  /** What makes the same event arriving twice a no-op. */
  idempotencyKey: string | null;
  description: string | null;
  createdAt: string;
}

export const REWARD_TYPES = [
  'DISCOUNT',
  'MERCHANDISE',
  'VOUCHER',
  'CREDITS',
  'CLASS',
  'CUSTOM',
] as const;
export type RewardType = (typeof REWARD_TYPES)[number];

export interface Reward {
  id: string;
  code: string;
  name: string;
  description: string;
  rewardType: RewardType;
  xpCost: number;
  requiredTierCode: string | null;
  rewardValue: Record<string, unknown>;
  stockTotal: number | null;
  stockRedeemed: number;
  maxPerMember: number | null;
  imageUrl: string | null;
  startsAt: string | null;
  endsAt: string | null;
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

export const REDEMPTION_STATUSES = [
  'PENDING',
  'APPROVED',
  'FULFILLED',
  'CANCELLED',
  'EXPIRED',
] as const;
export type RedemptionStatus = (typeof REDEMPTION_STATUSES)[number];

export interface Redemption {
  id: string;
  redemptionNumber: string;
  memberId: string;
  rewardId: string;
  xpCost: number;
  xpLedgerId: string | null;
  status: RedemptionStatus;
  voucherCode: string | null;
  requestedAt: string;
  approvedAt: string | null;
  fulfilledAt: string | null;
  cancelledAt: string | null;
  expiresAt: string | null;
  note: string | null;
  createdAt: string;
  updatedAt: string;
}

// ── POS: the till ────────────────────────────────────────────────────────────

export interface POSCategory {
  id: string;
  name: string;
  sortOrder: number;
  active: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface POSProduct {
  id: string;
  sku: string;
  name: string;
  description: string;
  categoryId: string | null;
  /** Null means selling it moves no stock, which is correct for a service. */
  inventoryItemId: string | null;
  priceIdr: number;
  /** Per base unit, so a carton product carries the piece cost. */
  costIdr: number;
  taxPercent: number;
  /** What the scanner reads. A single and a carton carry different codes. */
  barcode: string | null;
  /** How much stock one sold unit takes off the shelf: a carton of 24 is 24. */
  packUnit: string;
  packFactor: number;
  bonusXp: number;
  imageUrl: string | null;
  active: boolean;
  available: boolean;
  createdAt: string;
  updatedAt: string;
}

export const POS_ORDER_STATUSES = ['OPEN', 'COMPLETED', 'CANCELLED', 'VOIDED'] as const;
export type POSOrderStatus = (typeof POS_ORDER_STATUSES)[number];

export const POS_PAYMENT_STATUSES = ['UNPAID', 'PARTIAL', 'PAID', 'REFUNDED'] as const;
export type POSPaymentStatus = (typeof POS_PAYMENT_STATUSES)[number];

export const POS_PAYMENT_METHODS = [
  'CASH',
  'QRIS',
  'DEBIT',
  'CREDIT',
  'TRANSFER',
  'MEMBER_CREDIT',
] as const;
export type POSPaymentMethod = (typeof POS_PAYMENT_METHODS)[number];

export interface POSOrder {
  id: string;
  orderNumber: string;
  branchId: string;
  shiftId: string | null;
  cashierId: string;
  cashierName: string;
  memberId: string | null;
  /** What the customer is buying as. It decides which price list applies. */
  channel: SalesChannel;
  status: POSOrderStatus;
  paymentStatus: POSPaymentStatus;
  subtotalIdr: number;
  /** What the cashier took off by hand. */
  discountIdr: number;
  /** What the member's standing took off. Recomputed on every change. */
  tierDiscountIdr: number;
  discountReason: string | null;
  taxIdr: number;
  totalIdr: number;
  paidIdr: number;
  changeIdr: number;
  costIdr: number;
  grossProfitIdr: number;
  xpEarned: number;
  note: string | null;
  openedAt: string;
  completedAt: string | null;
  cancelledAt: string | null;
  voidedAt: string | null;
  voidedBy: string | null;
  voidReason: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface POSOrderItem {
  id: string;
  orderId: string;
  productId: string;
  /** Frozen: a receipt has to still be true after a rename and a reprice. */
  productName: string;
  productSku: string;
  inventoryItemId: string | null;
  qty: number;
  /** Frozen: "2" has to still mean two six-packs after a repack. */
  packUnit: string;
  packFactor: number;
  /** What this line takes off the shelf, in the item's base unit. */
  qtyBase: number;
  unitPriceIdr: number;
  discountIdr: number;
  taxPercent: number;
  lineTotalIdr: number;
  unitCostIdr: number;
  note: string | null;
  createdAt: string;
}

export interface POSPayment {
  id: string;
  orderId: string;
  method: POSPaymentMethod;
  amountIdr: number;
  changeIdr: number;
  reference: string | null;
  cashierId: string;
  takenAt: string;
}

export const SHIFT_STATUSES = ['OPEN', 'CLOSED'] as const;
export type ShiftStatus = (typeof SHIFT_STATUSES)[number];

export interface CashierShift {
  id: string;
  shiftNumber: string;
  cashierId: string;
  cashierName: string;
  branchId: string;
  status: ShiftStatus;
  openedAt: string;
  closedAt: string | null;
  openingCashIdr: number;
  closingCashIdr: number | null;
  /** Opening plus every cash tender, minus the change handed back. */
  expectedCashIdr: number;
  varianceIdr: number;
  note: string | null;
  createdAt: string;
  updatedAt: string;
}

// ── Labels the panel shares ──────────────────────────────────────────────────

/** Movement kinds read better as sentences than as constants. */
export const MOVEMENT_LABELS: Record<MovementKind, string> = {
  IN: 'Received',
  OUT: 'Issued',
  ADJUSTMENT: 'Adjusted',
  TRANSFER_IN: 'Transferred in',
  TRANSFER_OUT: 'Transferred out',
  RETURN: 'Returned',
};

/** Which approval level a role can sign at, mirrored from the backend so the
 *  panel can grey out a button the server would refuse anyway. */
export const APPROVAL_ROLES: Record<ApprovalLevel, readonly string[]> = {
  HEAD: ['BRANCH_MANAGER', 'HQ_ADMIN', 'SUPER_ADMIN'],
  FINANCE: ['FINANCE', 'HQ_ADMIN', 'SUPER_ADMIN'],
  DIRECTOR: ['HQ_ADMIN', 'SUPER_ADMIN'],
};

// ── Packs: the same goods bought by the carton and sold by the piece ─────────

export const UNIT_KINDS = ['COUNT', 'MEASURE'] as const;
/** COUNT units are whole things; MEASURE units are continuous. */
export type UnitKind = (typeof UNIT_KINDS)[number];

/** A word for an amount: piece, carton, kilogram. */
export interface Unit {
  id: string;
  code: string;
  name: string;
  kind: UnitKind;
  active: boolean;
  sortOrder: number;
}

/**
 * One way of handing an item over.
 *
 * Stock is always counted in the base pack's unit; every other pack says how
 * many of those it holds. The barcode lives here rather than on the item
 * because that is the point — a single and a carton scan differently.
 */
export interface ItemPack {
  id: string;
  itemId: string;
  unitCode: string;
  /** Base units inside one of these. The base pack's is always 1. */
  factor: number;
  barcode: string | null;
  isBase: boolean;
  purchaseDefault: boolean;
  saleDefault: boolean;
  active: boolean;
}

export const SALES_CHANNELS = ['RETAIL', 'WHOLESALE', 'STAFF'] as const;
/** What a customer is buying as. The only thing that moves a price. */
export type SalesChannel = (typeof SALES_CHANNELS)[number];

export const CHANNEL_LABELS: Record<SalesChannel, string> = {
  RETAIL: 'Retail',
  WHOLESALE: 'Wholesale',
  STAFF: 'Staff',
};

/** One price break: this channel, at this quantity and above, pays this. */
export interface ProductPrice {
  id: string;
  productId: string;
  channel: SalesChannel;
  minQty: number;
  priceIdr: number;
  active: boolean;
}

/** Pack quantity to base units. Ten cartons of 24 is 240 pieces. */
export function packToBase(packQty: number, factor: number): number {
  return round3(packQty * (factor > 0 ? factor : 1));
}

/**
 * Base units to pack quantity, deliberately unrounded: three and a half
 * cartons on hand is a true and useful thing to say.
 */
export function baseToPack(baseQty: number, factor: number): number {
  return round3(baseQty / (factor > 0 ? factor : 1));
}

/** A price quoted per pack, as a price per base unit. */
export function packPriceToBase(packPriceIdr: number, factor: number): number {
  if (factor <= 0) return packPriceIdr;
  return Math.round((packPriceIdr / factor) * 100) / 100;
}

/** "CTN (24 PCS)", or just "PCS" when there is nothing to convert. */
export function packLabel(pack: Pick<ItemPack, 'unitCode' | 'factor' | 'isBase'>, baseUnit: string): string {
  if (pack.isBase || pack.factor === 1) return pack.unitCode;
  return `${pack.unitCode} (${round3(pack.factor)} ${baseUnit})`;
}

function round3(value: number): number {
  return Math.round(value * 1000) / 1000;
}
