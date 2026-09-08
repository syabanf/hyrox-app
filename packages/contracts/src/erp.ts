import type {
  ApprovalLevel,
  CashierShift,
  GoodsReceipt,
  GoodsReceiptItem,
  Batch,
  ExpiryState,
  ExpirySummary,
  InventoryItem,
  ItemPack,
  LoyaltyProfile,
  LoyaltyTier,
  POSOrder,
  POSOrderItem,
  POSPayment,
  ProductPrice,
  PurchaseOrder,
  PurchaseOrderItem,
  PurchaseRequest,
  PurchaseRequestItem,
  PurchaseReturn,
  POSProduct,
  PurchaseReturnItem,
  Redemption,
  StockLevel,
  StockMovement,
  StockTake,
  StockTakeLine,
  XPEntry,
  XPRule,
} from '@nuhabit/domain';

// ── Gudang ───────────────────────────────────────────────────────────────────

export interface StockLevelView extends StockLevel {
  branchName: string;
  lowStock: boolean;
  reorderQty: number;
  valueIdr: number;
}

export interface InventoryItemView extends InventoryItem {
  categoryName: string | null;
  totalOnHand: number;
}

/** One shelf, ready to render. */
export interface StockRowView {
  item: InventoryItem;
  level: StockLevelView;
}

export interface InventoryItemDetailView {
  item: InventoryItemView;
  levels: StockLevelView[];
  movements: StockMovement[];
}

export interface StockValuationView {
  items: number;
  units: number;
  valueIdr: number;
  lowStock: number;
  outOfStock: number;
}

export interface InventoryOverviewView {
  valuation: StockValuationView;
  lowStock: StockRowView[];
  recent: StockMovement[];
}

export interface StockTakeLineView extends StockTakeLine {
  itemName: string;
  sku: string;
  unit: string;
  variance: number;
  valueIdr: number;
}

export interface StockTakeSummaryView {
  lines: number;
  linesVaried: number;
  qtyOver: number;
  qtyShort: number;
  valueVarianceIdr: number;
}

export interface StockTakeView extends StockTake {
  branchName: string;
  lines: StockTakeLineView[];
  summary: StockTakeSummaryView;
}

// ── Purchasing ───────────────────────────────────────────────────────────────

/** One signature on the approval chain. */
export interface ApprovalStepView {
  level: ApprovalLevel;
  signed: boolean;
  signedBy: string | null;
  signedAt: string | null;
}

export interface PurchaseRequestView extends PurchaseRequest {
  branchName: string;
  items: PurchaseRequestItem[];
  /** Every signature the amount requires, in order. */
  chain: ApprovalStepView[];
  /** What it is waiting on now; empty when fully signed. */
  awaitingLevel: ApprovalLevel | '';
}

export interface PurchaseOrderLineView extends PurchaseOrderItem {
  itemName: string;
  qtyOutstanding: number;
}

export interface PurchaseOrderView extends PurchaseOrder {
  supplierName: string;
  branchName: string;
  items: PurchaseOrderLineView[];
  receipts?: GoodsReceipt[];
}

export interface GoodsReceiptLineView extends GoodsReceiptItem {
  itemName: string;
  qtyOrdered: number;
  qtyReturnable: number;
}

export interface GoodsReceiptView extends GoodsReceipt {
  supplierName: string;
  branchName: string;
  poNumber: string;
  items: GoodsReceiptLineView[];
}

export interface PurchaseReturnLineView extends PurchaseReturnItem {
  itemName: string;
}

export interface PurchaseReturnView extends PurchaseReturn {
  supplierName: string;
  grnNumber: string;
  items: PurchaseReturnLineView[];
}

export interface PurchasingSummaryView {
  pendingRequests: number;
  openOrders: number;
  awaitingDelivery: number;
  suppliers: number;
  committedIdr: number;
  receivedMonthIdr: number;
}

// ── CRM ──────────────────────────────────────────────────────────────────────

export interface LoyaltyProfileView extends LoyaltyProfile {
  memberName: string;
  tier: LoyaltyTier | null;
  /** What they are climbing towards, and how far off it is. */
  nextTier: LoyaltyTier | null;
  xpToNext: number;
}

export interface RedemptionView extends Redemption {
  memberName: string;
  rewardName: string;
  rewardType: string;
}

export interface LoyaltyMemberDetailView {
  profile: LoyaltyProfileView;
  ledger: XPEntry[];
  redemptions: RedemptionView[];
}

export interface LoyaltySummaryView {
  members: number;
  outstandingXp: number;
  lifetimeXp: number;
  pendingClaims: number;
  earnedThisMonth: number;
  byTier: Record<string, number>;
}

export interface XPAwardView {
  entry: XPEntry | null;
  profile: LoyaltyProfile;
  awarded: boolean;
  duplicate: boolean;
  rule: XPRule | null;
  /** Names the band the member has just moved into. */
  tierChanged: LoyaltyTier | null;
}

// ── POS ──────────────────────────────────────────────────────────────────────

export interface POSProductView extends POSProduct {
  categoryName: string | null;
  /**
   * In the product's own pack, not in base units: a carton row that says "5"
   * when there are five bottles left has told the cashier the opposite of the
   * truth. Null for a service, which has no stock rather than none left.
   */
  onHand: number | null;
}

/** What a barcode resolved to, with every price that could apply to it. */
export interface ScanView extends POSProductView {
  prices: ProductPrice[];
}

/**
 * A batch with the question a shop actually asks answered: how long has this
 * got, and what is it worth if the answer is "none".
 */
export interface BatchView extends Batch {
  itemName: string;
  itemSku: string;
  branchName: string;
  unit: string;
  state: ExpiryState;
  /** Negative once past the date; null when there is no date at all. */
  daysLeft: number | null;
  valueIdr: number;
}

export interface ExpiryReportView {
  summary: ExpirySummary;
  /** Still actionable, in date order. */
  near: BatchView[];
  expired: BatchView[];
}

/** A pack with its arithmetic already done, so a form need not repeat it. */
export interface ItemPackView extends ItemPack {
  /** Reads the way a shelf edge does: "CTN (24 PCS)". */
  label: string;
  /** The branch's stock expressed in this pack. */
  onHandPacks?: number;
}

export interface POSOrderView extends POSOrder {
  memberName: string | null;
  items: POSOrderItem[];
  payments: POSPayment[];
  /** What is still owed, which is what the till shows next. */
  dueIdr: number;
}

export interface ShiftTotalsView {
  orders: number;
  salesIdr: number;
  cashIdr: number;
  qrisIdr: number;
  cardIdr: number;
  transferIdr: number;
  memberCreditIdr: number;
  voidedIdr: number;
  expectedCashIdr: number;
}

export interface CashierShiftView extends CashierShift {
  totals: ShiftTotalsView;
}

export interface POSSalesSummaryView {
  orders: number;
  salesIdr: number;
  costIdr: number;
  grossProfitIdr: number;
  discountIdr: number;
  voidedOrders: number;
  openOrders: number;
  xpAwarded: number;
}

export interface TopProductView {
  productId: string;
  productName: string;
  qty: number;
  salesIdr: number;
}

export interface POSOverviewView {
  today: POSSalesSummaryView;
  month: POSSalesSummaryView;
  topProducts: TopProductView[];
  openShifts: CashierShift[];
}

// ── Inputs ───────────────────────────────────────────────────────────────────

export interface UpsertInventoryItemInput {
  sku: string;
  name: string;
  description?: string;
  categoryId?: string | null;
  unit?: string;
  kind?: string;
  trackStock?: boolean;
  /** Dated goods. Turning it on makes a batch required on every receipt. */
  trackBatches?: boolean;
  expiryWarningDays?: number;
  barcode?: string | null;
  imageUrl?: string | null;
  active?: boolean;
}

export interface AdjustStockInput {
  itemId: string;
  branchId: string;
  /** Signed: negative takes stock away. Always carries a reason. */
  qty: number;
  reason: string;
  /**
   * Required when dated stock is being added: a surplus found at stocktake is
   * a physical pile with a date on it, and nothing can guess which. Stock
   * leaving names no batch — FEFO decides, the same as for a sale.
   */
  batchCode?: string;
  expiresOn?: string | null;
  note?: string | null;
}

export interface TransferStockInput {
  itemId: string;
  fromBranchId: string;
  toBranchId: string;
  qty: number;
  note?: string | null;
}

export interface ReorderPointInput {
  branchId: string;
  qtyMinimum: number;
  qtyMaximum?: number | null;
  binLocation?: string | null;
  note?: string | null;
}

export interface UpsertSupplierInput {
  code: string;
  name: string;
  contactName?: string | null;
  contactPhone?: string | null;
  email?: string | null;
  address?: string | null;
  city?: string | null;
  taxNumber?: string | null;
  paymentTerms?: string;
  bankName?: string | null;
  bankAccount?: string | null;
  bankHolder?: string | null;
  category?: string | null;
  status?: string;
  note?: string | null;
}

export interface CreatePurchaseRequestInput {
  branchId: string;
  requesterName?: string;
  departmentId?: string | null;
  priority?: string;
  requiredOn?: string | null;
  note?: string | null;
}

export interface PurchaseRequestLineInput {
  itemId?: string | null;
  description: string;
  qty: number;
  unit?: string;
  estimatedPriceIdr: number;
  note?: string | null;
}

export interface CreatePurchaseOrderInput {
  supplierId: string;
  branchId: string;
  requestId?: string | null;
  expectedOn?: string | null;
  taxPercent?: number;
  discountIdr?: number;
  terms?: string | null;
  shipTo?: string | null;
  note?: string | null;
}

export interface PurchaseOrderLineInput {
  itemId: string;
  description?: string;
  /** In the pack being ordered: ten cartons, not 240 pieces. */
  qty: number;
  /**
   * The pack to order in. Omitted means whatever this item is normally bought
   * by; a unit the item has no pack for is refused rather than guessed at.
   */
  unit?: string;
  unitPriceIdr: number;
  discountIdr?: number;
  note?: string | null;
}

export interface ReceiveLineInput {
  orderItemId: string;
  qtyAccepted: number;
  qtyRejected?: number;
  batchNumber?: string | null;
  expiresOn?: string | null;
  note?: string | null;
}

export interface AdjustXPInput {
  xpDelta: number;
  reason: string;
}

export interface UpsertTierInput {
  code: string;
  name: string;
  rank: number;
  minLifetimeXp: number;
  minSpendIdr: number;
  xpMultiplier: number;
  discountPercent: number;
  benefits?: string[];
  colour?: string;
  active?: boolean;
}

export interface UpsertRewardInput {
  code: string;
  name: string;
  description?: string;
  rewardType: string;
  xpCost: number;
  requiredTierCode?: string | null;
  rewardValue?: Record<string, unknown>;
  stockTotal?: number | null;
  maxPerMember?: number | null;
  imageUrl?: string | null;
  active?: boolean;
}

export interface UpsertPOSProductInput {
  sku: string;
  name: string;
  description?: string;
  categoryId?: string | null;
  inventoryItemId?: string | null;
  priceIdr: number;
  taxPercent?: number;
  /** What the scanner reads. Unique across the catalogue. */
  barcode?: string | null;
  /**
   * The pack this product is sold in. When it draws on stock, the factor is
   * resolved from the item's own packs rather than taken from here — two
   * places holding one conversion is two places for it to drift.
   */
  packUnit?: string;
  packFactor?: number;
  bonusXp?: number;
  imageUrl?: string | null;
  active?: boolean;
  available?: boolean;
}

export interface UpsertPackInput {
  unitCode: string;
  /** Base units inside one of these. A carton of 24 is 24. */
  factor: number;
  barcode?: string | null;
  purchaseDefault?: boolean;
  saleDefault?: boolean;
  active?: boolean;
}

export interface TenderInput {
  method: string;
  amountIdr: number;
  reference?: string | null;
}
