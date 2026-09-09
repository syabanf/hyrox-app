'use client';

import type { AdminRole } from '@nuhabit/domain';
import { useQuery } from '@tanstack/react-query';
import { ChevronDown, Loader2, LogIn } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useState, type FormEvent } from 'react';
import { ApiError, api, usingRealBackend } from '../../lib/api';
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

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState('');
  const [showRoster, setShowRoster] = useState(false);

  // What the server will accept. Read before anything is typed, because it
  // decides whether the role cards below are offered at all.
  const { data: mode } = useQuery({
    queryKey: ['auth-mode'],
    queryFn: api.auth.adminAuthMode,
    retry: false,
  });
  // Only fetched once the roster is opened: with demo mode off the endpoint
  // needs a token, and asking for it unprompted logs a 401 on every visit.
  const { data: users } = useQuery({
    queryKey: ['admin-users'],
    queryFn: api.auth.adminUsers,
    enabled: showRoster && mode?.demoRoster === true,
  });

  const signIn = async (input: { userId?: string; email?: string; password?: string }, busyKey: string) => {
    setBusy(busyKey);
    setError(null);
    try {
      const res = await api.auth.adminLogin(input);
      setSession(res.token, res.user, res.permissions, res.mustChangePassword);
      router.replace(res.mustChangePassword ? '/account/password' : '/dashboard');
    } catch (e) {
      setError(
        e instanceof ApiError
          ? e.message
          : 'Could not reach the server. Check that the API is running.',
      );
      setBusy('');
    }
  };

  const submit = (e: FormEvent) => {
    e.preventDefault();
    void signIn({ email: email.trim(), password }, 'form');
  };

  return (
    <div className="grid min-h-dvh lg:grid-cols-[1.1fr_1fr]">
      {/* Brand side: dark green ground, pattern texture, white wordmark. It
          is stuck to the viewport, so opening the demo roster scrolls the form
          column without dragging the artwork up with it. */}
      <div className="surface-ink relative hidden overflow-hidden lg:sticky lg:top-0 lg:flex lg:h-dvh lg:flex-col lg:justify-between lg:p-12">
        <div className="pattern-brand pointer-events-none absolute inset-0" aria-hidden />
        {/* self-start, or the wordmark is stretched to twice its width. The
            panel is a flex column, so align-items defaults to stretch and
            pulls a w-auto child across the whole cross axis — the height
            holds at h-9 and only the width grows, which is exactly how you
            flatten a logo without touching its CSS. */}
        <img
          src="/admin/brand/nuhabit-logo-white.png"
          alt="NüHabit"
          className="relative h-9 w-auto self-start"
        />
        <div className="relative">
          <h1 className="display max-w-md text-5xl uppercase leading-[0.95] text-white">
            The studio, from the back office
          </h1>
          <p className="mt-4 max-w-sm text-sm text-white/60">
            Members, classes, the gate, the till, stock and the people who buy it — one panel, one
            set of numbers.
          </p>
        </div>
        <p className="relative text-xs text-white/40">
          {usingRealBackend ? 'Connected to the NüHabit API' : 'Offline demo — no server behind it'}
        </p>
      </div>

      {/* Form side. */}
      <div className="flex flex-col justify-center px-6 py-12 sm:px-12">
        <div className="mx-auto w-full max-w-sm">
          <img
            src="/admin/brand/nuhabit-logo-black.png"
            alt="NüHabit"
            className="mb-8 h-8 w-auto lg:hidden"
          />
          <h2 className="display text-3xl uppercase">Sign in</h2>
          <p className="mt-1 text-sm text-muted">Staff accounts only.</p>

          <form onSubmit={submit} className="mt-8 flex flex-col gap-4">
            <label className="block">
              <span className="a-label">Email address</span>
              <input
                className="a-input"
                type="email"
                name="email"
                autoComplete="username"
                autoFocus
                placeholder="you@nuhabit.id"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </label>
            <label className="block">
              <span className="a-label">Password</span>
              <input
                className="a-input"
                type="password"
                name="password"
                autoComplete="current-password"
                placeholder="••••••••••"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </label>

            {error ? (
              <p
                role="alert"
                className="rounded-xl bg-danger/10 px-3 py-2 text-sm font-bold text-danger"
              >
                {error}
              </p>
            ) : null}

            <button
              type="submit"
              className="a-btn-forest w-full py-2.5"
              disabled={busy !== '' || email.trim() === '' || password === ''}
            >
              {busy === 'form' ? (
                <Loader2 size={16} className="animate-spin" />
              ) : (
                <LogIn size={16} />
              )}
              Sign in
            </button>
            <p className="text-xs text-muted">
              Forgotten it? Ask a Super Admin to set you a new one — they can do it from Config →
              Users.
            </p>
          </form>

          {mode?.demoRoster ? (
            <div className="mt-10 border-t border-line pt-6">
              <button
                type="button"
                onClick={() => setShowRoster((v) => !v)}
                className="flex w-full items-center justify-between text-left"
              >
                <span>
                  <span className="text-sm font-black">Demo sign-in</span>
                  <span className="mt-0.5 block text-xs text-muted">
                    Pick a role instead of typing a password.
                  </span>
                </span>
                <ChevronDown
                  size={18}
                  className={`shrink-0 text-muted transition ${showRoster ? 'rotate-180' : ''}`}
                />
              </button>

              {showRoster ? (
                <div className="mt-4 flex flex-col gap-2">
                  {(users ?? []).map((u) => (
                    <button
                      key={u.id}
                      type="button"
                      onClick={() => void signIn({ userId: u.id }, u.id)}
                      disabled={busy !== ''}
                      className="a-card group !p-3 text-left transition hover:border-brand disabled:opacity-40"
                    >
                      <div className="flex items-center justify-between gap-2">
                        <p className="text-sm font-black">{u.name}</p>
                        <span className="rounded-full bg-brand/15 px-2 py-0.5 text-[10px] font-black uppercase tracking-wide text-brand">
                          {u.role.replaceAll('_', ' ')}
                        </span>
                      </div>
                      <p className="mt-0.5 text-xs text-muted">{ROLE_DESCRIPTIONS[u.role]}</p>
                    </button>
                  ))}
                  {mode.accountsWithoutPassword > 0 ? (
                    <p className="text-xs text-warn">
                      {mode.accountsWithoutPassword} staff account
                      {mode.accountsWithoutPassword === 1 ? ' has' : 's have'} no password yet.
                    </p>
                  ) : null}
                  <p className="text-xs text-muted">
                    Every seeded account also takes the password{' '}
                    <code className="rounded bg-surface-raised px-1 py-0.5 font-bold">
                      nuhabit-demo-2026
                    </code>
                    .
                  </p>
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
