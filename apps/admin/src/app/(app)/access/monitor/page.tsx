'use client';

import type { ScanResultView } from '@hyrox/contracts';
import { Spinner, StatusBadge, formatTime } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, PageTitle } from '../../../../components/ui';

const PIPELINE_STEPS = [
  { key: 'token', label: 'QR valid' },
  { key: 'member', label: 'Membership active' },
  { key: 'passback', label: 'Anti-passback clear' },
  { key: 'credit', label: 'Credit available' },
] as const;

function failedStep(reason: string | null): string | null {
  if (!reason) return null;
  if (reason.startsWith('TOKEN')) return 'token';
  if (reason === 'MEMBER_NOT_ACTIVE') return 'member';
  if (reason === 'ANTI_PASSBACK') return 'passback';
  if (reason === 'INSUFFICIENT_CREDITS') return 'credit';
  return null;
}

export default function MonitorPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const { data: logs, isLoading } = useQuery({
    queryKey: ['access-logs', 'live'],
    queryFn: () => api.admin.accessLogs.list({ limit: 25 }),
    refetchInterval: 5_000,
  });

  return (
    <div>
      <PageTitle title="Live Check-in" subtitle="Real-time gate activity (auto-refreshes)" />
      <div className="grid gap-4 xl:grid-cols-[1fr_380px]">
        <div className="a-card !p-0">
          {isLoading ? (
            <Spinner label="Loading feed…" />
          ) : (
            <table className="a-table">
              <thead>
                <tr>
                  <th>Time</th>
                  <th>Member</th>
                  <th>Gate</th>
                  <th>Result</th>
                  <th className="text-right">Credits</th>
                  <th>Mode</th>
                </tr>
              </thead>
              <tbody>
                {(logs ?? []).map((v) => (
                  <tr key={v.log.id}>
                    <td className="whitespace-nowrap font-bold">{formatTime(v.log.createdAt)}</td>
                    <td>{v.memberName ?? <span className="text-muted">Unknown</span>}</td>
                    <td>{v.gateName}</td>
                    <td>
                      <StatusBadge status={v.log.result} />
                      {v.log.reasonCode ? (
                        <span className="ml-1 text-xs font-bold text-danger">
                          {v.log.reasonCode.replaceAll('_', ' ')}
                        </span>
                      ) : null}
                    </td>
                    <td className="text-right font-bold">{v.log.creditDelta || '—'}</td>
                    <td className="text-muted">{v.log.mode}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
        {can('access.simulate') ? (
          <GateSimulator onScanned={() => void qc.invalidateQueries()} />
        ) : (
          <div className="a-card text-sm text-muted">
            Your role can monitor the feed but not run the gate simulator.
          </div>
        )}
      </div>
    </div>
  );
}

function GateSimulator({ onScanned }: { onScanned: () => void }) {
  const { data: gates } = useQuery({ queryKey: ['gates'], queryFn: api.admin.gates.list });
  const { data: members } = useQuery({ queryKey: ['members', '', ''], queryFn: () => api.admin.members.list() });
  const [gateId, setGateId] = useState('');
  const [memberId, setMemberId] = useState('');
  const [result, setResult] = useState<ScanResultView | null>(null);
  const [error, setError] = useState<string | null>(null);

  const scan = useMutation({
    mutationFn: () => api.gate.scan(gateId, { memberId }),
    onSuccess: (res) => {
      setResult(res);
      setError(null);
      onScanned();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Scan failed.'),
  });

  const failed = result ? failedStep(result.reason) : null;

  return (
    <div className="a-card h-fit">
      <p className="display text-lg font-black">Gate Simulator</p>
      <p className="mb-3 text-sm text-muted">
        Runs the exact hardware path: issue QR → scan → validation pipeline → deduction → log.
      </p>
      <div className="flex flex-col gap-3">
        <div>
          <label className="a-label">Gate</label>
          <select className="a-input" value={gateId} onChange={(e) => setGateId(e.target.value)}>
            <option value="">Select…</option>
            {(gates ?? []).map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="a-label">Member</label>
          <select className="a-input" value={memberId} onChange={(e) => setMemberId(e.target.value)}>
            <option value="">Select…</option>
            {(members ?? []).map((m) => (
              <option key={m.member.id} value={m.member.id}>
                {m.member.fullName} · {m.member.status} · {m.balance} cr
              </option>
            ))}
          </select>
        </div>
        <button className="a-btn" disabled={scan.isPending || !gateId || !memberId} onClick={() => scan.mutate()}>
          Scan QR at gate
        </button>
        <ErrorNote message={error} />
        {result ? (
          <div
            className={`rounded-xl border p-4 ${
              result.decision === 'ALLOWED' ? 'border-ok/50 bg-ok/10' : 'border-danger/50 bg-danger/10'
            }`}
          >
            <p className={`display text-2xl font-black ${result.decision === 'ALLOWED' ? 'text-ok' : 'text-danger'}`}>
              {result.decision}
            </p>
            <p className="text-sm font-bold">{result.memberName}</p>
            {result.remainingCredits !== null ? (
              <p className="text-sm text-muted">{result.remainingCredits} credits remaining</p>
            ) : null}
            {result.entryKind ? (
              <p className="text-xs font-bold uppercase tracking-wide text-muted">
                {result.entryKind.replaceAll('_', ' ')} entry
              </p>
            ) : null}
            <div className="mt-3 flex flex-col gap-1.5">
              {PIPELINE_STEPS.map((s, i) => {
                const failIdx = PIPELINE_STEPS.findIndex((p) => p.key === failed);
                const state =
                  failed === null ? 'pass' : i < failIdx ? 'pass' : i === failIdx ? 'fail' : 'skip';
                return (
                  <div key={s.key} className="flex items-center gap-2 text-sm">
                    <span
                      className={
                        state === 'pass' ? 'text-ok' : state === 'fail' ? 'text-danger' : 'text-muted/50'
                      }
                    >
                      {state === 'pass' ? '✓' : state === 'fail' ? '✕' : '·'}
                    </span>
                    <span className={state === 'skip' ? 'text-muted/50' : ''}>{s.label}</span>
                    {state === 'fail' && result.reason ? (
                      <span className="text-xs font-bold text-danger">
                        {result.reason.replaceAll('_', ' ')}
                      </span>
                    ) : null}
                  </div>
                );
              })}
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
}
