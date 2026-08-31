import { ApiError } from '@hyrox/api-client';
import { ArrowRight } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { api } from '../../lib/api';
import { useAuthStore } from '../../lib/auth';

/** Bundled gym photo (originally Unsplash, committed under public/img). */
const HERO_PHOTO = '/img/hero-login.jpg';

/** Segmented 6-digit code input: an invisible input drives the display boxes. */
function OtpBoxes({ value, onChange }: { value: string; onChange: (next: string) => void }) {
  return (
    <div className="relative">
      <input
        autoFocus
        inputMode="numeric"
        autoComplete="one-time-code"
        maxLength={6}
        value={value}
        onChange={(e) => onChange(e.target.value.replace(/\D/g, '').slice(0, 6))}
        className="absolute inset-0 z-10 h-full w-full cursor-text opacity-0"
        aria-label="Verification code"
      />
      <div className="flex gap-2">
        {Array.from({ length: 6 }).map((_, i) => (
          <div
            key={i}
            className={`flex h-14 flex-1 items-center justify-center rounded-2xl border-2 text-2xl font-extrabold transition ${
              i === value.length
                ? 'border-brand bg-surface'
                : 'border-transparent bg-surface-raised'
            }`}
          >
            {value[i] ?? ''}
          </div>
        ))}
      </div>
    </div>
  );
}

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
        setError('No account found — create your membership below.');
        return;
      }
      setChallengeId(res.challengeId);
      setHint(res.hint);
      setCode('');
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
    <div className="mx-auto min-h-dvh max-w-md">
      {/* Photo hero, fading into the page background */}
      <div className="relative h-[46dvh] min-h-80 w-full overflow-hidden">
        <img src={HERO_PHOTO} alt="" className="h-full w-full object-cover" />
        <div className="absolute inset-0 bg-gradient-to-b from-black/60 via-black/25 to-[#f6f6f2]" />
        <div className="absolute inset-x-0 top-0 p-6 pt-[max(env(safe-area-inset-top),1.5rem)]">
          <p className="display text-lg text-white">
            HYROX<span className="text-[#ff4348]">STUDIO</span>
          </p>
        </div>
        <div className="absolute inset-x-0 bottom-14 px-6">
          <h1 className="display text-4xl leading-[1.05] text-white drop-shadow-[0_2px_12px_rgb(0_0_0/0.4)]">
            Train hard.
            <br />
            Check in faster.
          </h1>
        </div>
      </div>

      {/* Floating form card — relative so it paints above the hero's absolute overlay */}
      <div className="relative -mt-10 px-4 pb-10">
        <div className="card !p-6">
          {!challengeId ? (
            <div className="flex flex-col gap-4">
              <div>
                <p className="display text-2xl">Sign in</p>
                <p className="mt-0.5 text-sm text-muted">
                  We'll send a one-time code to verify it's you.
                </p>
              </div>
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
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' && identifier.length >= 3) void requestOtp();
                  }}
                />
              </div>
              <button
                className="btn-brand flex items-center justify-center gap-2"
                disabled={busy || identifier.length < 3}
                onClick={() => void requestOtp()}
              >
                Continue <ArrowRight size={18} />
              </button>
            </div>
          ) : (
            <div className="flex flex-col gap-4">
              <div>
                <p className="display text-2xl">Enter the code</p>
                <p className="mt-0.5 text-sm text-muted">
                  Sent to <span className="font-bold text-ink">{identifier}</span>
                </p>
              </div>
              <OtpBoxes
                value={code}
                onChange={(next) => {
                  setCode(next);
                  setError('');
                }}
              />
              <p className="text-xs text-muted">{hint}</p>
              <button className="btn-brand" disabled={busy || code.length < 4} onClick={() => void verify()}>
                Sign in
              </button>
              <button
                className="text-center text-sm font-bold text-muted"
                onClick={() => {
                  setChallengeId(null);
                  setCode('');
                  setError('');
                }}
              >
                Use a different email or phone
              </button>
            </div>
          )}

          {error ? <p className="mt-3 text-sm font-bold text-danger">{error}</p> : null}
        </div>

        <Link to="/auth/register" className="btn-ghost mt-3 block">
          Create your membership
        </Link>

        <p className="mt-6 text-center">
          <span className="chip bg-surface-raised text-muted">
            Demo: demo@hyrox.id · any 6-digit code
          </span>
        </p>
      </div>
    </div>
  );
}
