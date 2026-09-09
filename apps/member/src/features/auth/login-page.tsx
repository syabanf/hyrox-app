import { ApiError } from '@nuhabit/api-client';
import { ArrowRight } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { api } from '../../lib/api';
import { useAuthStore } from '../../lib/auth';

/** Bundled gym photo (originally Unsplash, committed under public/img). */
const HERO_PHOTO = '/img/hero-login.jpg';

export function LoginPage() {
  const navigate = useNavigate();
  const setSession = useAuthStore((s) => s.setSession);
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const canSubmit = identifier.trim().length >= 3 && password.length > 0;

  const signIn = async () => {
    if (!canSubmit) return;
    setBusy(true);
    setError('');
    try {
      const res = await api.auth.login({ identifier: identifier.trim(), password });
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
        {/* Brand pattern as a quiet texture over the photo, under the fade. */}
        <div className="pattern-brand pointer-events-none absolute inset-0" aria-hidden />
        <div className="absolute inset-0 bg-gradient-to-b from-black/60 via-black/25 to-[#f3ece2]" />
        <div className="absolute inset-x-0 top-0 p-6 pt-[max(env(safe-area-inset-top),1.5rem)]">
          {/* Lime wordmark on the dark hero (white PNG used as a mask). */}
          <div className="logo-lime h-7 w-[204px]" role="img" aria-label="NüHabit" />
        </div>
        <div className="absolute inset-x-0 bottom-14 px-6">
          <h1 className="display text-4xl leading-[1.05] text-white drop-shadow-[0_2px_12px_rgb(0_0_0/0.4)]">
            Train hard.
            <br />
            Check in faster.
          </h1>
        </div>
      </div>

      {/* Floating form card - relative so it paints above the hero's absolute overlay */}
      <div className="relative -mt-10 px-4 pb-10">
        <div className="card !p-6">
          {/* One form, submitted as a form: a password manager will not offer
              to fill a pair of inputs that never announce themselves as a
              sign-in, and Enter has to work from either field. */}
          <form
            className="flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              void signIn();
            }}
          >
            <div>
              <p className="display text-2xl">Sign in</p>
              <p className="mt-0.5 text-sm text-muted">Use the email or phone on your membership.</p>
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
                autoCapitalize="none"
                spellCheck={false}
              />
            </div>
            <div>
              <label className="label" htmlFor="password">
                Password
              </label>
              <input
                id="password"
                type="password"
                className="input"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                autoComplete="current-password"
              />
            </div>
            <button
              type="submit"
              className="btn-brand flex items-center justify-center gap-2"
              disabled={busy || !canSubmit}
            >
              Sign in <ArrowRight size={18} />
            </button>
            {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
          </form>
        </div>

        <Link to="/auth/register" className="btn-ghost mt-3 block">
          Create your membership
        </Link>

        <p className="mt-6 text-center">
          <span className="chip bg-surface-raised text-muted">
            Demo: demo@nuhabit.id · nuhabit-demo-2026
          </span>
        </p>
      </div>
    </div>
  );
}
