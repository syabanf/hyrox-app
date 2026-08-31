'use client';

import { Spinner, StatusBadge, formatDay } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { useState } from 'react';
import { api } from '../../../lib/api';
import { PageTitle } from '../../../components/ui';

const STATUSES = ['', 'ACTIVE', 'SUSPENDED', 'INACTIVE', 'ARCHIVED'];

export default function MembersPage() {
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState('');
  const { data, isLoading } = useQuery({
    queryKey: ['members', query, status],
    queryFn: () => api.admin.members.list({ query: query || undefined, status: status || undefined }),
  });

  return (
    <div>
      <PageTitle title="Members" subtitle={`${data?.length ?? '…'} members`} />
      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search name, email, phone…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <select className="a-input max-w-40" value={status} onChange={(e) => setStatus(e.target.value)}>
          {STATUSES.map((s) => (
            <option key={s} value={s}>
              {s || 'All statuses'}
            </option>
          ))}
        </select>
      </div>
      {isLoading ? (
        <Spinner label="Loading members…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>Member</th>
                <th>Contact</th>
                <th>Status</th>
                <th className="text-right">Balance</th>
                <th className="text-right">Visits</th>
                <th>Last visit</th>
              </tr>
            </thead>
            <tbody>
              {(data ?? []).map((m) => (
                <tr key={m.member.id}>
                  <td>
                    <Link href={`/members/${m.member.id}`} className="font-bold hover:text-brand">
                      {m.member.fullName}
                    </Link>
                  </td>
                  <td className="text-muted">
                    {m.member.email}
                    <br />
                    {m.member.phone}
                  </td>
                  <td>
                    <StatusBadge status={m.member.status} />
                  </td>
                  <td className="text-right font-black text-brand">{m.balance}</td>
                  <td className="text-right">{m.totalVisits}</td>
                  <td className="text-muted">{m.lastVisitAt ? formatDay(m.lastVisitAt) : '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
