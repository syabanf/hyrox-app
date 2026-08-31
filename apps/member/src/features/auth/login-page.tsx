import { ApiError } from '@hyrox/api-client';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { api } from '../../lib/api';
import { useAuthStore } from '../../lib/auth';

export function LoginPage() {
  const navigate = useNavigate();
  const setSession = useAuthStore((s) => s.setSession);
  const [identifier, setIdentifier] = useState('demo@hyrox.id');
  const [challengeId, setChallengeId] = useState<string | null>(null);
  const [hint, setHint] = useState('');
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const requestOtp = async () => {
    setBusy(true);
    setError('');
    try {
      const res = await api.auth.requestOtp(identifier);
      if (!res.memberExists) {
        setError('No account found — register below.');
        return;
      }
      setChallengeId(res.challengeId);
      setHint(res.hint);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Something went wrong.');
    } finally {
      setBusy(false);
    }
  };

  const verify = async () => {
    if (!challengeId) return;
    setBusy(true);
    setError('');
    try {
      const res = await api.auth.verifyOtp(challengeId, code);
      setSession(res.token, res.member);
      navigate('/', { replace: true });
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Something went wrong.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mx-auto flex min-h-dvh max-w-md flex-col justify-center gap-6 px-6">
      <div>
        <h1 className="display text-4xl font-black leading-none">
          HYROX<span className="text-brand">STUDIO</span>
        </h1>
        <p className="mt-2 text-muted">Train. Book. Scan. Go.</p>
      </div>

      {!challengeId ? (
        <div className="flex flex-col gap-4">
          <div>
            <label className="label" htmlFor="identifier">
              Email or phone
            </label>
            <input
              id="identifier"
              className="input"
              value={identifier}
              onChange={(e) => setIdentifier(e.target.value)}
              placeholder="you@example.com"
              autoComplete="username"
            />
          </div>
          <button className="btn-brand" disabled={busy || identifier.length < 3} onClick={() => void requestOtp()}>
            Send code
          </button>
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          <div>
            <label className="label" htmlFor="code">
              Verification code
            </label>
            <input
              id="code"
              className="input text-center text-2xl tracking-[0.5em]"
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              inputMode="numeric"
              placeholder="123456"
              autoFocus
            />
            <p className="mt-2 text-xs text-muted">{hint}</p>
          </div>
          <button className="btn-brand" disabled={busy || code.length < 4} onClick={() => void verify()}>
            Sign in
          </button>
          <button className="btn-ghost" onClick={() => setChallengeId(null)}>
            Back
          </button>
        </div>
      )}

      {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}

      <p className="text-center text-sm text-muted">
        New here?{' '}
        <Link to="/auth/register" className="font-bold text-brand">
          Create your membership
        </Link>
      </p>
      <p className="text-center text-xs text-muted/60">
        Demo account: demo@hyrox.id · any 6-digit code
      </p>
    </div>
  );
}
