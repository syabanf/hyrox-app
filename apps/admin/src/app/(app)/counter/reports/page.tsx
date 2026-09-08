'use client';

import { formatIdr, Spinner } from '@nuhabit/ui';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { PageTitle, QueryError, SearchSelect, StatCard } from '../../../../components/ui';
import { api } from '../../../../lib/api';

/**
 * What the counter did.
 *
 * Everything here reads completed sales only. A voided sale is money that came
 * in and went out again, and it has its own section rather than quietly
 * inflating the takings.
 */
export default function TillReportsPage() {
  const [branch, setBranch] = useState('');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const window = { branchId: branch || undefined, from: from || undefined, to: to || undefined };

  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.admin.branches.list });
  const { data: revenue, error } = useQuery({
    queryKey: ['pos', 'revenue', window],
    queryFn: () => api.admin.pos.reports.revenue(window),
  });
  const { data: products } = useQuery({
    queryKey: ['pos', 'product-sales', window],
    queryFn: () => api.admin.pos.reports.productSales(window),
  });
  const { data: rush } = useQuery({
    queryKey: ['pos', 'rush', window],
    queryFn: () => api.admin.pos.reports.rushHour(window),
  });
  const { data: voids } = useQuery({
    queryKey: ['pos', 'voids', window],
    queryFn: () => api.admin.pos.reports.voids(window),
  });

  const profit = (products ?? []).reduce((s, p) => s + p.profitIdr, 0);
  const voided = (voids ?? []).reduce((s, o) => s + o.totalIdr, 0);

  return (
    <div>
      <PageTitle title="Till reports" subtitle="What sold, when, and what was made on it" />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <SearchSelect
          value={branch}
          onChange={setBranch}
          allowEmpty
          emptyLabel="Every branch"
          placeholder="Search branch…"
          options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
        />
        <input className="a-input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
        <input className="a-input" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
      </div>

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard label="Takings" value={formatIdr(revenue?.totalIdr ?? 0)} tone="brand" />
        <StatCard label="Gross profit" value={formatIdr(profit)} />
        <StatCard
          label="Busiest hour"
          value={rush ? `${String(rush.busiestHour).padStart(2, '0')}:00` : '—'}
          hint={rush ? `${rush.peakShare}% of takings` : undefined}
        />
        <StatCard
          label="Voided"
          value={formatIdr(voided)}
          tone={voided > 0 ? 'danger' : undefined}
          hint="Not counted as takings"
        />
      </div>

      {!revenue ? (
        <Spinner label="Adding it up…" />
      ) : (
        <>
          <div className="mb-4 grid gap-4 lg:grid-cols-3">
            <BucketCard title="By category" buckets={revenue.byCategory} />
            <BucketCard title="By channel" buckets={revenue.byChannel} />
            <BucketCard title="By payment" buckets={revenue.byMethod} />
          </div>

          {rush ? (
            <div className="a-card mb-4">
              <h2 className="mb-1 font-black">Through the day</h2>
              <p className="mb-3 text-xs text-muted">
                Every hour, including the quiet ones. A gap here would read as missing data
                rather than as a quiet afternoon.
              </p>
              <div className="flex h-32 items-end gap-1">
                {rush.hours.map((hour) => (
                  <div key={hour.key} className="flex flex-1 flex-col items-center gap-1">
                    <div
                      className={`w-full rounded-t ${
                        Number(hour.key) === rush.busiestHour ? 'bg-brand' : 'bg-line'
                      }`}
                      style={{ height: `${Math.max(hour.share * 3, hour.salesIdr > 0 ? 4 : 1)}%` }}
                      title={`${hour.label} — ${formatIdr(hour.salesIdr)}`}
                    />
                    <span className="text-[9px] text-muted">{hour.key}</span>
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          <div className="a-card !p-0">
            <div className="px-4 pt-4">
              <h2 className="font-black">What sold</h2>
              <p className="text-xs text-muted">
                Ranked by what each contributed, not by margin — a high margin on something
                nobody buys is not the thing to stock more of.
              </p>
            </div>
            <div className="overflow-x-auto">
              <table className="a-table mt-2">
                <thead>
                  <tr>
                    <th>Product</th>
                    <th className="text-right">Sold</th>
                    <th className="text-right">Takings</th>
                    <th className="text-right">Cost</th>
                    <th className="text-right">Profit</th>
                    <th className="text-right">Margin</th>
                  </tr>
                </thead>
                <tbody>
                  {(products ?? []).map((line) => (
                    <tr key={line.productId}>
                      <td className="font-bold">{line.productName}</td>
                      <td className="text-right tabular-nums">{line.qty}</td>
                      <td className="text-right tabular-nums">{formatIdr(line.salesIdr)}</td>
                      <td className="text-right tabular-nums text-sm text-muted">
                        {formatIdr(line.costIdr)}
                      </td>
                      <td className="text-right tabular-nums font-bold">{formatIdr(line.profitIdr)}</td>
                      <td className="text-right tabular-nums text-sm">{line.marginPercent}%</td>
                    </tr>
                  ))}
                  {(products ?? []).length === 0 ? (
                    <tr>
                      <td colSpan={6} className="py-8 text-center text-sm text-muted">
                        Nothing sold in that window.
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </div>
        </>
      )}
    </div>
  );
}

function BucketCard({
  title,
  buckets,
}: {
  title: string;
  buckets: Array<{ key: string; label: string; salesIdr: number; share: number }>;
}) {
  return (
    <div className="a-card">
      <h2 className="mb-3 font-black">{title}</h2>
      <div className="grid gap-2">
        {buckets.map((bucket) => (
          <div key={bucket.key}>
            <div className="flex items-baseline justify-between text-sm">
              <span className="font-bold">{bucket.label}</span>
              <span className="tabular-nums text-muted">
                {formatIdr(bucket.salesIdr)} · {bucket.share}%
              </span>
            </div>
            <div className="mt-1 h-1.5 rounded-full bg-line">
              <div className="h-full rounded-full bg-brand" style={{ width: `${bucket.share}%` }} />
            </div>
          </div>
        ))}
        {buckets.length === 0 ? <p className="text-sm text-muted">Nothing yet.</p> : null}
      </div>
    </div>
  );
}
