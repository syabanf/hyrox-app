'use client';

import { Spinner, StatusBadge, formatDayTime } from '@hyrox/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, PageTitle, Pager, StatCard } from '../../../../components/ui';

export default function AccessLogsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [gateId, setGateId] = useState('');
  const [result, setResult] = useState('');
  const [mode, setMode] = useState('');
  const [memberQuery, setMemberQuery] = useState('');
  const [page, setPage] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const resolve = useMutation({
    mutationFn: ({ logId, action }: { logId: string; action: 'APPROVE' | 'REJECT' }) =>
      api.admin.accessLogs.resolve(logId, {
        action,
        reason: action === 'APPROVE' ? 'Verified offline entry' : 'Rejected offline entry',
      }),
    onSuccess: () => {
      setError(null);
      void qc.invalidateQueries();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'Resolve failed.'),
  });
  const { data: gates } = useQuery({ queryKey: ['gates'], queryFn: api.admin.gates.list });
  const { data: logs, isLoading } = useQuery({
    queryKey: ['access-logs', gateId, result, mode],
    queryFn: () =>
      api.admin.accessLogs.list({
        gateId: gateId || undefined,
        result: result || undefined,
        mode: mode || undefined,
        limit: 200,
      }),
  });

  const offline = (logs ?? []).filter((l) => l.log.mode === 'OFFLINE');
  const synced = offline.filter((l) => l.log.result === 'SYNCED').length;
  const conflicts = offline.filter((l) => l.log.result === 'CONFLICT').length;

  return (
    <div>
      <PageTitle title="Access Logs" subtitle="Every gate decision, incl. offline fallback & re-sync" />
      <ErrorNote message={error} />
      <div className="mb-4 grid gap-3 sm:grid-cols-3">
        <StatCard label="Offline transactions" value={offline.length} />
        <StatCard label="Synced" value={synced} />
        <StatCard label="Conflicts" value={conflicts} tone={conflicts > 0 ? 'danger' : undefined} hint="Need manual reconciliation" />
      </div>
      <div className="mb-4 flex flex-wrap gap-2">
        <input
          className="a-input max-w-xs"
          placeholder="Search member…"
          value={memberQuery}
          onChange={(e) => {
            setMemberQuery(e.target.value);
            setPage(0);
          }}
        />
        <select className="a-input max-w-44" value={gateId} onChange={(e) => setGateId(e.target.value)}>
          <option value="">All gates</option>
          {(gates ?? []).map((g) => (
            <option key={g.id} value={g.id}>
              {g.name}
            </option>
          ))}
        </select>
        <select className="a-input max-w-40" value={result} onChange={(e) => setResult(e.target.value)}>
          <option value="">All results</option>
          <option value="ALLOWED">Allowed</option>
          <option value="DENIED">Denied</option>
          <option value="CONFLICT">Conflict</option>
        </select>
        <select className="a-input max-w-40" value={mode} onChange={(e) => setMode(e.target.value)}>
          <option value="">All modes</option>
          <option value="ONLINE">Online</option>
          <option value="OFFLINE">Offline</option>
        </select>
      </div>
      {isLoading ? (
        <Spinner label="Loading logs…" />
      ) : (
        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>When</th>
                <th>Member</th>
                <th>Gate</th>
                <th>Branch</th>
                <th>Result</th>
                <th className="text-right">Credits</th>
                <th>Mode</th>
                <th className="text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {(logs ?? [])
                .filter((v) => !memberQuery || (v.memberName ?? '').toLowerCase().includes(memberQuery.toLowerCase()))
                .slice(page * 12, page * 12 + 12)
                .map((v) => (
                <tr key={v.log.id}>
                  <td className="whitespace-nowrap text-muted">{formatDayTime(v.log.createdAt)}</td>
                  <td className="font-bold">{v.memberName ?? '—'}</td>
                  <td>{v.gateName}</td>
                  <td>{v.branchName}</td>
                  <td>
                    <StatusBadge status={v.log.result} />
                    {v.log.reasonCode ? (
                      <span className="ml-1 text-xs text-danger">{v.log.reasonCode.replaceAll('_', ' ')}</span>
                    ) : null}
                  </td>
                  <td className="text-right font-bold">{v.log.creditDelta || '—'}</td>
                  <td className="text-muted">{v.log.mode}</td>
                  <td className="text-right">
                    {v.log.result === 'CONFLICT' && can('access.simulate') ? (
                      <div className="flex justify-end gap-2">
                        <button
                          className="a-btn !px-2.5 !py-1 text-xs"
                          disabled={resolve.isPending}
                          onClick={() => resolve.mutate({ logId: v.log.id, action: 'APPROVE' })}
                        >
                          Approve
                        </button>
                        <button
                          className="a-btn-danger !px-2.5 !py-1 text-xs"
                          disabled={resolve.isPending}
                          onClick={() => resolve.mutate({ logId: v.log.id, action: 'REJECT' })}
                        >
                          Reject
                        </button>
                      </div>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pager
            page={page}
            pageCount={Math.max(1, Math.ceil((logs ?? []).filter((v) => !memberQuery || (v.memberName ?? '').toLowerCase().includes(memberQuery.toLowerCase())).length / 12))}
            onPage={setPage}
          />
        </div>
      )}
    </div>
  );
}
