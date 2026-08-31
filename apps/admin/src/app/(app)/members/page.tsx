'use client';

import { Spinner, StatusBadge, formatDay } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { api, ApiError } from '../../../lib/api';
import { usePermissions } from '../../../lib/auth';
import { ErrorNote, Modal, PageTitle, SearchSelect } from '../../../components/ui';

const STATUSES = ['', 'ACTIVE', 'SUSPENDED', 'INACTIVE', 'ARCHIVED'];

export default function MembersPage() {
  const { can } = usePermissions();
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const { data, isLoading } = useQuery({
    queryKey: ['members', query, status],
    queryFn: () => api.admin.members.list({ query: query || undefined, status: status || undefined }),
  });

  return (
    <div>
      <PageTitle
        title="Members"
        subtitle={`${data?.length ?? '…'} members`}
        actions={
          can('members.manage') ? (
            <button className="a-btn" onClick={() => setCreateOpen(true)}>
              + New member
            </button>
          ) : undefined
        }
      />
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
      {createOpen ? <CreateMemberModal onClose={() => setCreateOpen(false)} /> : null}
    </div>
  );
}

function CreateMemberModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const router = useRouter();
  const [fullName, setFullName] = useState('');
  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [preferredBranchId, setPreferredBranchId] = useState('');
  const [notes, setNotes] = useState('');
  const [error, setError] = useState<string | null>(null);
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const mutation = useMutation({
    mutationFn: () =>
      api.admin.members.create({
        fullName,
        email,
        phone,
        preferredBranchId: preferredBranchId || null,
        notes: notes.trim() === '' ? null : notes.trim(),
      }),
    onSuccess: (detail) => {
      void qc.invalidateQueries({ queryKey: ['members'] });
      router.push(`/members/${detail.member.id}`);
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Create failed.'),
  });

  return (
    <Modal title="New member" onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Full name</label>
          <input className="a-input" value={fullName} onChange={(e) => setFullName(e.target.value)} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Email</label>
            <input className="a-input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
          </div>
          <div>
            <label className="a-label">Phone</label>
            <input className="a-input" value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+62…" />
          </div>
        </div>
        <div>
          <label className="a-label">Preferred branch</label>
          <SearchSelect
            value={preferredBranchId}
            onChange={setPreferredBranchId}
            allowEmpty
            emptyLabel="None"
            placeholder="Search branch…"
            options={(branches ?? []).map((b) => ({ value: b.id, label: b.name }))}
          />
        </div>
        <div>
          <label className="a-label">Notes (optional)</label>
          <input className="a-input" value={notes} onChange={(e) => setNotes(e.target.value)} />
        </div>
        <ErrorNote message={error} />
        <button
          className="a-btn"
          disabled={mutation.isPending || fullName.length < 2 || !email.includes('@') || phone.length < 6}
          onClick={() => mutation.mutate()}
        >
          Create member
        </button>
        <p className="text-xs text-muted">
          The member signs in with this email via OTP. The wallet starts at zero — record a top-up or
          adjustment from their profile.
        </p>
      </div>
    </Modal>
  );
}
