'use client';

import type { EXPIRY_LABELS } from '@nuhabit/domain';
import { formatIdr, Spinner } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { PageTitle, QueryError, SearchSelect, StatCard } from '../../../../components/ui';
import { api } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

/**
 * What is about to go off, and what it is worth.
 *
 * The two halves are deliberately separate. Near expiry is still money —
 * somebody can discount it, move it to the busier branch, or push it at the
 * counter. Expired is already a loss, and the only thing left to do is take it
 * off the shelf and say so.
 */
export default function ExpiryPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [branch, setBranch] = useState('');

  const { data: branches } = useQuery({
    queryKey: ['branches'],
    queryFn: api.admin.branches.list,
  });
  const { data: report, isLoading, error } = useQuery({
    queryKey: ['inventory', 'expiry', branch],
    queryFn: () => api.admin.inventory.expiry(branch || undefined),
  });

  const writeOff = useMutation({
    mutationFn: (batchId: string) => api.admin.inventory.writeOffBatch(batchId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['inventory'] });
    },
  });

  return (
    <div>
      <PageTitle
        title="Dates"
        subtitle="What is about to go off, and what it will cost if it does"
      />
      <QueryError error={error} />

      <div className="mb-4 max-w-xs">
        <SearchSelect
          value={branch}
          onChange={setBranch}
          allowEmpty
          emptyLabel="Every branch"
          placeholder="Search branch…"
          options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
        />
      </div>

      {isLoading || !report ? (
        <Spinner label="Checking dates…" />
      ) : (
        <>
          <div className="mb-4 grid gap-3 sm:grid-cols-4">
            <StatCard label="Near expiry" value={report.summary.nearBatches} tone="brand" />
            <StatCard
              label="Still worth"
              value={formatIdr(report.summary.nearValueIdr)}
              hint="Discount it, or move it"
            />
            <StatCard label="Expired" value={report.summary.expiredBatches} tone="danger" />
            <StatCard
              label="Already lost"
              value={formatIdr(report.summary.expiredValueIdr)}
              hint="Waiting to be written off"
            />
          </div>

          <BatchTable
            title="Going off soon"
            hint="In date order. Each item is judged against its own warning window."
            rows={report.near}
            empty="Nothing is close to its date."
          />

          <BatchTable
            title="Past their date"
            hint="These cannot be sold. Writing one off is an adjustment carrying a reason."
            rows={report.expired}
            empty="Nothing has expired."
            onWriteOff={can('inventory.count') ? (id) => writeOff.mutate(id) : undefined}
            writingOff={writeOff.isPending}
          />
        </>
      )}
    </div>
  );
}

function BatchTable({
  title,
  hint,
  rows,
  empty,
  onWriteOff,
  writingOff,
}: {
  title: string;
  hint: string;
  rows: Array<{
    id: string;
    batchCode: string;
    itemName: string;
    itemSku: string;
    branchName: string;
    unit: string;
    qtyOnHand: number;
    expiresOn: string | null;
    daysLeft: number | null;
    valueIdr: number;
    state: keyof typeof EXPIRY_LABELS;
  }>;
  empty: string;
  onWriteOff?: (batchId: string) => void;
  writingOff?: boolean;
}) {
  return (
    <div className="a-card mb-4 !p-0">
      <div className="px-4 pt-4">
        <h2 className="font-black">{title}</h2>
        <p className="text-xs text-muted">{hint}</p>
      </div>
      <div className="overflow-x-auto">
        <table className="a-table mt-2">
          <thead>
            <tr>
              <th>Item</th>
              <th>Batch</th>
              <th>Branch</th>
              <th className="text-right">On hand</th>
              <th>Expires</th>
              <th className="text-right">Value</th>
              {onWriteOff ? <th /> : null}
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.id}>
                <td>
                  <p className="font-bold">{row.itemName}</p>
                  <p className="text-xs text-muted">{row.itemSku}</p>
                </td>
                <td className="font-mono text-xs">{row.batchCode}</td>
                <td className="text-sm text-muted">{row.branchName}</td>
                <td className="text-right tabular-nums">
                  {row.qtyOnHand} {row.unit.toLowerCase()}
                </td>
                <td className="text-sm">
                  {row.expiresOn ?? '—'}
                  {row.daysLeft != null ? (
                    <span
                      className={`ml-2 text-xs ${row.daysLeft < 0 ? 'text-danger' : 'text-muted'}`}
                    >
                      {row.daysLeft < 0
                        ? `${-row.daysLeft} days ago`
                        : `${row.daysLeft} days left`}
                    </span>
                  ) : null}
                </td>
                <td className="text-right tabular-nums">{formatIdr(row.valueIdr)}</td>
                {onWriteOff ? (
                  <td className="text-right">
                    <button
                      className="a-btn-ghost !px-2 !py-1 text-xs"
                      disabled={writingOff}
                      onClick={() => onWriteOff(row.id)}
                    >
                      Write off
                    </button>
                  </td>
                ) : null}
              </tr>
            ))}
            {rows.length === 0 ? (
              <tr>
                <td colSpan={onWriteOff ? 7 : 6} className="py-8 text-center text-sm text-muted">
                  {empty}
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </div>
  );
}
