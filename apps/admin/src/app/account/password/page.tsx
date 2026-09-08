'use client';

import { KeyRound, Loader2 } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { useEffect, useState, type FormEvent } from 'react';
import { ApiError, api } from '../../../lib/api';
import { useAdminAuth } from '../../../lib/auth';

/**
 * Changing your own password.
 *
 * It sits outside the panel shell on purpose: an account that was handed a
 * password is sent straight here after signing in, and a sidebar full of
 * places to go instead would defeat the point of forcing it.
 */
export default function ChangePasswordPage() {
  const router = useRouter();
  const { token, user, mustChangePassword, passwordChanged, clear } = useAdminAuth();

  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!token) router.replace('/login');
  }, [token, router]);
  if (!token || !user) return null;

  const mismatch = confirm !== '' && next !== confirm;
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (mismatch) return;
    setBusy(true);
    setError(null);
    try {
      await api.auth.changePassword(current, next);
      passwordChanged();
      router.replace('/dashboard');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not change the password.');
      setBusy(false);
    }
  };

  return (
    <div className="mx-auto flex min-h-dvh max-w-md flex-col justify-center px-6 py-12">
      <div className="surface-ink relative overflow-hidden rounded-3xl p-6 text-white">
        <div className="pattern-brand pointer-events-none absolute inset-0" aria-hidden />
        <div className="relative flex items-center gap-3">
          <KeyRound size={20} className="shrink-0 text-lime" />
          <div>
            <h1 className="display text-2xl uppercase">
              {mustChangePassword ? 'Choose your password' : 'Change your password'}
            </h1>
            <p className="mt-1 text-sm text-white/60">
              {mustChangePassword
                ? `The password you signed in with was set by somebody else. Replace it before you carry on.`
                : `Signed in as ${user.name}.`}
            </p>
          </div>
        </div>
      </div>

      <form onSubmit={submit} className="mt-6 flex flex-col gap-4">
        <label className="block">
          <span className="a-label">Current password</span>
          <input
            className="a-input"
            type="password"
            autoComplete="current-password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
          />
        </label>
        <label className="block">
          <span className="a-label">New password</span>
          <input
            className="a-input"
            type="password"
            autoComplete="new-password"
            value={next}
            onChange={(e) => setNext(e.target.value)}
          />
          <span className="mt-1 block text-xs text-muted">
            At least 10 characters. Length is what counts — a short phrase beats a mangled word.
          </span>
        </label>
        <label className="block">
          <span className="a-label">New password again</span>
          <input
            className="a-input"
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
          />
          {mismatch ? (
            <span className="mt-1 block text-xs font-bold text-danger">
              The two do not match.
            </span>
          ) : null}
        </label>

        {error ? (
          <p role="alert" className="rounded-xl bg-danger/10 px-3 py-2 text-sm font-bold text-danger">
            {error}
          </p>
        ) : null}

        <button
          type="submit"
          className="a-btn-forest w-full py-2.5"
          disabled={busy || current === '' || next.length < 10 || mismatch || confirm === ''}
        >
          {busy ? <Loader2 size={16} className="animate-spin" /> : null}
          Save the new password
        </button>
        <button
          type="button"
          className="a-btn-ghost w-full"
          onClick={() => {
            clear();
            router.replace('/login');
          }}
        >
          {mustChangePassword ? 'Sign out instead' : 'Back to signing out'}
        </button>
      </form>
    </div>
  );
}
