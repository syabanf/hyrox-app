'use client';

import type {
  CoachStatementView,
  IncentivePayoutView,
  IncentiveSchemeView,
} from '@nuhabit/contracts';
import type { CoachStatementLine, IncentiveScheme, PayoutStatus } from '@nuhabit/domain';
import { Spinner, StatusBadge, formatDay, formatDayTime, formatIdr } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Ban, Banknote, Check, ChevronDown, ChevronRight, Pencil, Trash2 } from 'lucide-react';
import { useState, type ReactNode } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import {
  ErrorNote,
  Modal,
  PageTitle,
  Pager,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../../components/ui';

const TABS = ['Statements', 'Payouts', 'Schemes'] as const;
type Tab = (typeof TABS)[number];

const monthLabel = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
const currentMonth = () => monthLabel(new Date());
const periodTitle = (period: string) =>
  new Date(`${period}-01T00:00:00`).toLocaleDateString(undefined, {
    month: 'long',
    year: 'numeric',
  });

export default function IncentivesPage() {
  const [tab, setTab] = useState<Tab>('Statements');
  return (
    <div>
      <PageTitle
        title="Coach Incentives"
        subtitle="Monthly coach pay computed from completed sessions and attendance - IDR payroll, independent of member credits"
      />
      <div className="mb-4 flex flex-wrap gap-1 rounded-xl bg-surface p-1">
        {TABS.map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded-lg px-3.5 py-1.5 text-sm font-bold ${
              tab === t ? 'bg-brand text-white' : 'text-muted hover:text-ink'
            }`}
          >
            {t}
          </button>
        ))}
      </div>
      {tab === 'Statements' ? <StatementsTab /> : null}
      {tab === 'Payouts' ? <PayoutsTab /> : null}
      {tab === 'Schemes' ? <SchemesTab /> : null}
    </div>
  );
}

// ── Shared: per-session lines ───────────────────────────────────────────────

function LinesTable({ lines }: { lines: CoachStatementLine[] }) {
  if (lines.length === 0)
    return <p className="px-3 py-3 text-sm text-muted">No completed sessions in this period.</p>;
  return (
    <div className="overflow-x-auto rounded-xl bg-surface-raised/60">
      <table className="a-table">
        <thead>
          <tr>
            <th>Session</th>
            <th>Class</th>
            <th className="text-right">Attended</th>
            <th className="text-right">No-shows</th>
            <th className="text-right">Fee</th>
            <th className="text-right">Attendee pay</th>
            <th className="text-right">Bonus</th>
            <th className="text-right">Penalty</th>
            <th className="text-right">Total</th>
          </tr>
        </thead>
        <tbody>
          {lines.map((l) => (
            <tr key={l.sessionId}>
              <td className="whitespace-nowrap text-muted">{formatDayTime(l.startsAt)}</td>
              <td className="font-bold">{l.classTypeName}</td>
              <td className="text-right">
                {l.attended}/{l.capacity}
                <span className="block text-xs text-muted">{l.booked} booked</span>
              </td>
              <td
                className={`text-right ${l.noShows > 0 ? 'font-bold text-danger' : 'text-muted'}`}
              >
                {l.noShows}
              </td>
              <td className="text-right">{formatIdr(l.sessionFeeIdr)}</td>
              <td className="text-right">{formatIdr(l.attendeeIdr)}</td>
              <td className={`text-right ${l.bonusIdr > 0 ? 'font-bold text-ok' : 'text-muted'}`}>
                {l.bonusIdr > 0 ? formatIdr(l.bonusIdr) : '-'}
              </td>
              <td
                className={`text-right ${l.penaltyIdr > 0 ? 'font-bold text-danger' : 'text-muted'}`}
              >
                {l.penaltyIdr > 0 ? `−${formatIdr(l.penaltyIdr)}` : '-'}
              </td>
              <td className="text-right font-black">{formatIdr(l.totalIdr)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ── Statements ──────────────────────────────────────────────────────────────

function StatementsTab() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [period, setPeriod] = useState(currentMonth());
  const [branchId, setBranchId] = useState('');
  const [open, setOpen] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });
  const { data, isLoading } = useQuery({
    queryKey: ['incentive-statements', period, branchId],
    queryFn: () => api.admin.incentives.statements({ period, branchId: branchId || undefined }),
    enabled: /^\d{4}-\d{2}$/.test(period),
  });

  const createPayout = useMutation({
    mutationFn: (coachId: string) =>
      api.admin.incentives.payouts.create({ coachId, periodMonth: period }),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Could not create payout.'),
  });

  const rows = data ?? [];
  const sum = (pick: (s: CoachStatementView) => number) => rows.reduce((t, s) => t + pick(s), 0);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-end gap-2">
        <div>
          <label className="a-label">Period</label>
          <input
            type="month"
            className="a-input w-44"
            value={period}
            onChange={(e) => {
              setPeriod(e.target.value);
              setOpen(null);
            }}
          />
        </div>
        <div className="w-48">
          <label className="a-label">Branch</label>
          <SearchSelect
            value={branchId}
            onChange={setBranchId}
            options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
            allowEmpty
            emptyLabel="All branches"
            placeholder="Search branch…"
          />
        </div>
      </div>
      <ErrorNote message={error} />
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label={`Payable · ${periodTitle(period)}`}
          value={formatIdr(sum((s) => s.totals.totalIdr))}
          tone="brand"
          hint="Sum of every coach's statement"
        />
        <StatCard tone="lime" label="Coaches" value={rows.length} hint="Active coaches with a statement" />
        <StatCard tone="ok"
          label="Sessions"
          value={sum((s) => s.totals.sessions)}
          hint="Completed in period"
        />
        <StatCard tone="warn"
          label="Attendees"
          value={sum((s) => s.totals.attended)}
          hint="Checked-in members"
        />
      </div>
      {isLoading ? (
        <Spinner label="Computing statements…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th />
                <th>Coach</th>
                <th className="text-right">Sessions</th>
                <th className="text-right">Attended</th>
                <th className="text-right">No-shows</th>
                <th className="text-right">Session fees</th>
                <th className="text-right">Attendee pay</th>
                <th className="text-right">Bonus</th>
                <th className="text-right">Penalty</th>
                <th className="text-right">Total</th>
                <th>Payout</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((s) => {
                const expanded = open === s.coachId;
                return (
                  <StatementRows
                    key={s.coachId}
                    statement={s}
                    expanded={expanded}
                    onToggle={() => setOpen(expanded ? null : s.coachId)}
                  >
                    {can('incentives.manage') && s.payout === null && s.totals.sessions > 0 ? (
                      <button
                        className="a-btn !px-3 !py-1 text-xs"
                        disabled={createPayout.isPending}
                        onClick={() => createPayout.mutate(s.coachId)}
                      >
                        Create payout
                      </button>
                    ) : s.totals.sessions === 0 ? (
                      <span className="text-xs text-muted">Nothing to pay</span>
                    ) : null}
                  </StatementRows>
                );
              })}
              {rows.length === 0 ? (
                <tr>
                  <td colSpan={12} className="py-6 text-center text-muted">
                    No active coaches for this filter.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      )}
      <p className="text-xs text-muted">
        Formula per session: session fee + attended × per-attendee + full-class bonus (attendance ≥
        threshold % of capacity) − no-shows × penalty, never below Rp0. Creating a payout freezes
        the statement; later attendance edits never change it.
      </p>
    </div>
  );
}

function StatementRows({
  statement: s,
  expanded,
  onToggle,
  children,
}: {
  statement: CoachStatementView;
  expanded: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  return (
    <>
      <tr className="cursor-pointer" onClick={onToggle}>
        <td className="w-8 text-muted">
          {expanded ? <ChevronDown size={15} /> : <ChevronRight size={15} />}
        </td>
        <td>
          <p className="font-bold">{s.coachName}</p>
          <p className="text-xs text-muted">
            {s.branchName} · {s.scheme.coachId ? 'coach scheme' : 'default scheme'}
          </p>
        </td>
        <td className="text-right">{s.totals.sessions}</td>
        <td className="text-right">{s.totals.attended}</td>
        <td className={`text-right ${s.totals.noShows > 0 ? 'text-danger' : 'text-muted'}`}>
          {s.totals.noShows}
        </td>
        <td className="text-right">{formatIdr(s.totals.sessionFeeIdr)}</td>
        <td className="text-right">{formatIdr(s.totals.attendeeIdr)}</td>
        <td className="text-right text-ok">
          {s.totals.bonusIdr > 0 ? formatIdr(s.totals.bonusIdr) : '-'}
        </td>
        <td className="text-right text-danger">
          {s.totals.penaltyIdr > 0 ? `−${formatIdr(s.totals.penaltyIdr)}` : '-'}
        </td>
        <td className="text-right font-black">{formatIdr(s.totals.totalIdr)}</td>
        <td>
          {s.payout ? (
            <StatusBadge status={s.payout.payout.status} />
          ) : (
            <span className="text-muted">-</span>
          )}
        </td>
        <td className="text-right" onClick={(e) => e.stopPropagation()}>
          {children}
        </td>
      </tr>
      {expanded ? (
        <tr>
          <td colSpan={12} className="!p-2">
            <LinesTable lines={s.lines} />
          </td>
        </tr>
      ) : null}
    </>
  );
}

// ── Payouts ─────────────────────────────────────────────────────────────────

const PAGE_SIZE = 10;

function PayoutsTab() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [status, setStatus] = useState('');
  const [period, setPeriod] = useState('');
  const [page, setPage] = useState(0);
  const [open, setOpen] = useState<string | null>(null);
  const [payTarget, setPayTarget] = useState<IncentivePayoutView | null>(null);
  const [voidTarget, setVoidTarget] = useState<IncentivePayoutView | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ['incentive-payouts'],
    queryFn: () => api.admin.incentives.payouts.list(),
  });

  const approve = useMutation({
    mutationFn: (id: string) => api.admin.incentives.payouts.action(id, 'approve'),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Approve failed.'),
  });

  const rows = (data ?? [])
    .filter((v) => !status || v.payout.status === status)
    .filter((v) => !period || v.periodMonth === period);
  const pageCount = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount - 1);
  const paged = rows.slice(safePage * PAGE_SIZE, safePage * PAGE_SIZE + PAGE_SIZE);
  const manage = can('incentives.manage');
  const totalFor = (st: PayoutStatus) =>
    (data ?? [])
      .filter((v) => v.payout.status === st)
      .reduce((t, v) => t + v.payout.statement.totals.totalIdr, 0);

  return (
    <div className="flex flex-col gap-4">
      <ErrorNote message={error} />
      <div className="grid gap-3 sm:grid-cols-3">
        <StatCard
          label="Awaiting approval"
          value={formatIdr(totalFor('DRAFT'))}
          hint="DRAFT payouts"
        />
        <StatCard
          label="Approved, unpaid"
          value={formatIdr(totalFor('APPROVED'))}
          tone="brand"
          hint="Ready for transfer"
        />
        <StatCard label="Paid out" value={formatIdr(totalFor('PAID'))} hint="All time" />
      </div>
      <div className="flex flex-wrap gap-2">
        <div className="w-44">
          <SearchSelect
            value={status}
            onChange={(v) => {
              setStatus(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All statuses"
            placeholder="Search status…"
            options={['DRAFT', 'APPROVED', 'PAID', 'VOID'].map((s) => ({ value: s, label: s }))}
          />
        </div>
        <input
          type="month"
          className="a-input w-44"
          value={period}
          onChange={(e) => {
            setPeriod(e.target.value);
            setPage(0);
          }}
        />
      </div>
      {isLoading ? (
        <Spinner label="Loading payouts…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th />
                <th>Period</th>
                <th>Coach</th>
                <th className="text-right">Sessions</th>
                <th className="text-right">Total</th>
                <th>Status</th>
                <th>Approved</th>
                <th>Paid</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {paged.map((v) => {
                const p = v.payout;
                const expanded = open === p.id;
                return (
                  <PayoutRows
                    key={p.id}
                    view={v}
                    expanded={expanded}
                    onToggle={() => setOpen(expanded ? null : p.id)}
                  >
                    {manage ? (
                      <RowActions
                        items={[
                          {
                            label: 'Approve',
                            icon: Check,
                            disabled: p.status !== 'DRAFT' || approve.isPending,
                            onClick: () => approve.mutate(p.id),
                          },
                          {
                            label: 'Mark paid',
                            icon: Banknote,
                            disabled: p.status !== 'APPROVED',
                            onClick: () => setPayTarget(v),
                          },
                          {
                            label: 'Void',
                            icon: Ban,
                            tone: 'danger',
                            disabled: p.status === 'PAID' || p.status === 'VOID',
                            onClick: () => setVoidTarget(v),
                          },
                        ]}
                      />
                    ) : null}
                  </PayoutRows>
                );
              })}
              {rows.length === 0 ? (
                <tr>
                  <td colSpan={9} className="py-6 text-center text-muted">
                    No payouts yet - create one from the Statements tab.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
          <Pager page={safePage} pageCount={pageCount} onPage={setPage} />
        </div>
      )}
      {payTarget ? (
        <PayoutActionModal
          view={payTarget}
          action="pay"
          onClose={() => setPayTarget(null)}
          onDone={() => {
            setPayTarget(null);
            void qc.invalidateQueries();
          }}
        />
      ) : null}
      {voidTarget ? (
        <PayoutActionModal
          view={voidTarget}
          action="void"
          onClose={() => setVoidTarget(null)}
          onDone={() => {
            setVoidTarget(null);
            void qc.invalidateQueries();
          }}
        />
      ) : null}
    </div>
  );
}

function PayoutRows({
  view: v,
  expanded,
  onToggle,
  children,
}: {
  view: IncentivePayoutView;
  expanded: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  const p = v.payout;
  return (
    <>
      <tr className="cursor-pointer" onClick={onToggle}>
        <td className="w-8 text-muted">
          {expanded ? <ChevronDown size={15} /> : <ChevronRight size={15} />}
        </td>
        <td className="font-bold">{periodTitle(v.periodMonth)}</td>
        <td>
          <p className="font-bold">{v.coachName}</p>
          <p className="text-xs text-muted">{v.branchName}</p>
        </td>
        <td className="text-right">{p.statement.totals.sessions}</td>
        <td className="text-right font-black">{formatIdr(p.statement.totals.totalIdr)}</td>
        <td>
          <StatusBadge status={p.status} />
        </td>
        <td className="text-muted">{p.approvedAt ? formatDay(p.approvedAt) : '-'}</td>
        <td className="text-muted">
          {p.paidAt ? (
            <>
              {formatDay(p.paidAt)}
              <span className="block font-mono text-xs">{p.paymentReference}</span>
            </>
          ) : (
            '-'
          )}
          {p.status === 'VOID' && p.note ? <span className="block text-xs">{p.note}</span> : null}
        </td>
        <td className="text-right" onClick={(e) => e.stopPropagation()}>
          {children}
        </td>
      </tr>
      {expanded ? (
        <tr>
          <td colSpan={9} className="!p-2">
            <LinesTable lines={p.statement.lines} />
          </td>
        </tr>
      ) : null}
    </>
  );
}

function PayoutActionModal({
  view,
  action,
  onClose,
  onDone,
}: {
  view: IncentivePayoutView;
  action: 'pay' | 'void';
  onClose: () => void;
  onDone: () => void;
}) {
  const [value, setValue] = useState('');
  const [error, setError] = useState<string | null>(null);
  const mutation = useMutation({
    mutationFn: () =>
      api.admin.incentives.payouts.action(
        view.payout.id,
        action,
        action === 'pay' ? { paymentReference: value } : { note: value },
      ),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Action failed.'),
  });
  const total = formatIdr(view.payout.statement.totals.totalIdr);
  return (
    <Modal
      title={action === 'pay' ? `Mark paid - ${view.coachName}` : `Void payout - ${view.coachName}`}
      onClose={onClose}
    >
      <div className="flex flex-col gap-3">
        <p className="text-sm text-muted">
          {periodTitle(view.periodMonth)} · <span className="font-bold text-ink">{total}</span>
          {action === 'void'
            ? ' - a voided payout frees the period so a new one can be created.'
            : ''}
        </p>
        <div>
          <label className="a-label">
            {action === 'pay' ? 'Payment / transfer reference' : 'Reason'}
          </label>
          <input
            className="a-input"
            autoFocus
            placeholder={
              action === 'pay' ? 'e.g. TRF-20260901-0007' : 'e.g. Wrong month, attendance corrected'
            }
            value={value}
            onChange={(e) => setValue(e.target.value)}
          />
        </div>
        <ErrorNote message={error} />
        <button
          className={action === 'pay' ? 'a-btn' : 'a-btn-danger'}
          disabled={mutation.isPending || value.trim().length < 3}
          onClick={() => mutation.mutate()}
        >
          {action === 'pay' ? `Confirm paid ${total}` : 'Void payout'}
        </button>
      </div>
    </Modal>
  );
}

// ── Schemes ─────────────────────────────────────────────────────────────────

const RATE_FIELDS: { key: keyof RateDraft; label: string; hint: string; suffix?: string }[] = [
  {
    key: 'sessionFeeIdr',
    label: 'Session fee (IDR)',
    hint: 'Flat amount for every completed session.',
  },
  {
    key: 'perAttendeeIdr',
    label: 'Per attendee (IDR)',
    hint: 'Paid for each member who checked in.',
  },
  {
    key: 'fullClassBonusIdr',
    label: 'Full-class bonus (IDR)',
    hint: 'Extra when attendance reaches the threshold.',
  },
  {
    key: 'fullClassThresholdPercent',
    label: 'Full-class threshold (%)',
    hint: 'Attended ÷ capacity at or above this % earns the bonus.',
    suffix: '%',
  },
  {
    key: 'noShowPenaltyIdr',
    label: 'No-show penalty (IDR)',
    hint: 'Deducted per no-show; 0 disables it. A session never drops below Rp0.',
  },
];

interface RateDraft {
  sessionFeeIdr: string;
  perAttendeeIdr: string;
  fullClassBonusIdr: string;
  fullClassThresholdPercent: string;
  noShowPenaltyIdr: string;
}

/** One class type paid at its own rate. */
interface ClassRateDraft {
  classTypeId: string;
  sessionFeeIdr: string;
  perAttendeeIdr: string;
}

const toDraft = (s: IncentiveScheme | null): RateDraft => ({
  sessionFeeIdr: String(s?.sessionFeeIdr ?? 150_000),
  perAttendeeIdr: String(s?.perAttendeeIdr ?? 10_000),
  fullClassBonusIdr: String(s?.fullClassBonusIdr ?? 100_000),
  fullClassThresholdPercent: String(s?.fullClassThresholdPercent ?? 80),
  noShowPenaltyIdr: String(s?.noShowPenaltyIdr ?? 0),
});

const toClassRates = (s: IncentiveScheme | null): ClassRateDraft[] =>
  (s?.rates ?? []).map((r) => ({
    classTypeId: r.classTypeId,
    sessionFeeIdr: String(r.sessionFeeIdr),
    perAttendeeIdr: String(r.perAttendeeIdr),
  }));

const fromDraft = (d: RateDraft, classRates: ClassRateDraft[] = []) => ({
  sessionFeeIdr: Number(d.sessionFeeIdr),
  perAttendeeIdr: Number(d.perAttendeeIdr),
  fullClassBonusIdr: Number(d.fullClassBonusIdr),
  fullClassThresholdPercent: Number(d.fullClassThresholdPercent),
  noShowPenaltyIdr: Number(d.noShowPenaltyIdr),
  rates: classRates
    .filter((r) => r.classTypeId !== '')
    .map((r) => ({
      classTypeId: r.classTypeId,
      sessionFeeIdr: Number(r.sessionFeeIdr || 0),
      perAttendeeIdr: Number(r.perAttendeeIdr || 0),
    })),
});

const draftValid = (d: RateDraft, classRates: ClassRateDraft[] = []) =>
  Object.values(d).every((v) => v.trim() !== '' && Number.isInteger(Number(v)) && Number(v) >= 0) &&
  Number(d.fullClassThresholdPercent) <= 100 &&
  // A row with no class type chosen is a half-filled line, not a rate.
  classRates.every(
    (r) =>
      r.classTypeId !== '' &&
      Number.isInteger(Number(r.sessionFeeIdr)) &&
      Number(r.sessionFeeIdr) >= 0 &&
      Number.isInteger(Number(r.perAttendeeIdr)) &&
      Number(r.perAttendeeIdr) >= 0,
  ) &&
  // The server refuses two rates for one class; say so before it does.
  new Set(classRates.map((r) => r.classTypeId)).size === classRates.length;

/**
 * Per-class rates.
 *
 * A studio pays the same for every class until it does not: a ninety-minute
 * race simulation is not a forty-five-minute mobility class, and paying one
 * figure for both is how you lose whoever runs the long one.
 */
function ClassRateFields({
  rates,
  classTypes,
  onChange,
  disabled,
}: {
  rates: ClassRateDraft[];
  classTypes: { id: string; name: string }[];
  onChange: (next: ClassRateDraft[]) => void;
  disabled?: boolean;
}) {
  const taken = new Set(rates.map((r) => r.classTypeId));
  return (
    <div className="flex flex-col gap-2">
      <div>
        <p className="a-label !mb-0">Rates by class type</p>
        <p className="text-xs text-muted">
          Optional. A class type listed here is paid at its own figures; everything else uses the
          fee above.
        </p>
      </div>
      {rates.map((rate, i) => (
        <div key={i} className="grid gap-2 sm:grid-cols-[1fr_auto_auto_auto] sm:items-end">
          <div>
            <SearchSelect
              value={rate.classTypeId}
              onChange={(v) =>
                onChange(rates.map((r, j) => (i === j ? { ...r, classTypeId: v } : r)))
              }
              placeholder="Search class type…"
              options={classTypes
                .filter((t) => t.id === rate.classTypeId || !taken.has(t.id))
                .map((t) => ({ value: t.id, label: t.name }))}
            />
          </div>
          <div className="w-32">
            <label className="a-label">Session fee</label>
            <input
              className="a-input"
              inputMode="numeric"
              disabled={disabled}
              value={rate.sessionFeeIdr}
              onChange={(e) =>
                onChange(rates.map((r, j) => (i === j ? { ...r, sessionFeeIdr: e.target.value } : r)))
              }
            />
          </div>
          <div className="w-32">
            <label className="a-label">Per attendee</label>
            <input
              className="a-input"
              inputMode="numeric"
              disabled={disabled}
              value={rate.perAttendeeIdr}
              onChange={(e) =>
                onChange(
                  rates.map((r, j) => (i === j ? { ...r, perAttendeeIdr: e.target.value } : r)),
                )
              }
            />
          </div>
          <button
            className="a-btn-danger !px-3"
            disabled={disabled}
            onClick={() => onChange(rates.filter((_, j) => j !== i))}
            title="Remove this rate"
          >
            <Trash2 size={15} />
          </button>
        </div>
      ))}
      <button
        className="a-btn-ghost self-start !px-3 !py-1.5 text-xs"
        disabled={disabled || rates.length >= classTypes.length}
        onClick={() =>
          onChange([...rates, { classTypeId: '', sessionFeeIdr: '0', perAttendeeIdr: '0' }])
        }
      >
        + Add a class rate
      </button>
    </div>
  );
}

function RateFields({
  draft,
  onChange,
  disabled,
}: {
  draft: RateDraft;
  onChange: (next: RateDraft) => void;
  disabled?: boolean;
}) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      {RATE_FIELDS.map((f) => (
        <div key={f.key}>
          <label className="a-label">{f.label}</label>
          <input
            className="a-input"
            inputMode="numeric"
            disabled={disabled}
            value={draft[f.key]}
            onChange={(e) => onChange({ ...draft, [f.key]: e.target.value })}
          />
          <p className="mt-1 text-xs text-muted">{f.hint}</p>
        </div>
      ))}
    </div>
  );
}

function SchemesTab() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const manage = can('incentives.manage');
  const [editTarget, setEditTarget] = useState<IncentiveSchemeView | 'new' | null>(null);
  const { data, isLoading } = useQuery({
    queryKey: ['incentive-schemes'],
    queryFn: api.admin.incentives.schemes.list,
  });
  const { data: coaches } = useQuery({ queryKey: ['coaches'], queryFn: api.admin.coaches.list });
  const { data: classTypes } = useQuery({
    queryKey: ['class-types'],
    queryFn: api.admin.classTypes.list,
  });
  const classTypeOptions = (classTypes ?? []).map((t) => ({ id: t.id, name: t.name }));

  if (isLoading || !data) return <Spinner label="Loading schemes…" />;
  const defaultView = data.find((v) => v.scheme.coachId === null) ?? null;
  const overrides = data.filter((v) => v.scheme.coachId !== null);
  const done = () => {
    setEditTarget(null);
    void qc.invalidateQueries();
  };

  return (
    <div className="flex flex-col gap-4">
      {!manage ? (
        <p className="rounded-lg bg-warn/10 px-3 py-2 text-sm font-bold text-warn">
          Read-only - HQ and Finance manage incentive schemes (enforced by the server).
        </p>
      ) : null}
      <DefaultSchemeCard
        scheme={defaultView?.scheme ?? null}
        classTypes={classTypeOptions}
        canEdit={manage}
      />
      <div className="a-card !p-0">
        <div className="flex flex-wrap items-center justify-between gap-2 px-4 pt-4">
          <div>
            <h2 className="display text-lg font-black">Coach overrides</h2>
            <p className="text-xs text-muted">
              A coach with an active override is paid by it instead of the default.
            </p>
          </div>
          {manage ? (
            <button className="a-btn !px-3 !py-1.5 text-xs" onClick={() => setEditTarget('new')}>
              + Add override
            </button>
          ) : null}
        </div>
        <table className="a-table mt-2">
          <thead>
            <tr>
              <th>Coach</th>
              <th className="text-right">Session fee</th>
              <th className="text-right">Per attendee</th>
              <th className="text-right">Bonus</th>
              <th className="text-right">Threshold</th>
              <th className="text-right">No-show penalty</th>
              <th className="text-right">Class rates</th>
              <th>Status</th>
              <th>Updated</th>
              <th className="text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {overrides.map((v) => (
              <tr key={v.scheme.id}>
                <td className="font-bold">{v.coachName}</td>
                <td className="text-right">{formatIdr(v.scheme.sessionFeeIdr)}</td>
                <td className="text-right">{formatIdr(v.scheme.perAttendeeIdr)}</td>
                <td className="text-right">{formatIdr(v.scheme.fullClassBonusIdr)}</td>
                <td className="text-right">{v.scheme.fullClassThresholdPercent}%</td>
                <td className="text-right">
                  {v.scheme.noShowPenaltyIdr > 0 ? formatIdr(v.scheme.noShowPenaltyIdr) : '-'}
                </td>
                <td className="text-right">
                  {v.scheme.rates.length > 0 ? v.scheme.rates.length : '-'}
                </td>
                <td>
                  <StatusBadge status={v.scheme.active ? 'ACTIVE' : 'INACTIVE'} />
                </td>
                <td className="text-muted">{formatDay(v.scheme.updatedAt)}</td>
                <td className="text-right">
                  {manage ? (
                    <RowActions
                      items={[{ label: 'Edit', icon: Pencil, onClick: () => setEditTarget(v) }]}
                    />
                  ) : null}
                </td>
              </tr>
            ))}
            {overrides.length === 0 ? (
              <tr>
                <td colSpan={9} className="py-6 text-center text-muted">
                  No overrides - every coach is on the default scheme.
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
      {editTarget ? (
        <OverrideModal
          view={editTarget === 'new' ? null : editTarget}
          coaches={(coaches ?? [])
            .filter((c) => c.status === 'ACTIVE')
            .filter(
              (c) => editTarget !== 'new' || !overrides.some((o) => o.scheme.coachId === c.id),
            )
            .map((c) => ({ value: c.id, label: c.name }))}
          classTypes={classTypeOptions}
          defaults={defaultView?.scheme ?? null}
          onClose={() => setEditTarget(null)}
          onDone={done}
        />
      ) : null}
    </div>
  );
}

function DefaultSchemeCard({
  scheme,
  classTypes,
  canEdit,
}: {
  scheme: IncentiveScheme | null;
  classTypes: { id: string; name: string }[];
  canEdit: boolean;
}) {
  const qc = useQueryClient();
  const [draft, setDraft] = useState<RateDraft>(toDraft(scheme));
  const [classRates, setClassRates] = useState<ClassRateDraft[]>(toClassRates(scheme));
  const [dirty, setDirty] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: () => {
      const body = { coachId: null, ...fromDraft(draft, classRates), active: true };
      return scheme
        ? api.admin.incentives.schemes.update(scheme.id, body)
        : api.admin.incentives.schemes.create(body);
    },
    onSuccess: () => {
      setDirty(false);
      setSaved(true);
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });
  return (
    <div className="a-card flex flex-col gap-4">
      <div>
        <h2 className="display text-lg font-black">Organization default</h2>
        <p className="text-xs text-muted">
          Applies to every coach without an active override. Changes are audited and affect future
          statements only - existing payouts stay frozen.
        </p>
      </div>
      <RateFields
        draft={draft}
        disabled={!canEdit}
        onChange={(next) => {
          setDraft(next);
          setDirty(true);
          setSaved(false);
        }}
      />
      <ClassRateFields
        rates={classRates}
        classTypes={classTypes}
        disabled={!canEdit}
        onChange={(next) => {
          setClassRates(next);
          setDirty(true);
          setSaved(false);
        }}
      />
      <ErrorNote message={error} />
      {canEdit ? (
        <div className="flex items-center gap-3">
          <button
            className="a-btn"
            disabled={!dirty || !draftValid(draft, classRates) || save.isPending}
            onClick={() => save.mutate()}
          >
            Save default scheme
          </button>
          {saved ? <span className="text-sm font-bold text-ok">Saved.</span> : null}
          {scheme ? (
            <span className="text-xs text-muted">Updated {formatDay(scheme.updatedAt)}</span>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function OverrideModal({
  view,
  coaches,
  classTypes,
  defaults,
  onClose,
  onDone,
}: {
  view: IncentiveSchemeView | null;
  coaches: { value: string; label: string }[];
  classTypes: { id: string; name: string }[];
  defaults: IncentiveScheme | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [coachId, setCoachId] = useState(view?.scheme.coachId ?? '');
  const [draft, setDraft] = useState<RateDraft>(toDraft(view?.scheme ?? defaults));
  const [classRates, setClassRates] = useState<ClassRateDraft[]>(
    toClassRates(view?.scheme ?? null),
  );
  const [active, setActive] = useState(view?.scheme.active ?? true);
  const [error, setError] = useState<string | null>(null);
  const mutation = useMutation({
    mutationFn: () => {
      const body = { coachId, ...fromDraft(draft, classRates), active };
      return view
        ? api.admin.incentives.schemes.update(view.scheme.id, body)
        : api.admin.incentives.schemes.create(body);
    },
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });
  return (
    <Modal
      title={view ? `Edit override - ${view.coachName}` : 'New coach override'}
      onClose={onClose}
    >
      <div className="flex flex-col gap-4">
        {!view ? (
          <div>
            <label className="a-label">Coach</label>
            <SearchSelect
              value={coachId}
              onChange={setCoachId}
              placeholder="Search coach…"
              options={coaches}
            />
            {coaches.length === 0 ? (
              <p className="mt-1 text-xs text-muted">Every active coach already has an override.</p>
            ) : null}
          </div>
        ) : null}
        <RateFields draft={draft} onChange={setDraft} />
        <ClassRateFields rates={classRates} classTypes={classTypes} onChange={setClassRates} />
        <label className="flex items-center gap-2 text-sm font-bold">
          <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} />
          Active - inactive overrides fall back to the default scheme
        </label>
        <ErrorNote message={error} />
        <button
          className="a-btn"
          disabled={mutation.isPending || !coachId || !draftValid(draft, classRates)}
          onClick={() => mutation.mutate()}
        >
          {view ? 'Save changes' : 'Create override'}
        </button>
      </div>
    </Modal>
  );
}
