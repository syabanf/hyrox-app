import type { ScanResultView } from '@hyrox/contracts';
import { FlaskConical, X } from 'lucide-react';
import { useState } from 'react';
import { api } from '../lib/api';
import { useAuthStore } from '../lib/auth';
import { useInvalidateAll } from '../lib/queries';

/** Dev-only tools: simulate the physical gate, reset the demo, run the expiry sweep. */
const DEV_GATES = [
  { id: 'gat_sen_a', label: 'Senopati Gate A' },
  { id: 'gat_pik_a', label: 'PIK Gate A' },
];

export function DevDrawer() {
  const [open, setOpen] = useState(false);
  const [result, setResult] = useState<ScanResultView | null>(null);
  const [busy, setBusy] = useState(false);
  const member = useAuthStore((s) => s.member);
  const invalidate = useInvalidateAll();

  const scan = async (gateId: string) => {
    if (!member) return;
    setBusy(true);
    try {
      const res = await api.gate.scan(gateId, { memberId: member.id });
      setResult(res);
      invalidate();
    } finally {
      setBusy(false);
    }
  };

  const reset = async () => {
    await api.dev.reset();
    useAuthStore.getState().clear();
    location.href = '/auth/login';
  };

  const sweep = async () => {
    await api.dev.expirySweep();
    invalidate();
  };

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="fixed bottom-24 right-3 z-30 rounded-full border border-line bg-surface p-2.5 text-muted"
        aria-label="Dev tools"
      >
        <FlaskConical size={16} />
      </button>
    );
  }

  return (
    <div className="fixed bottom-24 right-3 z-30 w-64 rounded-2xl border border-line bg-surface-raised p-4 shadow-2xl">
      <div className="mb-3 flex items-center justify-between">
        <p className="text-xs font-black uppercase tracking-wider text-muted">Dev tools</p>
        <button onClick={() => setOpen(false)} aria-label="Close">
          <X size={16} />
        </button>
      </div>
      <div className="flex flex-col gap-2">
        {DEV_GATES.map((g) => (
          <button
            key={g.id}
            disabled={busy}
            onClick={() => void scan(g.id)}
            className="btn-ghost !py-2 text-sm"
          >
            Scan my QR · {g.label}
          </button>
        ))}
        <button onClick={() => void sweep()} className="btn-ghost !py-2 text-sm">
          Run expiry sweep
        </button>
        <button onClick={() => void reset()} className="btn-ghost !py-2 text-sm text-danger">
          Reset demo data
        </button>
      </div>
      {result ? (
        <div
          className={`mt-3 rounded-xl p-3 text-sm font-bold ${
            result.decision === 'ALLOWED' ? 'bg-ok/15 text-ok' : 'bg-danger/15 text-danger'
          }`}
        >
          {result.decision}
          {result.reason ? ` — ${result.reason.replaceAll('_', ' ')}` : ''}
          {result.remainingCredits !== null ? (
            <span className="block text-xs font-medium opacity-80">
              {result.remainingCredits} credits left
            </span>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
