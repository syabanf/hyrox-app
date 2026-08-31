import { Spinner, StatusBadge, formatDay } from '@hyrox/ui';
import { Camera, ChevronRight, HeartPulse, LogOut, Settings, Wallet } from 'lucide-react';
import { useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { api } from '../../lib/api';
import { useAuthStore } from '../../lib/auth';
import { resizeImageToDataUrl } from '../../lib/image';
import { useInvalidateAll, useMe } from '../../lib/queries';

export function ProfilePage() {
  const navigate = useNavigate();
  const { data: me, isLoading } = useMe();
  const clear = useAuthStore((s) => s.clear);
  const invalidate = useInvalidateAll();
  const [editing, setEditing] = useState(false);
  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [busy, setBusy] = useState(false);

  if (isLoading || !me) return <Spinner label="Loading profile…" />;
  const m = me.member;

  const startEdit = () => {
    setEmail(m.email);
    setPhone(m.phone);
    setEditing(true);
  };

  const save = async () => {
    setBusy(true);
    try {
      await api.me.update({ email, phone });
      invalidate();
      setEditing(false);
    } finally {
      setBusy(false);
    }
  };

  const uploadAvatar = async (file: File | undefined) => {
    if (!file) return;
    const avatarUrl = await resizeImageToDataUrl(file, 128, 0.8);
    await api.me.update({ avatarUrl });
    invalidate();
  };

  const logout = () => {
    clear();
    navigate('/auth/login', { replace: true });
  };

  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-center gap-4">
        <label className="relative cursor-pointer" title="Change photo">
          {m.avatarUrl ? (
            <img src={m.avatarUrl} alt="" className="h-16 w-16 rounded-full object-cover" />
          ) : (
            <div className="flex h-16 w-16 items-center justify-center rounded-full bg-brand text-2xl font-black text-white">
              {m.fullName
                .split(' ')
                .slice(0, 2)
                .map((p) => p[0])
                .join('')}
            </div>
          )}
          <span className="absolute -bottom-0.5 -right-0.5 rounded-full border border-line bg-surface p-1">
            <Camera size={12} />
          </span>
          <input
            type="file"
            accept="image/*"
            className="hidden"
            onChange={(e) => void uploadAvatar(e.target.files?.[0])}
          />
        </label>
        <div>
          <h1 className="display text-3xl font-black">{m.fullName}</h1>
          <StatusBadge status={m.status} />
        </div>
      </div>

      <section className="card flex flex-col gap-3 text-sm">
        <p className="label !mb-0">Personal information</p>
        {editing ? (
          <>
            <div>
              <label className="label">Email</label>
              <input className="input" value={email} onChange={(e) => setEmail(e.target.value)} />
            </div>
            <div>
              <label className="label">Phone</label>
              <input className="input" value={phone} onChange={(e) => setPhone(e.target.value)} />
            </div>
            <div className="flex gap-2">
              <button className="btn-brand flex-1 !py-2" disabled={busy} onClick={() => void save()}>
                Save
              </button>
              <button className="btn-ghost flex-1 !py-2" onClick={() => setEditing(false)}>
                Cancel
              </button>
            </div>
          </>
        ) : (
          <>
            <div className="flex justify-between">
              <span className="text-muted">Email</span>
              <span className="font-bold">{m.email}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted">Phone</span>
              <span className="font-bold">{m.phone}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-muted">Member since</span>
              <span className="font-bold">{formatDay(m.createdAt)}</span>
            </div>
            <button className="btn-ghost !py-2 text-sm" onClick={startEdit}>
              Edit contact info
            </button>
          </>
        )}
      </section>

      <section className="card flex flex-col !p-2 text-sm">
        {[
          { to: '/wallet', icon: Wallet, title: 'Wallet & credits', hint: 'Balance, top up, history' },
          {
            to: '/profile/emergency',
            icon: HeartPulse,
            title: 'Emergency contact',
            hint: m.emergencyContact
              ? `${m.emergencyContact.name} · ${m.emergencyContact.phone}`
              : 'Not set — add one',
          },
          { to: '/profile/settings', icon: Settings, title: 'Settings', hint: 'Units, reminders' },
        ].map(({ to, icon: Icon, title, hint }) => (
          <Link key={to} to={to} className="flex items-center gap-3 rounded-xl px-2 py-2.5 active:bg-surface-raised">
            <Icon size={18} className="text-brand" />
            <span className="min-w-0 flex-1">
              <span className="block font-bold">{title}</span>
              <span className="block truncate text-xs text-muted">{hint}</span>
            </span>
            <ChevronRight size={16} className="text-muted" />
          </Link>
        ))}
      </section>

      <section className="card flex flex-col gap-2 text-sm">
        <p className="label !mb-0">Digital waiver</p>
        <div className="flex justify-between">
          <span className="text-muted">Version {m.waiverVersion ?? '—'}</span>
          <span className="font-bold text-ok">
            {m.waiverAcceptedAt ? `Signed ${formatDay(m.waiverAcceptedAt)}` : 'Not signed'}
          </span>
        </div>
      </section>

      <button onClick={logout} className="btn-ghost flex items-center justify-center gap-2 text-danger">
        <LogOut size={16} /> Sign out
      </button>
    </div>
  );
}
