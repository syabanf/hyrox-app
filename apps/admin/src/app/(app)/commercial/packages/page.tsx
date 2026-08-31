'use client';

import type { CreditPackage } from '@hyrox/domain';
import { Spinner, StatusBadge, formatIdr } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, Modal, PageTitle } from '../../../../components/ui';

export default function PackagesPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<CreditPackage | 'new' | null>(null);
  const { data, isLoading } = useQuery({ queryKey: ['admin-packages'], queryFn: api.admin.packages.list });

  const [error, setError] = useState<string | null>(null);
  const archive = useMutation({
    mutationFn: (id: string) => api.admin.packages.update(id, { status: 'ARCHIVED' }),
    onSuccess: () => void qc.invalidateQueries(),
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.admin.packages.remove(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Delete failed.'),
  });

  return (
    <div>
      <PageTitle
        title="Credit Packages"
        subtitle="Used packages are archived, never deleted"
        actions={
          can('packages.manage') ? (
            <button className="a-btn" onClick={() => setEditing('new')}>
              + New package
            </button>
          ) : undefined
        }
      />
      <ErrorNote message={error} />
      {isLoading ? (
        <Spinner label="Loading packages…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Package</th>
                <th className="text-right">Credits</th>
                <th className="text-right">Price</th>
                <th className="text-right">Validity</th>
                <th className="text-right">Purchases</th>
                <th className="text-right">Revenue</th>
                <th>Status</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {(data ?? []).map(({ pkg, purchaseCount, revenueIdr }) => (
                <tr key={pkg.id}>
                  <td className="font-bold">{pkg.name}</td>
                  <td className="text-right font-black text-brand">{pkg.credits}</td>
                  <td className="text-right">{formatIdr(pkg.priceIdr)}</td>
                  <td className="text-right">{pkg.validityDays} days</td>
                  <td className="text-right">{purchaseCount}</td>
                  <td className="text-right font-bold">{formatIdr(revenueIdr)}</td>
                  <td>
                    <StatusBadge status={pkg.status} />
                  </td>
                  <td className="text-right">
                    {can('packages.manage') ? (
                      <div className="flex justify-end gap-2">
                        <button className="text-sm font-bold text-brand" onClick={() => setEditing(pkg)}>
                          Edit
                        </button>
                        {pkg.status === 'ACTIVE' ? (
                          <button
                            className="text-sm font-bold text-muted hover:text-danger"
                            onClick={() => archive.mutate(pkg.id)}
                          >
                            Archive
                          </button>
                        ) : null}
                        <button
                          className="text-sm font-bold text-muted hover:text-danger"
                          onClick={() => {
                            if (confirm(`Delete package "${pkg.name}"? Only possible while unused.`))
                              remove.mutate(pkg.id);
                          }}
                        >
                          Delete
                        </button>
                      </div>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editing ? (
        <PackageModal
          pkg={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries();
          }}
        />
      ) : null}
    </div>
  );
}

function PackageModal({ pkg, onClose, onDone }: { pkg: CreditPackage | null; onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState(pkg?.name ?? '');
  const [credits, setCredits] = useState(String(pkg?.credits ?? 10));
  const [price, setPrice] = useState(String(pkg?.priceIdr ?? 1_500_000));
  const [validity, setValidity] = useState(String(pkg?.validityDays ?? 60));
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      const body = {
        name,
        credits: Number(credits),
        priceIdr: Number(price),
        validityDays: Number(validity),
        branchId: null,
        purchaseLimitPerMember: null,
        status: 'ACTIVE' as const,
      };
      return pkg ? api.admin.packages.update(pkg.id, body) : api.admin.packages.create(body);
    },
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });

  return (
    <Modal title={pkg ? `Edit ${pkg.name}` : 'New package'} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Name</label>
          <input className="a-input" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="grid grid-cols-3 gap-3">
          <div>
            <label className="a-label">Credits</label>
            <input className="a-input" value={credits} onChange={(e) => setCredits(e.target.value)} />
          </div>
          <div>
            <label className="a-label">Price (IDR)</label>
            <input className="a-input" value={price} onChange={(e) => setPrice(e.target.value)} />
          </div>
          <div>
            <label className="a-label">Validity (days)</label>
            <input className="a-input" value={validity} onChange={(e) => setValidity(e.target.value)} />
          </div>
        </div>
        <ErrorNote message={error} />
        <button className="a-btn" disabled={mutation.isPending || name.length < 2} onClick={() => mutation.mutate()}>
          {pkg ? 'Save changes' : 'Create package'}
        </button>
      </div>
    </Modal>
  );
}
