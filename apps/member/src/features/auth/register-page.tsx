import { ApiError } from '@hyrox/api-client';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { api } from '../../lib/api';
import { useAuthStore } from '../../lib/auth';

const STEPS = ['Contact', 'Verify', 'Personal', 'Emergency', 'Waiver'] as const;

export function RegisterPage() {
  const navigate = useNavigate();
  const setSession = useAuthStore((s) => s.setSession);
  const [step, setStep] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [otpHint, setOtpHint] = useState('');
  const [code, setCode] = useState('');
  const [fullName, setFullName] = useState('');
  const [dateOfBirth, setDateOfBirth] = useState('');
  const [gender, setGender] = useState<'MALE' | 'FEMALE' | 'OTHER' | ''>('');
  const [ecName, setEcName] = useState('');
  const [ecPhone, setEcPhone] = useState('');
  const [ecRelation, setEcRelation] = useState('');
  const [waiverAccepted, setWaiverAccepted] = useState(false);
  const [termsAccepted, setTermsAccepted] = useState(false);

  const next = () => setStep((s) => Math.min(s + 1, STEPS.length - 1));
  const back = () => setStep((s) => Math.max(s - 1, 0));

  const requestOtp = async () => {
    setBusy(true);
    setError('');
    try {
      const res = await api.auth.requestOtp(email || phone);
      if (res.memberExists) {
        setError('That email or phone is already registered — sign in instead.');
        return;
      }
      setOtpHint(res.hint);
      next();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Something went wrong.');
    } finally {
      setBusy(false);
    }
  };

  const submit = async () => {
    setBusy(true);
    setError('');
    try {
      const res = await api.auth.register({
        fullName,
        email,
        phone,
        dateOfBirth: dateOfBirth ? new Date(dateOfBirth).toISOString() : null,
        gender: gender || null,
        emergencyContact:
          ecName && ecPhone ? { name: ecName, phone: ecPhone, relation: ecRelation || 'Contact' } : null,
        preferredBranchId: null,
        waiverAccepted: true,
        termsAccepted: true,
      });
      setSession(res.token, res.member);
      navigate('/', { replace: true });
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Something went wrong.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mx-auto flex min-h-dvh max-w-md flex-col gap-6 px-6 py-8">
      <div>
        <h1 className="display text-3xl font-black">Join the studio</h1>
        <div className="mt-4 flex gap-1.5">
          {STEPS.map((s, i) => (
            <div
              key={s}
              className={`h-1.5 flex-1 rounded-full ${i <= step ? 'bg-brand' : 'bg-line'}`}
            />
          ))}
        </div>
        <p className="mt-2 text-xs font-bold uppercase tracking-wider text-muted">
          Step {step + 1} of {STEPS.length} — {STEPS[step]}
        </p>
      </div>

      {step === 0 ? (
        <div className="flex flex-col gap-4">
          <div>
            <label className="label">Email</label>
            <input className="input" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="you@example.com" />
          </div>
          <div>
            <label className="label">Phone</label>
            <input className="input" value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+62812…" />
          </div>
          <button
            className="btn-brand"
            disabled={busy || !email.includes('@') || phone.length < 6}
            onClick={() => void requestOtp()}
          >
            Send verification code
          </button>
        </div>
      ) : null}

      {step === 1 ? (
        <div className="flex flex-col gap-4">
          <div>
            <label className="label">Verification code</label>
            <input
              className="input text-center text-2xl tracking-[0.5em]"
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
              inputMode="numeric"
              placeholder="123456"
            />
            <p className="mt-2 text-xs text-muted">{otpHint}</p>
          </div>
          <button className="btn-brand" disabled={code.length < 4} onClick={next}>
            Verify
          </button>
        </div>
      ) : null}

      {step === 2 ? (
        <div className="flex flex-col gap-4">
          <div>
            <label className="label">Full name</label>
            <input className="input" value={fullName} onChange={(e) => setFullName(e.target.value)} />
          </div>
          <div>
            <label className="label">Date of birth</label>
            <input type="date" className="input" value={dateOfBirth} onChange={(e) => setDateOfBirth(e.target.value)} />
          </div>
          <div>
            <label className="label">Gender</label>
            <div className="flex gap-2">
              {(['MALE', 'FEMALE', 'OTHER'] as const).map((g) => (
                <button
                  key={g}
                  type="button"
                  onClick={() => setGender(g)}
                  className={`flex-1 rounded-xl border px-3 py-2.5 text-sm font-bold ${
                    gender === g ? 'border-brand bg-brand/10 text-brand' : 'border-line bg-surface text-muted'
                  }`}
                >
                  {g[0] + g.slice(1).toLowerCase()}
                </button>
              ))}
            </div>
          </div>
          <button className="btn-brand" disabled={fullName.trim().length < 2} onClick={next}>
            Continue
          </button>
        </div>
      ) : null}

      {step === 3 ? (
        <div className="flex flex-col gap-4">
          <div>
            <label className="label">Contact name</label>
            <input className="input" value={ecName} onChange={(e) => setEcName(e.target.value)} />
          </div>
          <div>
            <label className="label">Contact phone</label>
            <input className="input" value={ecPhone} onChange={(e) => setEcPhone(e.target.value)} />
          </div>
          <div>
            <label className="label">Relationship</label>
            <input className="input" value={ecRelation} onChange={(e) => setEcRelation(e.target.value)} placeholder="Spouse, parent…" />
          </div>
          <button className="btn-brand" onClick={next}>
            Continue
          </button>
        </div>
      ) : null}

      {step === 4 ? (
        <div className="flex flex-col gap-4">
          <div className="card max-h-48 overflow-y-auto text-sm leading-relaxed text-muted">
            <p className="mb-2 font-black uppercase text-ink">Digital waiver (v1.0)</p>
            <p>
              I acknowledge that HYROX-style functional training involves inherent physical risks. I
              confirm I am medically fit to participate, and I release HYROX Studio, its staff and
              coaches from liability for injuries sustained during training, except in cases of
              gross negligence. I consent to the studio storing my membership and attendance data
              for operating the facility.
            </p>
          </div>
          <label className="flex items-start gap-3 text-sm">
            <input
              type="checkbox"
              checked={waiverAccepted}
              onChange={(e) => setWaiverAccepted(e.target.checked)}
              className="mt-0.5 h-5 w-5 accent-[var(--color-brand)]"
            />
            I have read and accept the digital waiver.
          </label>
          <label className="flex items-start gap-3 text-sm">
            <input
              type="checkbox"
              checked={termsAccepted}
              onChange={(e) => setTermsAccepted(e.target.checked)}
              className="mt-0.5 h-5 w-5 accent-[var(--color-brand)]"
            />
            I agree to the membership terms &amp; conditions.
          </label>
          <button
            className="btn-brand"
            disabled={busy || !waiverAccepted || !termsAccepted}
            onClick={() => void submit()}
          >
            Create my membership
          </button>
        </div>
      ) : null}

      {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}

      <div className="mt-auto flex items-center justify-between text-sm text-muted">
        {step > 0 && step !== 1 ? (
          <button onClick={back} className="font-bold">
            ← Back
          </button>
        ) : (
          <span />
        )}
        <Link to="/auth/login" className="font-bold text-brand">
          I already have an account
        </Link>
      </div>
    </div>
  );
}
