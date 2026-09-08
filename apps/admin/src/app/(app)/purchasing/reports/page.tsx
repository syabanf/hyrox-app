'use client';

import { formatIdr, Spinner } from '@nuhabit/ui';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { PageTitle, QueryError, SearchSelect, StatCard } from '../../../../components/ui';
import { api } from '../../../../lib/api';

/**
 * What buying cost, and how suppliers actually behaved.
 *
 * The supplier table ranks on behaviour rather than on price, because a
 * supplier who is cheap and short is worse than one who is dear and complete:
 * stock that never arrived cannot be sold at any price.
 */
export default function PurchasingReportsPage() {
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [item, setItem] = useState('');
  const window = { from: from || undefined, to: to || undefined };

  const { data: summary, error } = useQuery({
    queryKey: ['purchasing', 'summary', window],
    queryFn: () => api.admin.purchasing.reports.orders(window),
  });
  const { data: suppliers } = useQuery({
    queryKey: ['purchasing', 'supplier-performance', window],
    queryFn: () => api.admin.purchasing.reports.suppliers(window),
  });
  const { data: items } = useQuery({
    queryKey: ['inventory', 'items', 'all'],
    queryFn: () => api.admin.inventory.items.list(),
  });
  const { data: history } = useQuery({
    queryKey: ['purchasing', 'price-history', item],
    queryFn: () => api.admin.purchasing.reports.priceHistory(item),
    enabled: Boolean(item),
  });
  const { data: valuation } = useQuery({
    queryKey: ['inventory', 'valuation'],
    queryFn: () => api.admin.inventory.reports.valuation(),
  });

  return (
    <div>
      <PageTitle title="Buying reports" subtitle="What it cost, and who delivered it properly" />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-2">
        <input className="a-input" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
        <input className="a-input" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
      </div>

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard label="Orders" value={summary?.orders ?? 0} />
        <StatCard label="Committed" value={formatIdr(summary?.totalIdr ?? 0)} tone="brand" />
        <StatCard label="Arrived" value={formatIdr(summary?.receivedIdr ?? 0)} />
        <StatCard
          label="Still owed"
          value={formatIdr(summary?.outstandingIdr ?? 0)}
          hint="Committed to but not yet arrived"
        />
      </div>

      <div className="a-card mb-4 !p-0">
        <div className="px-4 pt-4">
          <h2 className="font-black">How suppliers behaved</h2>
          <p className="text-xs text-muted">
            Scored on delivery rather than on price. Fill rate weighs heaviest — goods that
            never arrived cannot be sold at any price.
          </p>
        </div>
        <div className="overflow-x-auto">
          <table className="a-table mt-2">
            <thead>
              <tr>
                <th>Supplier</th>
                <th className="text-right">Orders</th>
                <th className="text-right">Spend</th>
                <th className="text-right">Arrived in full</th>
                <th className="text-right">On time</th>
                <th className="text-right">Rejected</th>
                <th className="text-right">Lead time</th>
                <th className="text-right">Score</th>
              </tr>
            </thead>
            <tbody>
              {(suppliers ?? []).map((supplier) => (
                <tr key={supplier.supplierId}>
                  <td className="font-bold">{supplier.supplierName}</td>
                  <td className="text-right tabular-nums">{supplier.orders}</td>
                  <td className="text-right tabular-nums">{formatIdr(supplier.spendIdr)}</td>
                  <td className="text-right tabular-nums">{supplier.fillRate}%</td>
                  <td className="text-right tabular-nums">{supplier.onTimeRate}%</td>
                  <td
                    className={`text-right tabular-nums ${
                      supplier.rejectRate > 5 ? 'font-bold text-danger' : 'text-muted'
                    }`}
                  >
                    {supplier.rejectRate}%
                  </td>
                  <td className="text-right tabular-nums text-sm text-muted">
                    {supplier.averageLeadDays} days
                  </td>
                  <td className="text-right tabular-nums font-black">{supplier.score}</td>
                </tr>
              ))}
              {(suppliers ?? []).length === 0 ? (
                <tr>
                  <td colSpan={8} className="py-8 text-center text-sm text-muted">
                    Nothing bought in that window.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <div className="a-card">
          <h2 className="mb-1 font-black">What an item has actually cost</h2>
          <p className="mb-3 text-xs text-muted">
            Read from receipts, not price lists: a quoted price is a promise and a received
            price is a fact.
          </p>
          <div className="mb-3">
            <SearchSelect
              value={item}
              onChange={setItem}
              allowEmpty
              emptyLabel="Pick an item"
              placeholder="Search item…"
              options={(items ?? []).map((i) => ({ value: i.id, label: i.name, hint: i.sku }))}
            />
          </div>
          {item ? (
            <table className="a-table">
              <thead>
                <tr>
                  <th>Received</th>
                  <th>Supplier</th>
                  <th className="text-right">Per pack</th>
                  <th className="text-right">Per unit</th>
                </tr>
              </thead>
              <tbody>
                {(history ?? []).map((entry) => (
                  <tr key={entry.grnNumber}>
                    <td className="text-sm">{entry.receivedOn}</td>
                    <td className="text-sm">{entry.supplierName}</td>
                    <td className="text-right tabular-nums text-sm">
                      {formatIdr(entry.packPriceIdr)}
                      <span className="ml-1 text-xs text-muted">/{entry.unit.toLowerCase()}</span>
                    </td>
                    <td className="text-right tabular-nums font-bold">
                      {formatIdr(entry.unitPriceIdr)}
                    </td>
                  </tr>
                ))}
                {(history ?? []).length === 0 ? (
                  <tr>
                    <td colSpan={4} className="py-6 text-center text-sm text-muted">
                      Never received.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          ) : null}
        </div>

        <div className="a-card">
          <h2 className="mb-1 font-black">What the shelf is worth</h2>
          <p className="mb-3 text-xs text-muted">
            At weighted-average cost — what the stock on hand actually cost, not the last
            price paid.
          </p>
          {!valuation ? (
            <Spinner label="Valuing…" />
          ) : (
            <>
              <div className="mb-3 grid gap-3 sm:grid-cols-2">
                <StatCard label="Total" value={formatIdr(valuation.totalIdr)} tone="brand" />
                <StatCard
                  label="In the top five lines"
                  value={`${valuation.concentration}%`}
                  hint="Where the capital actually sits"
                />
              </div>
              <table className="a-table">
                <thead>
                  <tr>
                    <th>Item</th>
                    <th className="text-right">On hand</th>
                    <th className="text-right">Worth</th>
                    <th className="text-right">Share</th>
                  </tr>
                </thead>
                <tbody>
                  {valuation.lines.slice(0, 10).map((line) => (
                    <tr key={line.itemId}>
                      <td>
                        <p className="font-bold">{line.name}</p>
                        <p className="text-xs text-muted">{line.sku}</p>
                      </td>
                      <td className="text-right tabular-nums">
                        {line.qtyOnHand} {line.unit.toLowerCase()}
                      </td>
                      <td className="text-right tabular-nums">{formatIdr(line.valueIdr)}</td>
                      <td className="text-right tabular-nums text-sm text-muted">{line.share}%</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
