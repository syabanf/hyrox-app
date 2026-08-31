'use client';

import type { AdminRole } from '@hyrox/domain';
import { useQuery } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { useState } from 'react';
import { api } from '../../lib/api';
import { useAdminAuth } from '../../lib/auth';

const ROLE_DESCRIPTIONS: Record<AdminRole, string> = {
  SUPER_ADMIN: 'Everything, including business rules & users',
  HQ_ADMIN: 'All branches: members, commercial, operations, reports',
  BRANCH_MANAGER: 'Own branch: members, classes, gate, reports',
  FRONT_DESK: 'Member lookup, bookings, check-in, gate monitor',
  COACH: 'Assigned classes, participants, attendance',
  FINANCE: 'Payments, refunds, credit liability, financial reports',
};

export default function LoginPage() {
  const router = useRouter();
  const setSession = useAdminAuth((s) => s.setSession);
  const [busy, setBusy] = useState('');
  const { data: users } = useQuery({ queryKey: ['admin-users'], queryFn: api.auth.adminUsers });

  const login = async (userId: string) => {
    setBusy(userId);
    try {
      const res = await api.auth.adminLogin(userId);
      setSession(res.token, res.user, res.permissions);
      router.replace('/dashboard');
    } finally {
      setBusy('');
    }
  };

  return (
    <div className="mx-auto flex min-h-dvh max-w-3xl flex-col justify-center gap-8 px-6 py-10">
      <div>
        <h1 className="display text-4xl font-black">
          HYROX<span className="text-brand">STUDIO</span> <span className="text-muted">Admin</span>
        </h1>
        <p className="mt-2 text-muted">
          Demo mode — pick a role to sign in. RBAC is enforced by the (mock) server, not just the UI.
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {(users ?? []).map((u) => (
          <button
            key={u.id}
            onClick={() => void login(u.id)}
            disabled={busy !== ''}
            className="a-card group text-left transition hover:border-brand"
          >
            <div className="flex items-center justify-between">
              <p className="font-black">{u.name}</p>
              <span className="rounded-full bg-brand/15 px-2.5 py-0.5 text-[11px] font-black uppercase tracking-wide text-brand">
                {u.role.replaceAll('_', ' ')}
              </span>
            </div>
            <p className="mt-1 text-sm text-muted">{ROLE_DESCRIPTIONS[u.role]}</p>
            <p className="mt-2 text-xs font-bold text-brand opacity-0 transition group-hover:opacity-100">
              Sign in as {u.name.split(' ')[0]} →
            </p>
          </button>
        ))}
      </div>
    </div>
  );
}
