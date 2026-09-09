'use client';

import { Spinner, StatusBadge, formatDay, formatDayTime, gateReasonLabel } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';
import { ErrorNote, PageTitle, Pager, StatCard, StatRow } from '../../../../components/ui';
import { FilterBar, FilterSelect, useFilters } from '../../../../components/filters';

const LOG_FILTERS = { gateId: '', result: '', mode: '', q: '', on: '' };

export default function AccessLogsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  // In the URL so the reports can link here already narrowed — "the denials"
  // and "the offline scans" are the two questions this screen gets asked.
  const { filters, set, clear, dirty } = useFilters(LOG_FILTERS);
  const { gateId, result, mode, q: memberQuery, on } = filters;
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
      <PageTitle
        title="Access Logs"
        subtitle="Every gate decision, incl. offline fallback & re-sync - approving a conflict deducts the booked class"
      />
      <ErrorNote message={error} />
      <StatRow>
        <StatCard tone="ink" label="Offline transactions" value={offline.length} />
        <StatCard tone="ok" label="Synced" value={synced} />
        <StatCard label="Conflicts" value={conflicts} tone={conflicts > 0 ? 'danger' : undefined} hint="Need manual reconciliation" />
      </StatRow>
      <FilterBar
        dirty={dirty}
        onClear={clear}
        chips={[
          ...(gateId
            ? [{ key: 'gateId', label: (gates ?? []).find((g) => g.id === gateId)?.name ?? gateId, onRemove: () => set('gateId', '') }]
            : []),
          ...(result ? [{ key: 'result', label: result, onRemove: () => set('result', '') }] : []),
          ...(mode ? [{ key: 'mode', label: mode, onRemove: () => set('mode', '') }] : []),
          ...(on ? [{ key: 'on', label: formatDay(on), onRemove: () => set('on', '') }] : []),
          ...(memberQuery ? [{ key: 'q', label: `"${memberQuery}"`, onRemove: () => set('q', '') }] : []),
        ]}
      >
        <input
          className="a-input max-w-xs"
          placeholder="Search member…"
          value={memberQuery}
          onChange={(e) => {
            set('q', e.target.value);
            setPage(0);
          }}
        />
        <FilterSelect
          value={gateId}
          onChange={(v) => set('gateId', v)}
          emptyLabel="All gates"
          options={(gates ?? []).map((g) => ({ value: g.id, label: g.name }))}
        />
        <FilterSelect
          value={result}
          onChange={(v) => set('result', v)}
          emptyLabel="All results"
          width="w-40"
          options={[
            { value: 'ALLOWED', label: 'Allowed' },
            { value: 'DENIED', label: 'Denied' },
            { value: 'CONFLICT', label: 'Conflict' },
          ]}
        />
        <FilterSelect
          value={mode}
          onChange={(v) => set('mode', v)}
          emptyLabel="All modes"
          width="w-40"
          options={[
            { value: 'ONLINE', label: 'Online' },
            { value: 'OFFLINE', label: 'Offline' },
          ]}
        />
        {/* A single day, because the busiest-day row in Reports links here. */}
        <input
          type="date"
          className="a-input w-40"
          value={on}
          onChange={(e) => {
            set('on', e.target.value);
            setPage(0);
          }}
        />
      </FilterBar>
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
                .filter((v) => !on || v.log.createdAt.slice(0, 10) === on)
                .slice(page * 12, page * 12 + 12)
                .map((v) => (
                <tr key={v.log.id}>
                  <td className="whitespace-nowrap text-muted">{formatDayTime(v.log.createdAt)}</td>
                  <td className="font-bold">{v.memberName ?? '-'}</td>
                  <td>{v.gateName}</td>
                  <td>{v.branchName}</td>
                  <td>
                    <StatusBadge status={v.log.result} />
                    {v.log.reasonCode ? (
                      <span className="ml-1 text-xs text-danger" title={gateReasonLabel(v.log.reasonCode)}>
                        {v.log.reasonCode.replaceAll('_', ' ')}
                      </span>
                    ) : null}
                  </td>
                  <td className="text-right font-bold">{v.log.creditDelta || '-'}</td>
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
            pageCount={Math.max(1, Math.ceil((logs ?? []).filter((v) => !memberQuery || (v.memberName ?? '').toLowerCase().includes(memberQuery.toLowerCase()))
                .filter((v) => !on || v.log.createdAt.slice(0, 10) === on).length / 12))}
            onPage={setPage}
          />
        </div>
      )}
    </div>
  );
}
