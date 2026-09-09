'use client';

import { Spinner, StatusBadge, formatDay } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { api, ApiError } from '../../../lib/api';
import { usePermissions } from '../../../lib/auth';
import { Eye } from 'lucide-react';
import { ErrorNote, Modal, PageTitle, Pager, RowActions, SearchSelect, StatCard } from '../../../components/ui';
import { FilterBar, FilterSelect, useFilters } from '../../../components/filters';

const STATUSES = ['', 'ACTIVE', 'SUSPENDED', 'INACTIVE', 'ARCHIVED'];

const MEMBER_FILTERS = { q: '', status: '' };

export default function MembersPage() {
  const { can } = usePermissions();
  const { filters, set, clear, dirty } = useFilters(MEMBER_FILTERS);
  const { q: query, status } = filters;
  const [createOpen, setCreateOpen] = useState(false);
  const [page, setPage] = useState(0);
  const { data, isLoading } = useQuery({
    // NOT_ACTIVE is not a member status; it is what the "suspended / inactive"
    // card counts as one number, so it is applied here rather than sent to a
    // server that would rightly not recognise it.
    queryKey: ['members', query, status],
    queryFn: () =>
      api.admin.members.list({
        query: query || undefined,
        status: status && status !== 'NOT_ACTIVE' ? status : undefined,
      }),
  });

  const rows =
    status === 'NOT_ACTIVE'
      ? (data ?? []).filter((m) => m.member.status !== 'ACTIVE')
      : (data ?? []);
  const pageCount = Math.max(1, Math.ceil(rows.length / 10));
  const safePage = Math.min(page, pageCount - 1);

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
      <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {/* Each card filters the table to what it counts. */}
        {/* No `active`: this card clears the filter rather than being one, and
            a permanent "filtering" ring on the default view says nothing. */}
        <StatCard tone="ink" label="Members" value={(data ?? []).length} onClick={() => set('status', '')} />
        <StatCard
          tone="brand"
          label="Active"
          value={(data ?? []).filter((m) => m.member.status === 'ACTIVE').length}
          active={status === 'ACTIVE'}
          onClick={() => set('status', status === 'ACTIVE' ? '' : 'ACTIVE')}
        />
        <StatCard
          tone="warn"
          label="Suspended / inactive"
          value={(data ?? []).filter((m) => m.member.status !== 'ACTIVE').length}
          active={status === 'NOT_ACTIVE'}
          onClick={() => set('status', status === 'NOT_ACTIVE' ? '' : 'NOT_ACTIVE')}
        />
        <StatCard
          tone="brand"
          label="Credits held"
          value={(data ?? []).reduce((sum, m) => sum + m.balance, 0)}
          hint="Outstanding across listed members"
        />
      </div>
      <FilterBar
        dirty={dirty}
        onClear={clear}
        chips={[
          ...(status
            ? [{
                key: 'status',
                label: status === 'NOT_ACTIVE' ? 'Suspended or inactive' : status,
                onRemove: () => set('status', ''),
              }]
            : []),
          ...(query ? [{ key: 'q', label: `"${query}"`, onRemove: () => set('q', '') }] : []),
        ]}
      >
        <input
          className="a-input max-w-xs"
          placeholder="Search name, email, phone…"
          value={query}
          onChange={(e) => {
            set('q', e.target.value);
            setPage(0);
          }}
        />
        <FilterSelect
          value={status}
          onChange={(v) => {
            set('status', v);
            setPage(0);
          }}
          emptyLabel="All statuses"
          options={STATUSES.filter(Boolean).map((s) => ({ value: s, label: s }))}
        />
        <span className="text-xs text-muted">
          {rows.length} {rows.length === 1 ? 'member' : 'members'}
        </span>
      </FilterBar>
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
                <th />
              </tr>
            </thead>
            <tbody>
              {rows
                .slice(safePage * 10, safePage * 10 + 10)
                .map((m) => (
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
                  <td className="text-muted">{m.lastVisitAt ? formatDay(m.lastVisitAt) : '-'}</td>
                  <td className="text-right">
                    <RowActions
                      items={[
                        {
                          label: 'Open 360°',
                          icon: Eye,
                          onClick: () => (location.href = `/admin/members/${m.member.id}`),
                        },
                      ]}
                    />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pager page={safePage} pageCount={pageCount} onPage={setPage} />
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
          The member signs in with this email via OTP. The wallet starts at zero - record a top-up or
          adjustment from their profile.
        </p>
      </div>
    </Modal>
  );
}
