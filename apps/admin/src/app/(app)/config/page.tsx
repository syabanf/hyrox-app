'use client';

import type { AdminRole, AdminUser, Branch, BusinessRules } from '@hyrox/domain';
import { ADMIN_ROLES, PERMISSIONS, ROLE_PERMISSIONS } from '@hyrox/domain';
import { Spinner, StatusBadge, formatDayTime } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../lib/api';
import { usePermissions } from '../../../lib/auth';
import { ErrorNote, Modal, PageTitle } from '../../../components/ui';

const TABS = ['Business Rules', 'Branches & Gates', 'Users', 'Roles', 'Audit Trail'] as const;
type Tab = (typeof TABS)[number];

export default function ConfigPage() {
  const [tab, setTab] = useState<Tab>('Business Rules');
  return (
    <div>
      <PageTitle title="Configuration" subtitle="Policies live here as data — never hard-coded" />
      <div className="mb-4 flex flex-wrap gap-1 rounded-xl bg-surface p-1">
        {TABS.map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`rounded-lg px-3.5 py-1.5 text-sm font-bold ${
              tab === t ? 'bg-brand text-white' : 'text-muted hover:text-ink'
            }`}
          >
            {t}
          </button>
        ))}
      </div>
      {tab === 'Business Rules' ? <RulesTab /> : null}
      {tab === 'Branches & Gates' ? <BranchesTab /> : null}
      {tab === 'Users' ? <UsersTab /> : null}
      {tab === 'Roles' ? <RolesTab /> : null}
      {tab === 'Audit Trail' ? <AuditTab /> : null}
    </div>
  );
}

const RULE_FIELDS: { key: keyof BusinessRules; label: string; kind: 'number' | 'select'; options?: string[] }[] = [
  { key: 'openGymCreditCost', label: 'Open gym credit cost', kind: 'number' },
  { key: 'defaultCreditExpiryDays', label: 'Default credit expiry (days)', kind: 'number' },
  { key: 'cancellationDeadlineHours', label: 'Cancellation deadline (hours before)', kind: 'number' },
  { key: 'lateCancellationPolicy', label: 'Late cancellation policy', kind: 'select', options: ['FORFEIT', 'FREE'] },
  { key: 'noShowPolicy', label: 'No-show policy', kind: 'select', options: ['FORFEIT', 'FREE'] },
  { key: 'reEntryGraceMinutes', label: 'Re-entry grace (minutes)', kind: 'number' },
  { key: 'antiPassbackMinutes', label: 'Anti-passback window (minutes)', kind: 'number' },
  { key: 'qrTtlSeconds', label: 'QR expiration (seconds)', kind: 'number' },
  { key: 'lowBalanceThreshold', label: 'Low balance threshold', kind: 'number' },
  { key: 'expiryReminderDays', label: 'Expiry reminder window (days)', kind: 'number' },
  { key: 'bookingOpensDaysBefore', label: 'Booking opens (days before)', kind: 'number' },
  { key: 'bookingClosesMinutesBefore', label: 'Booking closes (minutes before)', kind: 'number' },
];

function RulesTab() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const canEdit = can('rules.update');
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const { data, isLoading } = useQuery({ queryKey: ['rules'], queryFn: api.admin.rules.get });

  const save = useMutation({
    mutationFn: () => {
      const patch: Record<string, unknown> = {};
      for (const [k, v] of Object.entries(draft)) {
        const field = RULE_FIELDS.find((f) => f.key === k);
        patch[k] = field?.kind === 'number' ? Number(v) : v;
      }
      return api.admin.rules.update(patch);
    },
    onSuccess: () => {
      setDraft({});
      setSaved(true);
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });

  if (isLoading || !data) return <Spinner label="Loading rules…" />;

  const valueOf = (key: keyof BusinessRules): string =>
    draft[key] !== undefined ? draft[key]! : String(data.defaults[key]);

  return (
    <div className="flex max-w-3xl flex-col gap-4">
      {!canEdit ? (
        <p className="rounded-lg bg-warn/10 px-3 py-2 text-sm font-bold text-warn">
          Read-only — only Super Admin can change business rules (and the server enforces it).
        </p>
      ) : null}
      <div className="a-card grid gap-4 sm:grid-cols-2">
        {RULE_FIELDS.map((f) => (
          <div key={f.key}>
            <label className="a-label">{f.label}</label>
            {f.kind === 'select' ? (
              <select
                className="a-input"
                disabled={!canEdit}
                value={valueOf(f.key)}
                onChange={(e) => {
                  setDraft((d) => ({ ...d, [f.key]: e.target.value }));
                  setSaved(false);
                }}
              >
                {f.options!.map((o) => (
                  <option key={o}>{o}</option>
                ))}
              </select>
            ) : (
              <input
                className="a-input"
                disabled={!canEdit}
                value={valueOf(f.key)}
                onChange={(e) => {
                  setDraft((d) => ({ ...d, [f.key]: e.target.value }));
                  setSaved(false);
                }}
              />
            )}
          </div>
        ))}
      </div>
      {data.branchOverrides.length > 0 ? (
        <div className="a-card text-sm">
          <p className="a-label">Branch overrides</p>
          {data.branchOverrides.map((o) => (
            <p key={o.branchId} className="text-muted">
              <span className="font-bold text-ink">{o.branchName}:</span>{' '}
              {Object.entries(o.override)
                .map(([k, v]) => `${k} = ${String(v)}`)
                .join(', ')}
            </p>
          ))}
        </div>
      ) : null}
      <ErrorNote message={error} />
      {saved ? <p className="text-sm font-bold text-ok">Rules saved — changes apply immediately (audited).</p> : null}
      {canEdit ? (
        <button
          className="a-btn self-start"
          disabled={save.isPending || Object.keys(draft).length === 0}
          onClick={() => save.mutate()}
        >
          Save business rules
        </button>
      ) : null}
    </div>
  );
}

function BranchesTab() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [branchModal, setBranchModal] = useState<'new' | { id: string } | null>(null);
  const [gateBranch, setGateBranch] = useState<string | null>(null);
  const { data: branches, isLoading } = useQuery({ queryKey: ['admin-branches'], queryFn: api.admin.branches.list });
  const { data: gates } = useQuery({ queryKey: ['gates'], queryFn: api.admin.gates.list });
  const refresh = () => void qc.invalidateQueries();

  const toggleGate = useMutation({
    mutationFn: ({ id, status }: { id: string; status: 'ONLINE' | 'OFFLINE' }) =>
      api.admin.gates.update(id, { status }),
    onSuccess: refresh,
  });

  const [error, setError] = useState<string | null>(null);
  const onError = (e: unknown) => setError(e instanceof ApiError ? e.message : 'Delete failed.');
  const removeBranch = useMutation({
    mutationFn: (id: string) => api.admin.branches.remove(id),
    onSuccess: () => {
      setError(null);
      refresh();
    },
    onError,
  });
  const removeGate = useMutation({
    mutationFn: (id: string) => api.admin.gates.remove(id),
    onSuccess: () => {
      setError(null);
      refresh();
    },
    onError,
  });

  if (isLoading) return <Spinner label="Loading branches…" />;
  return (
    <div className="flex flex-col gap-3">
      <ErrorNote message={error} />
      {can('branches.manage') ? (
        <button className="a-btn self-start" onClick={() => setBranchModal('new')}>
          + New branch
        </button>
      ) : null}
      <div className="grid gap-3 lg:grid-cols-2">
        {(branches ?? []).map((b) => (
          <div key={b.id} className="a-card">
            <div className="flex items-center justify-between">
              <p className="font-black">{b.name}</p>
              <div className="flex items-center gap-2">
                <StatusBadge status={b.status} />
                {can('branches.manage') ? (
                  <>
                    <button className="text-sm font-bold text-brand" onClick={() => setBranchModal({ id: b.id })}>
                      Edit
                    </button>
                    <button
                      className="text-sm font-bold text-muted hover:text-danger"
                      onClick={() => {
                        if (confirm(`Delete branch "${b.name}"? Gates and sessions must be removed first.`))
                          removeBranch.mutate(b.id);
                      }}
                    >
                      Delete
                    </button>
                  </>
                ) : null}
              </div>
            </div>
            <p className="text-sm text-muted">{b.address}</p>
            <p className="text-sm text-muted">
              {b.operatingHours} · {b.timezone} · Manager: {b.managerName ?? '—'}
            </p>
            <div className="mt-3 flex flex-wrap items-center gap-2">
              {(gates ?? [])
                .filter((g) => g.branchId === b.id)
                .map((g) => (
                  <span key={g.id} className="flex items-center gap-1.5 rounded-lg border border-line px-2.5 py-1 text-xs font-bold">
                    {g.name} <StatusBadge status={g.status} />
                    {can('gates.manage') ? (
                      <>
                        <button
                          className="text-[10px] font-black uppercase text-muted hover:text-brand"
                          onClick={() =>
                            toggleGate.mutate({ id: g.id, status: g.status === 'ONLINE' ? 'OFFLINE' : 'ONLINE' })
                          }
                        >
                          toggle
                        </button>
                        <button
                          className="text-[10px] font-black uppercase text-muted hover:text-danger"
                          onClick={() => {
                            if (confirm(`Delete ${g.name}?`)) removeGate.mutate(g.id);
                          }}
                        >
                          del
                        </button>
                      </>
                    ) : null}
                  </span>
                ))}
              {can('gates.manage') ? (
                <button className="rounded-lg border border-dashed border-line px-2.5 py-1 text-xs font-bold text-muted hover:text-brand" onClick={() => setGateBranch(b.id)}>
                  + gate
                </button>
              ) : null}
            </div>
          </div>
        ))}
      </div>
      {branchModal ? (
        <BranchModal
          branch={branchModal === 'new' ? null : ((branches ?? []).find((b) => b.id === branchModal.id) ?? null)}
          onClose={() => setBranchModal(null)}
          onDone={() => {
            setBranchModal(null);
            refresh();
          }}
        />
      ) : null}
      {gateBranch ? (
        <GateModal
          branchId={gateBranch}
          onClose={() => setGateBranch(null)}
          onDone={() => {
            setGateBranch(null);
            refresh();
          }}
        />
      ) : null}
    </div>
  );
}

function BranchModal({
  branch,
  onClose,
  onDone,
}: {
  branch: Branch | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(branch?.name ?? '');
  const [address, setAddress] = useState(branch?.address ?? '');
  const [operatingHours, setOperatingHours] = useState(branch?.operatingHours ?? '06:00 – 22:00');
  const [managerName, setManagerName] = useState(branch?.managerName ?? '');
  const [status, setStatus] = useState<'ACTIVE' | 'INACTIVE'>(branch?.status ?? 'ACTIVE');
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () =>
      branch
        ? api.admin.branches.update(branch.id, {
            name,
            address,
            operatingHours,
            managerName: managerName || null,
            status,
          })
        : api.admin.branches.create({
            name,
            address,
            operatingHours,
            timezone: 'Asia/Jakarta',
            managerName: managerName || null,
          }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });

  return (
    <Modal title={branch ? `Edit ${branch.name}` : 'New branch'} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Name</label>
          <input className="a-input" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div>
          <label className="a-label">Address</label>
          <input className="a-input" value={address} onChange={(e) => setAddress(e.target.value)} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Operating hours</label>
            <input className="a-input" value={operatingHours} onChange={(e) => setOperatingHours(e.target.value)} />
          </div>
          <div>
            <label className="a-label">Manager</label>
            <input className="a-input" value={managerName} onChange={(e) => setManagerName(e.target.value)} />
          </div>
        </div>
        {branch ? (
          <div>
            <label className="a-label">Status</label>
            <select className="a-input" value={status} onChange={(e) => setStatus(e.target.value as typeof status)}>
              <option>ACTIVE</option>
              <option>INACTIVE</option>
            </select>
          </div>
        ) : null}
        <ErrorNote message={error} />
        <button className="a-btn" disabled={mutation.isPending || name.length < 2 || address.length < 3} onClick={() => mutation.mutate()}>
          {branch ? 'Save changes' : 'Create branch'}
        </button>
      </div>
    </Modal>
  );
}

function GateModal({ branchId, onClose, onDone }: { branchId: string; onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState('');
  const [error, setError] = useState<string | null>(null);
  const mutation = useMutation({
    mutationFn: () => api.admin.gates.create({ name, branchId, status: 'ONLINE' }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });
  return (
    <Modal title="New gate" onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Gate name</label>
          <input className="a-input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Gate C" />
        </div>
        <ErrorNote message={error} />
        <button className="a-btn" disabled={mutation.isPending || name.length < 2} onClick={() => mutation.mutate()}>
          Create gate
        </button>
      </div>
    </Modal>
  );
}

function UsersTab() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<AdminUser | 'new' | null>(null);
  const [error, setError] = useState<string | null>(null);
  const { data: users, isLoading } = useQuery({ queryKey: ['admin-users'], queryFn: api.auth.adminUsers });
  const { data: branches } = useQuery({ queryKey: ['branches'], queryFn: api.catalog.branches });

  const remove = useMutation({
    mutationFn: (id: string) => api.admin.users.remove(id),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Delete failed.'),
  });

  if (isLoading) return <Spinner label="Loading users…" />;
  return (
    <div className="flex flex-col gap-3">
      <ErrorNote message={error} />
      {can('users.manage') ? (
        <button className="a-btn self-start" onClick={() => setEditing('new')}>
          + New user
        </button>
      ) : (
        <p className="rounded-lg bg-warn/10 px-3 py-2 text-sm font-bold text-warn">
          Read-only — only Super Admin can manage staff accounts.
        </p>
      )}
      <div className="a-card !p-0">
        <table className="a-table">
          <thead>
            <tr>
              <th>User</th>
              <th>Email</th>
              <th>Role</th>
              <th>Branch scope</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(users ?? []).map((u) => (
              <tr key={u.id}>
                <td className="font-bold">{u.name}</td>
                <td className="text-muted">{u.email}</td>
                <td>
                  <span className="rounded-full bg-brand/15 px-2.5 py-0.5 text-xs font-black uppercase text-brand">
                    {u.role.replaceAll('_', ' ')}
                  </span>
                </td>
                <td className="text-muted">
                  {u.branchId ? ((branches ?? []).find((b) => b.id === u.branchId)?.name ?? u.branchId) : 'All branches'}
                </td>
                <td className="text-right">
                  {can('users.manage') ? (
                    <div className="flex justify-end gap-2">
                      <button className="text-sm font-bold text-brand" onClick={() => setEditing(u)}>
                        Edit
                      </button>
                      <button
                        className="text-sm font-bold text-muted hover:text-danger"
                        onClick={() => {
                          if (confirm(`Delete staff user "${u.name}"?`)) remove.mutate(u.id);
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
      {editing ? (
        <UserModal
          user={editing === 'new' ? null : editing}
          branches={(branches ?? []).map((b) => ({ id: b.id, name: b.name }))}
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

function UserModal({
  user,
  branches,
  onClose,
  onDone,
}: {
  user: AdminUser | null;
  branches: { id: string; name: string }[];
  onClose: () => void;
  onDone: () => void;
}) {
  const [name, setName] = useState(user?.name ?? '');
  const [email, setEmail] = useState(user?.email ?? '');
  const [role, setRole] = useState<AdminRole>(user?.role ?? 'FRONT_DESK');
  const [branchId, setBranchId] = useState(user?.branchId ?? '');
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => {
      const body = { name, email, role, branchId: branchId || null };
      return user ? api.admin.users.update(user.id, body) : api.admin.users.create(body);
    },
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Save failed.'),
  });

  return (
    <Modal title={user ? `Edit ${user.name}` : 'New staff user'} onClose={onClose}>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Name</label>
          <input className="a-input" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div>
          <label className="a-label">Email</label>
          <input className="a-input" value={email} onChange={(e) => setEmail(e.target.value)} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="a-label">Role</label>
            <select className="a-input" value={role} onChange={(e) => setRole(e.target.value as AdminRole)}>
              {ADMIN_ROLES.map((r) => (
                <option key={r} value={r}>
                  {r.replaceAll('_', ' ')}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="a-label">Branch scope</label>
            <select className="a-input" value={branchId} onChange={(e) => setBranchId(e.target.value)}>
              <option value="">All branches</option>
              {branches.map((b) => (
                <option key={b.id} value={b.id}>
                  {b.name}
                </option>
              ))}
            </select>
          </div>
        </div>
        <ErrorNote message={error} />
        <button
          className="a-btn"
          disabled={mutation.isPending || name.length < 2 || !email.includes('@')}
          onClick={() => mutation.mutate()}
        >
          {user ? 'Save changes' : 'Create user'}
        </button>
      </div>
    </Modal>
  );
}

function RolesTab() {
  return (
    <div className="a-card overflow-x-auto !p-0">
      <table className="a-table">
        <thead>
          <tr>
            <th>Permission</th>
            {ADMIN_ROLES.map((r) => (
              <th key={r} className="text-center">
                {r.replaceAll('_', ' ')}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {PERMISSIONS.map((p) => (
            <tr key={p}>
              <td className="font-mono text-xs">{p}</td>
              {ADMIN_ROLES.map((r) => (
                <td key={r} className="text-center">
                  {ROLE_PERMISSIONS[r].includes(p) ? (
                    <span className="font-black text-ok">✓</span>
                  ) : (
                    <span className="text-muted/40">—</span>
                  )}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function AuditTab() {
  const { data, isLoading } = useQuery({ queryKey: ['audit'], queryFn: () => api.admin.audit.list(100) });
  if (isLoading) return <Spinner label="Loading audit trail…" />;
  return (
    <div className="a-card !p-0">
      <table className="a-table">
        <thead>
          <tr>
            <th>When</th>
            <th>Entity</th>
            <th>Action</th>
            <th>Change</th>
            <th>Actor</th>
            <th>Reason</th>
          </tr>
        </thead>
        <tbody>
          {(data ?? []).map((a) => (
            <tr key={a.id}>
              <td className="whitespace-nowrap text-muted">{formatDayTime(a.createdAt)}</td>
              <td className="font-mono text-xs">
                {a.entityType}:{a.entityId}
              </td>
              <td className="font-bold">{a.action}</td>
              <td className="max-w-xs truncate text-muted">
                {a.previousValue ?? '—'} → {a.newValue ?? '—'}
              </td>
              <td>{a.actorName}</td>
              <td className="text-muted">{a.reason ?? '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
