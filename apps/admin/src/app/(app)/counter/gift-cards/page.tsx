'use client';

import { formatDayTime, formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { CreditCard, Snowflake } from 'lucide-react';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  QueryError,
  SearchSelect,
  StatCard,
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

/**
 * Money somebody has already paid for.
 *
 * Deliberately not the credit wallet: credits buy classes and are a liability
 * measured in sessions, while a card is money spendable on anything at the
 * counter. Keeping them apart is what makes "what do we owe" answerable.
 */
export default function GiftCardsPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [query, setQuery] = useState('');
  const [issuing, setIssuing] = useState(false);
  const [open, setOpen] = useState<string | null>(null);

  const { data: cards, isLoading, error } = useQuery({
    queryKey: ['pos', 'gift-cards'],
    queryFn: () => api.admin.pos.giftCards.list({ limit: 200 }),
  });

  const q = query.trim().toLowerCase();
  const rows = (cards ?? []).filter(
    (c) => !q || c.code.toLowerCase().includes(q) || (c.barcode ?? '').includes(q),
  );
  const live = rows.filter((c) => c.status === 'ACTIVE');

  return (
    <div>
      <PageTitle
        title="Gift cards"
        subtitle="Money already paid for, spendable at the counter"
        actions={
          can('pos.sell') ? (
            <button className="a-btn" onClick={() => setIssuing(true)}>
              <CreditCard size={16} /> Issue a card
            </button>
          ) : null
        }
      />
      <QueryError error={error} />

      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <StatCard tone="ink" label="Cards" value={rows.length} icon={CreditCard} />
        <StatCard label="Live" value={live.length} tone="brand" />
        <StatCard tone="warn"
          label="Outstanding"
          value={formatIdr(live.reduce((s, c) => s + c.balanceIdr, 0))}
          hint="What the shop still owes on cards"
        />
        <StatCard
          label="Frozen"
          value={rows.filter((c) => c.status === 'FROZEN').length}
          icon={Snowflake}
        />
      </div>

      <div className="mb-4">
        <input
          className="a-input max-w-xs font-mono"
          placeholder="Search or scan a card…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      {isLoading ? (
        <Spinner label="Loading cards…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Card</th>
                  <th>Belongs to</th>
                  <th>Issued</th>
                  <th>Expires</th>
                  <th className="text-right">Issued with</th>
                  <th className="text-right">Left</th>
                  <th>Status</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {rows.map((card) => (
                  <tr key={card.id}>
                    <td className="font-mono font-bold">{card.code}</td>
                    <td className="text-sm text-muted">
                      {card.memberId ? 'a named member' : 'whoever holds it'}
                    </td>
                    <td className="text-sm">{card.issuedOn}</td>
                    <td className="text-sm text-muted">{card.expiresOn ?? 'never'}</td>
                    <td className="text-right tabular-nums text-sm text-muted">
                      {formatIdr(card.initialIdr)}
                    </td>
                    <td className="text-right tabular-nums font-bold">
                      {formatIdr(card.balanceIdr)}
                    </td>
                    <td>
                      <StatusBadge status={card.status} />
                    </td>
                    <td className="text-right">
                      <button
                        className="a-btn-ghost !px-3 !py-1 text-xs"
                        onClick={() => setOpen(card.code)}
                      >
                        Open
                      </button>
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="py-8 text-center text-sm text-muted">
                      No cards issued.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {issuing ? (
        <IssueModal
          onClose={() => setIssuing(false)}
          onDone={() => {
            setIssuing(false);
            void qc.invalidateQueries({ queryKey: ['pos', 'gift-cards'] });
          }}
        />
      ) : null}

      {open ? (
        <CardSheet
          code={open}
          canManage={can('pos.manage')}
          canSell={can('pos.sell')}
          onClose={() => setOpen(null)}
          onChanged={() => void qc.invalidateQueries({ queryKey: ['pos', 'gift-cards'] })}
        />
      ) : null}
    </div>
  );
}

function IssueModal({ onClose, onDone }: { onClose: () => void; onDone: () => void }) {
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState({
    code: '',
    barcode: '',
    memberId: '',
    amountIdr: '',
    expiresOn: '',
    note: '',
  });

  const { data: members } = useQuery({
    queryKey: ['members', 'all'],
    queryFn: () => api.admin.members.list(),
  });

  const save = useMutation({
    mutationFn: () =>
      api.admin.pos.giftCards.issue({
        code: form.code || undefined,
        barcode: form.barcode || null,
        memberId: form.memberId || null,
        amountIdr: Number(form.amountIdr),
        expiresOn: form.expiresOn || null,
        note: form.note || null,
      }),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That card did not issue.'),
  });

  return (
    <Modal title="Issue a gift card" onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Card number" hint="Leave empty and one is generated.">
            <input
              className="a-input font-mono"
              value={form.code}
              onChange={(e) => setForm((f) => ({ ...f, code: e.target.value.toUpperCase() }))}
            />
          </Field>
          <Field label="Barcode" hint="If the card is printed with one.">
            <input
              className="a-input font-mono"
              value={form.barcode}
              onChange={(e) => setForm((f) => ({ ...f, barcode: e.target.value }))}
            />
          </Field>
        </div>
        <Field label="Amount on it">
          <input
            className="a-input max-w-[14rem]"
            type="number"
            value={form.amountIdr}
            onChange={(e) => setForm((f) => ({ ...f, amountIdr: e.target.value }))}
          />
        </Field>
        <Field
          label="Belongs to"
          hint="A named card needs its owner on the sale. Leave empty for a gift anybody can spend."
        >
          <SearchSelect
            value={form.memberId}
            onChange={(v) => setForm((f) => ({ ...f, memberId: v }))}
            allowEmpty
            emptyLabel="Whoever holds it"
            placeholder="Search member…"
            options={(members ?? []).map((m) => ({ value: m.member.id, label: m.member.fullName }))}
          />
        </Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Expires" hint="Leave empty if it does not lapse.">
            <input
              className="a-input"
              type="date"
              value={form.expiresOn}
              onChange={(e) => setForm((f) => ({ ...f, expiresOn: e.target.value }))}
            />
          </Field>
          <Field label="Note">
            <input
              className="a-input"
              value={form.note}
              onChange={(e) => setForm((f) => ({ ...f, note: e.target.value }))}
            />
          </Field>
        </div>
        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || Number(form.amountIdr) <= 0}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Issuing…' : 'Issue it'}
          </button>
        </div>
      </div>
    </Modal>
  );
}

function CardSheet({
  code,
  canManage,
  canSell,
  onClose,
  onChanged,
}: {
  code: string;
  canManage: boolean;
  canSell: boolean;
  onClose: () => void;
  onChanged: () => void;
}) {
  const qc = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const [topUp, setTopUp] = useState('');

  const { data: card, isLoading } = useQuery({
    queryKey: ['pos', 'gift-card', code],
    queryFn: () => api.admin.pos.giftCards.get(code),
  });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ['pos', 'gift-card', code] });
    onChanged();
  };

  const add = useMutation({
    mutationFn: () => api.admin.pos.giftCards.topUp(code, Number(topUp)),
    onSuccess: () => {
      setError(null);
      setTopUp('');
      refresh();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That top-up did not save.'),
  });

  const setStatus = useMutation({
    mutationFn: (status: string) => api.admin.pos.giftCards.setStatus(code, status),
    onSuccess: refresh,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That did not save.'),
  });

  if (isLoading || !card) {
    return (
      <Modal title="Gift card" onClose={onClose}>
        <Spinner label="Loading…" />
      </Modal>
    );
  }

  return (
    <Modal title={card.code} onClose={onClose} wide>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-3">
          <StatCard label="Left on it" value={formatIdr(card.balanceIdr)} tone="brand" />
          <StatCard label="Issued with" value={formatIdr(card.initialIdr)} />
          <StatCard label="Status" value={<StatusBadge status={card.status} />} />
        </div>

        {card.memberId ? (
          <p className="rounded-lg bg-surface-raised px-3 py-2 text-xs text-muted">
            This card belongs to {card.memberName || 'a member'}. It only pays for a sale with them
            named on it — otherwise the name would be decoration.
          </p>
        ) : null}

        {canSell && card.status === 'ACTIVE' ? (
          <div className="grid gap-2 sm:grid-cols-[1fr_auto]">
            <input
              className="a-input"
              placeholder="Top up by…"
              value={topUp}
              onChange={(e) => setTopUp(e.target.value)}
            />
            <button className="a-btn" disabled={Number(topUp) <= 0 || add.isPending} onClick={() => add.mutate()}>
              Top up
            </button>
          </div>
        ) : null}

        <div className="a-card !p-0">
          <table className="a-table">
            <thead>
              <tr>
                <th>What happened</th>
                <th className="text-right">Amount</th>
                <th className="text-right">Left after</th>
                <th>When</th>
              </tr>
            </thead>
            <tbody>
              {card.entries.map((entry) => (
                <tr key={entry.id}>
                  <td className="text-sm font-bold">{entry.kind.toLowerCase().replace('_', ' ')}</td>
                  <td
                    className={`text-right tabular-nums ${
                      entry.amountIdr < 0 ? 'text-danger' : ''
                    }`}
                  >
                    {entry.amountIdr < 0 ? '−' : '+'}
                    {formatIdr(Math.abs(entry.amountIdr))}
                  </td>
                  <td className="text-right tabular-nums text-sm text-muted">
                    {formatIdr(entry.balanceAfter)}
                  </td>
                  <td className="text-sm text-muted">{formatDayTime(entry.createdAt)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="mt-1 flex justify-end gap-2">
          {canManage && card.status === 'ACTIVE' ? (
            <button className="a-btn-ghost" onClick={() => setStatus.mutate('FROZEN')}>
              <Snowflake size={14} /> Freeze
            </button>
          ) : null}
          {canManage && card.status === 'FROZEN' ? (
            <button className="a-btn-ghost" onClick={() => setStatus.mutate('ACTIVE')}>
              Unfreeze
            </button>
          ) : null}
          <button className="a-btn-ghost" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </Modal>
  );
}
