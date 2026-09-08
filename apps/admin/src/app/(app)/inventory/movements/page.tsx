'use client';

import { MOVEMENT_KINDS, MOVEMENT_LABELS } from '@nuhabit/domain';
import { formatDayTime, formatIdr, Spinner } from '@nuhabit/ui';
import { useQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { useState } from 'react';
import { Pager, PageTitle, QueryError, SearchSelect, StatCard } from '../../../../components/ui';
import { api } from '../../../../lib/api';

const PAGE_SIZE = 30;

export default function MovementsPage() {
  const [branch, setBranch] = useState('');
  const [kind, setKind] = useState('');
  const [item, setItem] = useState('');
  const [page, setPage] = useState(0);

  const { data: movements, isLoading, error } = useQuery({
    queryKey: ['inventory', 'movements', branch, kind, item],
    queryFn: () =>
      api.admin.inventory.movements({
        branchId: branch || undefined,
        kind: kind || undefined,
        itemId: item || undefined,
        limit: 500,
      }),
  });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });
  const { data: items } = useQuery({
    queryKey: ['inventory', 'items', 'all'],
    queryFn: () => api.admin.inventory.items.list(),
  });

  const rows = movements ?? [];
  const shown = rows.slice(page * PAGE_SIZE, (page + 1) * PAGE_SIZE);
  const pageCount = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));
  const names = new Map((items ?? []).map((i) => [i.id, i.name]));

  return (
    <div>
      <PageTitle
        title="Stock ledger"
        subtitle="Every change to every quantity, kept forever"
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard label="Movements" value={rows.length} />
        <StatCard label="Received" value={rows.filter((m) => m.qty > 0).length} tone="brand" />
        <StatCard label="Issued" value={rows.filter((m) => m.qty < 0).length} />
        <StatCard
          label="Adjustments"
          value={rows.filter((m) => m.kind === 'ADJUSTMENT').length}
          hint="Each one carries a reason"
        />
      </div>

      <div className="mb-4 flex flex-wrap gap-2">
        <div className="w-44">
          <SearchSelect
            value={branch}
            onChange={(v) => {
              setBranch(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All branches"
            placeholder="Search branch…"
            options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
          />
        </div>
        <div className="w-52">
          <SearchSelect
            value={item}
            onChange={(v) => {
              setItem(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="All items"
            placeholder="Search item…"
            options={(items ?? []).map((i) => ({ value: i.id, label: i.name, hint: i.sku }))}
          />
        </div>
        <div className="w-44">
          <SearchSelect
            value={kind}
            onChange={(v) => {
              setKind(v);
              setPage(0);
            }}
            allowEmpty
            emptyLabel="Any kind"
            placeholder="Search kind…"
            options={MOVEMENT_KINDS.map((k) => ({ value: k, label: MOVEMENT_LABELS[k] }))}
          />
        </div>
      </div>

      {isLoading ? (
        <Spinner label="Reading the ledger…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>When</th>
                  <th>Item</th>
                  <th>What happened</th>
                  <th className="text-right">Change</th>
                  <th className="text-right">Before → after</th>
                  <th className="text-right">Value</th>
                  <th>Who</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((m) => (
                  <tr key={m.id}>
                    <td className="text-xs text-muted">{formatDayTime(m.createdAt)}</td>
                    <td>
                      <Link href={`/inventory/items/${m.itemId}`} className="font-bold hover:text-brand">
                        {names.get(m.itemId) ?? m.itemId}
                      </Link>
                    </td>
                    <td className="text-sm">
                      <span className="font-bold">{MOVEMENT_LABELS[m.kind]}</span>
                      {m.referenceNumber ? (
                        <p className="text-xs text-muted">{m.referenceNumber}</p>
                      ) : m.reason ? (
                        <p className="text-xs text-muted">{m.reason}</p>
                      ) : null}
                    </td>
                    <td
                      className={`text-right tabular-nums font-bold ${
                        m.qty < 0 ? 'text-danger' : 'text-brand'
                      }`}
                    >
                      {m.qty > 0 ? '+' : ''}
                      {m.qty}
                    </td>
                    <td className="text-right tabular-nums text-sm text-muted">
                      {m.qtyBefore} → {m.qtyAfter}
                    </td>
                    <td className="text-right tabular-nums text-sm">{formatIdr(m.totalCostIdr)}</td>
                    <td className="text-xs text-muted">{m.actorName ?? '—'}</td>
                  </tr>
                ))}
                {shown.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-sm text-muted">
                      Nothing has moved yet.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageCount={pageCount} onPage={setPage} />
        </div>
      )}
    </div>
  );
}
