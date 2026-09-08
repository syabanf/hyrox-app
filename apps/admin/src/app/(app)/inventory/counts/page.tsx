'use client';

import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Ban, Check, Eye, Trash2 } from 'lucide-react';
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

export default function CountsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [branch, setBranch] = useState('');
  const [status, setStatus] = useState('');
  const [opening, setOpening] = useState(false);
  const [openTake, setOpenTake] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data: takes, isLoading, error: listError } = useQuery({
    queryKey: ['inventory', 'stock-takes', branch, status],
    queryFn: () =>
      api.admin.inventory.stockTakes.list({
        branchId: branch || undefined,
        status: status || undefined,
      }),
  });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const decide = useMutation({
    mutationFn: ({ id, action }: { id: string; action: 'apply' | 'cancel' }) =>
      api.admin.inventory.stockTakes.decide(id, action),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries({ queryKey: ['inventory'] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That count did not change.'),
  });

  const rows = takes ?? [];

  return (
    <div>
      <PageTitle
        title="Stock takes"
        subtitle="Applying a count writes one adjustment per varied line, and nothing for the rest"
        actions={
          can('inventory.count') ? (
            <button className="a-btn" onClick={() => setOpening(true)}>
              + New count
            </button>
          ) : undefined
        }
      />
      <ErrorNote message={error} />
      <QueryError error={listError} />

      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard label="Counts" value={rows.length} />
        <StatCard label="In progress" value={rows.filter((t) => t.status === 'DRAFT').length} tone="brand" />
        <StatCard label="Applied" value={rows.filter((t) => t.status === 'APPLIED').length} />
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
            options={['DRAFT', 'APPLIED', 'CANCELLED'].map((s) => ({
              value: s,
              label: s[0]! + s.slice(1).toLowerCase(),
            }))}
          />
        </div>
      </div>

      {isLoading ? (
        <Spinner label="Loading counts…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Count</th>
                <th>Branch</th>
                <th>Counted on</th>
                <th>Status</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((take) => (
                <tr key={take.id}>
                  <td className="font-bold">{take.takeNumber}</td>
                  <td className="text-sm">
                    {(branches ?? []).find((b) => b.id === take.branchId)?.name ?? take.branchId}
                  </td>
                  <td className="tabular-nums text-sm">{take.countedOn}</td>
                  <td>
                    <StatusBadge status={take.status} />
                  </td>
                  <td className="text-right">
                    <RowActions
                      items={[
                        { label: 'Open', icon: Eye, onClick: () => setOpenTake(take.id) },
                        {
                          label: 'Apply',
                          icon: Check,
                          disabled: !can('inventory.count') || take.status !== 'DRAFT',
                          onClick: () => {
                            if (confirm('Apply this count? It will move stock.'))
                              decide.mutate({ id: take.id, action: 'apply' });
                          },
                        },
                        {
                          label: 'Cancel',
                          icon: Ban,
                          tone: 'danger' as const,
                          disabled: !can('inventory.count') || take.status !== 'DRAFT',
                          onClick: () => decide.mutate({ id: take.id, action: 'cancel' }),
                        },
                      ]}
                    />
                  </td>
                </tr>
              ))}
              {rows.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-sm text-muted">
                    No counts yet.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      )}

      {opening ? (
        <OpenCountModal
          branches={(branches ?? []).map((b) => ({ id: b.id, name: b.name }))}
          onClose={() => setOpening(false)}
          onDone={(id) => {
            setOpening(false);
            setOpenTake(id);
            void qc.invalidateQueries({ queryKey: ['inventory'] });
          }}
        />
      ) : null}
      {openTake ? (
        <CountSheet
          takeId={openTake}
          onClose={() => {
            setOpenTake(null);
            void qc.invalidateQueries({ queryKey: ['inventory'] });
          }}
        />
      ) : null}
    </div>
  );
}

function OpenCountModal({
  branches,
  onClose,
  onDone,
}: {
  branches: { id: string; name: string }[];
  onClose: () => void;
  onDone: (id: string) => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [branch, setBranch] = useState(branches[0]?.id ?? '');
  const [note, setNote] = useState('');

  const save = useMutation({
    mutationFn: () => api.admin.inventory.stockTakes.open({ branchId: branch, note: note || null }),
    onSuccess: (take) => onDone(take.id),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That count did not open.'),
  });

  return (
    <Modal title="New stock take" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <Field label="Branch">
          <SearchSelect
            value={branch}
            onChange={setBranch}
            placeholder="Search branch…"
            options={branches.map((b) => ({ value: b.id, label: b.name }))}
          />
        </Field>
        <Field label="Note">
          <input className="a-input" value={note} onChange={(e) => setNote(e.target.value)} />
        </Field>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button className="a-btn" disabled={save.isPending || !branch} onClick={() => save.mutate()}>
            {save.isPending ? 'Opening…' : 'Open'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

/** The count sheet: type what is on the shelf, and see the variance appear. */
function CountSheet({ takeId, onClose }: { takeId: string; onClose: () => void }) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [item, setItem] = useState('');
  const [counted, setCounted] = useState('');

  const { data: take } = useQuery({
    queryKey: ['inventory', 'stock-take', takeId],
    queryFn: () => api.admin.inventory.stockTakes.get(takeId),
  });
  const { data: items } = useQuery({
    queryKey: ['inventory', 'items', 'all'],
    queryFn: () => api.admin.inventory.items.list(),
  });

  const count = useMutation({
    mutationFn: () =>
      api.admin.inventory.stockTakes.count(takeId, {
        itemId: item,
        qtyCounted: Number(counted),
      }),
    onSuccess: () => {
      setError(null);
      setItem('');
      setCounted('');
      void qc.invalidateQueries({ queryKey: ['inventory', 'stock-take', takeId] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That count did not save.'),
  });

  const remove = useMutation({
    mutationFn: (lineId: string) => api.admin.inventory.stockTakes.removeLine(takeId, lineId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['inventory', 'stock-take', takeId] }),
  });

  return (
    <Modal title={take ? `${take.takeNumber} · ${take.branchName}` : 'Stock take'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        {take?.status === 'DRAFT' ? (
          <div className="grid gap-2 sm:grid-cols-[1fr_7rem_auto]">
            <SearchSelect
              value={item}
              onChange={setItem}
              placeholder="Search item…"
              options={(items ?? []).map((i) => ({ value: i.id, label: i.name, hint: i.sku }))}
            />
            <input
              className="a-input"
              placeholder="Counted"
              value={counted}
              onChange={(e) => setCounted(e.target.value)}
            />
            <button
              className="a-btn"
              disabled={count.isPending || !item || counted === ''}
              onClick={() => count.mutate()}
            >
              Add
            </button>
          </div>
        ) : null}

        <div className="a-card !p-0 max-h-72 overflow-y-auto">
          <table className="a-table">
            <thead>
              <tr>
                <th>Item</th>
                <th className="text-right">Expected</th>
                <th className="text-right">Counted</th>
                <th className="text-right">Variance</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(take?.lines ?? []).map((line) => (
                <tr key={line.id}>
                  <td className="text-sm font-bold">{line.itemName}</td>
                  <td className="text-right tabular-nums text-sm text-muted">{line.qtyExpected}</td>
                  <td className="text-right tabular-nums text-sm">{line.qtyCounted}</td>
                  <td
                    className={`text-right tabular-nums font-bold ${
                      line.variance < 0 ? 'text-danger' : line.variance > 0 ? 'text-brand' : 'text-muted'
                    }`}
                  >
                    {line.variance > 0 ? '+' : ''}
                    {line.variance}
                    <p className="text-xs font-normal text-muted">{formatIdr(line.valueIdr)}</p>
                  </td>
                  <td className="text-right">
                    {take?.status === 'DRAFT' ? (
                      <RowActions
                        items={[
                          {
                            label: 'Remove',
                            icon: Trash2,
                            tone: 'danger' as const,
                            onClick: () => remove.mutate(line.id),
                          },
                        ]}
                      />
                    ) : null}
                  </td>
                </tr>
              ))}
              {(take?.lines ?? []).length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-6 text-center text-sm text-muted">
                    Nothing counted yet.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        {take ? (
          <p className="rounded-lg bg-surface-raised px-3 py-2 text-sm">
            {take.summary.linesVaried} of {take.summary.lines} lines varied ·{' '}
            <span className="font-bold">{take.summary.qtyShort} short</span>,{' '}
            {take.summary.qtyOver} over ·{' '}
            <span className={take.summary.valueVarianceIdr < 0 ? 'font-bold text-danger' : 'font-bold'}>
              {formatIdr(take.summary.valueVarianceIdr)}
            </span>
          </p>
        ) : null}

        <div className="mt-2 flex justify-end">
          <button className="a-btn-ghost" onClick={onClose}>
            Done
          </button>
        </div>
      </div>
    </Modal>
  );
}
