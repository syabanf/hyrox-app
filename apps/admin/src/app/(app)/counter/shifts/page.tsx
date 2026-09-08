'use client';

import { formatDayTime, formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Eye, LockKeyhole } from 'lucide-react';
import Link from 'next/link';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  QueryError,
  RowActions,
  SearchSelect,
  StatCard,
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

export default function ShiftsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [branch, setBranch] = useState('');
  const [status, setStatus] = useState('');
  const [closing, setClosing] = useState<string | null>(null);
  const [opened, setOpened] = useState<string | null>(null);

  const { data: shifts, isLoading, error } = useQuery({
    queryKey: ['pos', 'shifts', branch, status],
    queryFn: () =>
      api.admin.pos.shifts.list({ branchId: branch || undefined, status: status || undefined }),
  });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const rows = shifts ?? [];

  return (
    <div>
      <PageTitle
        title="Till shifts"
        subtitle="The drawer counts cash and nothing else — a card sale never touched it"
        actions={
          <Link href="/counter" className="a-btn-ghost">
            Till
          </Link>
        }
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard label="Shifts" value={rows.length} />
        <StatCard label="Open now" value={rows.filter((s) => s.status === 'OPEN').length} tone="brand" />
        <StatCard
          label="Short or over"
          value={rows.filter((s) => s.status === 'CLOSED' && s.varianceIdr !== 0).length}
          tone={rows.some((s) => s.status === 'CLOSED' && s.varianceIdr !== 0) ? 'danger' : undefined}
        />
      </div>

      <div className="mb-4 flex flex-wrap gap-2">
        <div className="w-44">
          <SearchSelect
            value={branch}
            onChange={setBranch}
            allowEmpty
            emptyLabel="All branches"
            placeholder="Search branch…"
            options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
          />
        </div>
        <div className="w-40">
          <SearchSelect
            value={status}
            onChange={setStatus}
            allowEmpty
            emptyLabel="Any status"
            placeholder="Search status…"
            options={['OPEN', 'CLOSED'].map((s) => ({ value: s, label: s[0]! + s.slice(1).toLowerCase() }))}
          />
        </div>
      </div>

      {isLoading ? (
        <Spinner label="Loading shifts…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Shift</th>
                  <th>Cashier</th>
                  <th>Opened</th>
                  <th className="text-right">Float</th>
                  <th className="text-right">Expected</th>
                  <th className="text-right">Counted</th>
                  <th className="text-right">Variance</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((shift) => (
                  <tr key={shift.id}>
                    <td>
                      <button className="font-bold hover:text-brand" onClick={() => setOpened(shift.id)}>
                        {shift.shiftNumber}
                      </button>
                      <p className="text-xs">
                        <StatusBadge status={shift.status} />
                      </p>
                    </td>
                    <td className="text-sm">{shift.cashierName}</td>
                    <td className="text-xs text-muted">{formatDayTime(shift.openedAt)}</td>
                    <td className="text-right tabular-nums text-sm">{formatIdr(shift.openingCashIdr)}</td>
                    <td className="text-right tabular-nums text-sm">
                      {shift.status === 'CLOSED' ? formatIdr(shift.expectedCashIdr) : '—'}
                    </td>
                    <td className="text-right tabular-nums text-sm">
                      {shift.closingCashIdr != null ? formatIdr(shift.closingCashIdr) : '—'}
                    </td>
                    <td
                      className={`text-right tabular-nums font-bold ${
                        shift.status === 'CLOSED' && shift.varianceIdr !== 0 ? 'text-danger' : ''
                      }`}
                    >
                      {shift.status === 'CLOSED' ? formatIdr(shift.varianceIdr) : '—'}
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          { label: 'Open', icon: Eye, onClick: () => setOpened(shift.id) },
                          {
                            label: 'Close the till',
                            icon: LockKeyhole,
                            disabled: !can('pos.sell') || shift.status !== 'OPEN',
                            onClick: () => setClosing(shift.id),
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="py-8 text-center text-sm text-muted">
                      No shifts yet.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {closing ? (
        <CloseModal
          shiftId={closing}
          onClose={() => setClosing(null)}
          onDone={() => {
            setClosing(null);
            void qc.invalidateQueries({ queryKey: ['pos'] });
          }}
        />
      ) : null}
      {opened ? <ShiftSheet shiftId={opened} onClose={() => setOpened(null)} /> : null}
    </div>
  );
}

function CloseModal({
  shiftId,
  onClose,
  onDone,
}: {
  shiftId: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [counted, setCounted] = useState('');
  const [note, setNote] = useState('');

  const { data: shift } = useQuery({
    queryKey: ['pos', 'shift', shiftId],
    queryFn: () => api.admin.pos.shifts.get(shiftId),
  });

  const save = useMutation({
    mutationFn: () =>
      api.admin.pos.shifts.close(shiftId, { countedCashIdr: Number(counted), note: note || null }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That till did not close.'),
  });

  const expected = shift?.totals.expectedCashIdr ?? 0;
  const variance = counted === '' ? 0 : Number(counted) - expected;

  return (
    <Modal title="Close the till" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        {shift ? (
          <div className="rounded-lg bg-surface-raised px-3 py-2 text-sm">
            <p>
              <span className="font-bold">{formatIdr(expected)}</span> should be in the drawer:{' '}
              {formatIdr(shift.openingCashIdr)} float plus {formatIdr(shift.totals.cashIdr)} cash
              taken.
            </p>
            <p className="mt-1 text-xs text-muted">
              Card and QRIS takings ({formatIdr(shift.totals.qrisIdr + shift.totals.cardIdr)}) never
              touched the drawer and are not counted here.
            </p>
          </div>
        ) : null}
        <Field label="Counted in the drawer">
          <input className="a-input" value={counted} onChange={(e) => setCounted(e.target.value)} />
        </Field>
        {counted !== '' ? (
          <p className={`text-sm font-bold ${variance === 0 ? 'text-brand' : 'text-danger'}`}>
            {variance === 0
              ? 'Balances exactly.'
              : `${variance > 0 ? 'Over' : 'Short'} by ${formatIdr(Math.abs(variance))}.`}
          </p>
        ) : null}
        <Field label="Note">
          <input className="a-input" value={note} onChange={(e) => setNote(e.target.value)} />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || counted === ''} onClick={() => save.mutate()}>
            {save.isPending ? 'Closing…' : 'Close'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function ShiftSheet({ shiftId, onClose }: { shiftId: string; onClose: () => void }) {
  const { data: shift } = useQuery({
    queryKey: ['pos', 'shift', shiftId],
    queryFn: () => api.admin.pos.shifts.get(shiftId),
  });

  return (
    <Modal title={shift?.shiftNumber ?? 'Shift'} onClose={onClose}>
      {!shift ? (
        <Spinner label="Loading…" />
      ) : (
        <div className="grid gap-3">
          <p className="text-sm text-muted">
            {shift.cashierName} · opened {formatDayTime(shift.openedAt)}
            {shift.closedAt ? ` · closed ${formatDayTime(shift.closedAt)}` : ''}
          </p>
          <div className="grid gap-1 text-sm">
            <Line label="Sales" value={formatIdr(shift.totals.salesIdr)} bold />
            <Line label="Cash" value={formatIdr(shift.totals.cashIdr)} />
            <Line label="QRIS" value={formatIdr(shift.totals.qrisIdr)} />
            <Line label="Card" value={formatIdr(shift.totals.cardIdr)} />
            <Line label="Transfer" value={formatIdr(shift.totals.transferIdr)} />
            <Line label="Member credit" value={formatIdr(shift.totals.memberCreditIdr)} />
            {shift.totals.voidedIdr > 0 ? (
              <Line label="Voided" value={formatIdr(shift.totals.voidedIdr)} />
            ) : null}
            <div className="mt-1 border-t border-line pt-1">
              <Line label="Expected in the drawer" value={formatIdr(shift.totals.expectedCashIdr)} bold />
            </div>
          </div>
          <div className="mt-2 flex justify-end">
            <button className="a-btn-ghost" onClick={onClose}>
              Close
            </button>
          </div>
        </div>
      )}
    </Modal>
  );
}

function Line({ label, value, bold }: { label: string; value: string; bold?: boolean }) {
  return (
    <div className="flex items-center justify-between">
      <span className={bold ? 'font-bold' : 'text-muted'}>{label}</span>
      <span className={`tabular-nums ${bold ? 'font-bold' : ''}`}>{value}</span>
    </div>
  );
}
