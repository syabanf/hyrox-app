'use client';

import type { PromotionView, UpsertPromotionInput } from '@nuhabit/contracts';
import {
  CHANNEL_LABELS,
  PROMOTION_KINDS,
  PROMOTION_KIND_LABELS,
  SALES_CHANNELS,
  type PromotionKind,
} from '@nuhabit/domain';
import { formatIdr, Spinner, StatusBadge } from '@nuhabit/ui';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Pencil, Tag } from 'lucide-react';
import { useState } from 'react';
import {
  ErrorNote,
  Field,
  Modal,
  PageTitle,
  QueryError,
  RowActions,
  SearchSelect,
  StatCard, StatRow,
} from '../../../../components/ui';
import { api, ApiError } from '../../../../lib/api';
import { usePermissions } from '../../../../lib/auth';

/**
 * Offers the till applies by itself.
 *
 * A price break says what one product costs at a quantity. An offer is about a
 * basket, and it has a start, an end and somebody's decision behind it — which
 * is why they live apart and why an offer can be switched off on Monday
 * without repricing the catalogue.
 */
export default function OffersPage() {
  const qc = useQueryClient();
  const { can } = usePermissions();
  const [editing, setEditing] = useState<PromotionView | 'new' | null>(null);

  const { data: promotions, isLoading, error } = useQuery({
    queryKey: ['pos', 'promotions'],
    queryFn: () => api.admin.pos.promotions.list(),
  });

  const rows = promotions ?? [];
  const live = rows.filter((p) => p.active);

  return (
    <div>
      <PageTitle
        title="Offers"
        subtitle="What the till takes off by itself, and what somebody has to ask for"
        actions={
          can('pos.manage') ? (
            <button className="a-btn" onClick={() => setEditing('new')}>
              + New offer
            </button>
          ) : null
        }
      />
      <QueryError error={error} />

      <StatRow>
        <StatCard tone="ink" label="Offers" value={rows.length} icon={Tag} />
        <StatCard label="Running" value={live.length} tone="brand" />
        <StatCard
          tone="info"
          label="Automatic"
          value={live.filter((p) => !p.requiresCode).length}
          hint="Applied without being asked for"
        />
        <StatCard
          label="Exclusive"
          value={live.filter((p) => p.exclusive).length}
          hint="Cannot combine with anything else"
        />
      </StatRow>

      {isLoading ? (
        <Spinner label="Loading offers…" />
      ) : (
        <div className="a-card !p-0">
          <div className="overflow-x-auto">
            <table className="a-table">
              <thead>
                <tr>
                  <th>Offer</th>
                  <th>What it does</th>
                  <th>Applies to</th>
                  <th>When</th>
                  <th className="text-right">Used</th>
                  <th>Status</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((promotion) => (
                  <tr key={promotion.id}>
                    <td>
                      <p className="font-bold">{promotion.name}</p>
                      <p className="font-mono text-xs text-muted">
                        {promotion.code}
                        {promotion.requiresCode ? ' · asked for by name' : ''}
                        {promotion.exclusive ? ' · exclusive' : ''}
                      </p>
                    </td>
                    <td className="text-sm">{describe(promotion)}</td>
                    <td className="text-sm text-muted">
                      {promotion.targets.length === 0
                        ? 'the whole basket'
                        : `${promotion.targets.length} product${promotion.targets.length === 1 ? '' : 's'}`}
                      {promotion.channels.length > 0
                        ? ` · ${promotion.channels.map((c) => c.toLowerCase()).join(', ')}`
                        : ''}
                    </td>
                    <td className="text-sm text-muted">
                      {promotion.startsOn || promotion.endsOn
                        ? `${promotion.startsOn ?? '…'} → ${promotion.endsOn ?? '…'}`
                        : 'always'}
                    </td>
                    <td className="text-right tabular-nums text-sm">
                      {promotion.usedCount}
                      {promotion.maxUses ? ` / ${promotion.maxUses}` : ''}
                    </td>
                    <td>
                      <StatusBadge status={promotion.active ? 'ACTIVE' : 'INACTIVE'} />
                    </td>
                    <td className="text-right">
                      <RowActions
                        items={[
                          {
                            label: 'Edit',
                            icon: Pencil,
                            disabled: !can('pos.manage'),
                            onClick: () => setEditing(promotion),
                          },
                        ]}
                      />
                    </td>
                  </tr>
                ))}
                {rows.length === 0 ? (
                  <tr>
                    <td colSpan={7} className="py-8 text-center text-sm text-muted">
                      No offers yet.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {editing ? (
        <OfferModal
          promotion={editing === 'new' ? null : editing}
          onClose={() => setEditing(null)}
          onDone={() => {
            setEditing(null);
            void qc.invalidateQueries({ queryKey: ['pos', 'promotions'] });
          }}
        />
      ) : null}
    </div>
  );
}

/** What an offer does, in the words a person would use. */
function describe(promotion: PromotionView): string {
  switch (promotion.kind) {
    case 'PERCENT':
      return `${promotion.percent}% off`;
    case 'AMOUNT':
      return `${formatIdr(promotion.amountIdr ?? 0)} off`;
    case 'BUY_X_GET_Y':
      return `buy ${promotion.buyQty}, get ${promotion.freeQty} free`;
    case 'BUNDLE':
      return `the set for ${formatIdr(promotion.bundlePriceIdr ?? 0)}`;
  }
}

function OfferModal({
  promotion,
  onClose,
  onDone,
}: {
  promotion: PromotionView | null;
  onClose: () => void;
  onDone: () => void;
}) {
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState<UpsertPromotionInput>(() => ({
    code: promotion?.code ?? '',
    name: promotion?.name ?? '',
    description: promotion?.description ?? '',
    kind: promotion?.kind ?? 'PERCENT',
    percent: promotion?.percent ?? 10,
    amountIdr: promotion?.amountIdr ?? null,
    buyQty: promotion?.buyQty ?? null,
    freeQty: promotion?.freeQty ?? null,
    bundlePriceIdr: promotion?.bundlePriceIdr ?? null,
    minSpendIdr: promotion?.minSpendIdr ?? 0,
    requiresCode: promotion?.requiresCode ?? false,
    channels: promotion?.channels ?? [],
    exclusive: promotion?.exclusive ?? false,
    priority: promotion?.priority ?? 0,
    startsOn: promotion?.startsOn ?? null,
    endsOn: promotion?.endsOn ?? null,
    maxUsesPerMember: promotion?.maxUsesPerMember ?? null,
    active: promotion?.active ?? true,
    targets: (promotion?.targets ?? []).map((t) => ({
      productId: t.productId,
      categoryId: t.categoryId,
      qty: t.qty,
    })),
  }));
  const set = <K extends keyof UpsertPromotionInput>(k: K, v: UpsertPromotionInput[K]) =>
    setForm((f) => ({ ...f, [k]: v }));

  const { data: products } = useQuery({
    queryKey: ['pos', 'products'],
    queryFn: () => api.admin.pos.products.list(),
  });

  const save = useMutation({
    mutationFn: () => api.admin.pos.promotions.save(form),
    onSuccess: onDone,
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That offer did not save.'),
  });

  const kind = form.kind as PromotionKind;
  const targets = form.targets ?? [];

  return (
    <Modal title={promotion ? `Edit ${promotion.name}` : 'New offer'} onClose={onClose}>
      <div className="grid gap-3">
        <ErrorNote message={error} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Code" hint="What a customer would say to claim it.">
            <input
              className="a-input font-mono"
              value={form.code}
              onChange={(e) => set('code', e.target.value.toUpperCase())}
            />
          </Field>
          <Field label="Name" hint="What the receipt calls it.">
            <input className="a-input" value={form.name} onChange={(e) => set('name', e.target.value)} />
          </Field>
        </div>

        <Field label="What it does">
          <SearchSelect
            value={form.kind}
            onChange={(v) => set('kind', v)}
            placeholder="Search…"
            options={[
              ...PROMOTION_KINDS.map((k) => ({ value: k, label: PROMOTION_KIND_LABELS[k] })),
            ]}
          />
        </Field>

        {kind === 'PERCENT' ? (
          <Field label="Percent off">
            <input
              className="a-input max-w-[10rem]"
              type="number"
              value={form.percent ?? 0}
              onChange={(e) => set('percent', Number(e.target.value))}
            />
          </Field>
        ) : null}
        {kind === 'AMOUNT' ? (
          <Field label="Amount off" hint="Never more than the goods it targets are worth.">
            <input
              className="a-input max-w-[14rem]"
              type="number"
              value={form.amountIdr ?? 0}
              onChange={(e) => set('amountIdr', Number(e.target.value))}
            />
          </Field>
        ) : null}
        {kind === 'BUY_X_GET_Y' ? (
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Buy">
              <input
                className="a-input"
                type="number"
                value={form.buyQty ?? 2}
                onChange={(e) => set('buyQty', Number(e.target.value))}
              />
            </Field>
            <Field label="Get free" hint="The cheapest of each group is the free one.">
              <input
                className="a-input"
                type="number"
                value={form.freeQty ?? 1}
                onChange={(e) => set('freeQty', Number(e.target.value))}
              />
            </Field>
          </div>
        ) : null}
        {kind === 'BUNDLE' ? (
          <Field label="Price for the whole set" hint="The set has to be complete, or it does not apply.">
            <input
              className="a-input max-w-[14rem]"
              type="number"
              value={form.bundlePriceIdr ?? 0}
              onChange={(e) => set('bundlePriceIdr', Number(e.target.value))}
            />
          </Field>
        ) : null}

        <Field
          label="Applies to"
          hint="Leave empty for the whole basket. A bundle needs every product it contains."
        >
          <div className="grid gap-2">
            {targets.map((target, index) => (
              <div key={index} className="flex items-center gap-2">
                <SearchSelect
                  value={target.productId ?? ''}
                  onChange={(v) =>
                    set(
                      'targets',
                      targets.map((t, i) => (i === index ? { ...t, productId: v || null } : t)),
                    )
                  }
                  placeholder="Search product…"
                  options={(products ?? []).map((p) => ({ value: p.id, label: p.name, hint: p.sku }))}
                />
                {kind === 'BUNDLE' ? (
                  <input
                    className="a-input w-20"
                    type="number"
                    min={1}
                    value={target.qty ?? 1}
                    onChange={(e) =>
                      set(
                        'targets',
                        targets.map((t, i) =>
                          i === index ? { ...t, qty: Number(e.target.value) } : t,
                        ),
                      )
                    }
                  />
                ) : null}
                <button
                  className="a-btn-ghost !px-2 !py-1 text-xs"
                  onClick={() => set('targets', targets.filter((_, i) => i !== index))}
                >
                  Remove
                </button>
              </div>
            ))}
            <button
              className="a-btn-ghost self-start !px-3 !py-1 text-xs"
              onClick={() => set('targets', [...targets, { productId: null, categoryId: null, qty: 1 }])}
            >
              + Product
            </button>
          </div>
        </Field>

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Runs from">
            <input
              className="a-input"
              type="date"
              value={form.startsOn ?? ''}
              onChange={(e) => set('startsOn', e.target.value || null)}
            />
          </Field>
          <Field label="Until">
            <input
              className="a-input"
              type="date"
              value={form.endsOn ?? ''}
              onChange={(e) => set('endsOn', e.target.value || null)}
            />
          </Field>
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Minimum basket" hint="Below this it does not apply at all.">
            <input
              className="a-input"
              type="number"
              value={form.minSpendIdr ?? 0}
              onChange={(e) => set('minSpendIdr', Number(e.target.value))}
            />
          </Field>
          <Field label="Once per member" hint="Leave empty for no limit.">
            <input
              className="a-input"
              type="number"
              value={form.maxUsesPerMember ?? ''}
              onChange={(e) =>
                set('maxUsesPerMember', e.target.value ? Number(e.target.value) : null)
              }
            />
          </Field>
        </div>

        <Field label="Which channels" hint="None chosen means every channel.">
          <div className="flex flex-wrap gap-2">
            {SALES_CHANNELS.map((channel) => {
              const on = (form.channels ?? []).includes(channel);
              return (
                <button
                  key={channel}
                  onClick={() =>
                    set(
                      'channels',
                      on
                        ? (form.channels ?? []).filter((c) => c !== channel)
                        : [...(form.channels ?? []), channel],
                    )
                  }
                  className={`rounded-full px-3 py-1.5 text-xs font-bold transition ${
                    on ? 'bg-brand text-white' : 'bg-surface-raised text-muted'
                  }`}
                >
                  {CHANNEL_LABELS[channel]}
                </button>
              );
            })}
          </div>
        </Field>

        <label className="flex items-start gap-2 text-sm font-bold">
          <input
            type="checkbox"
            className="mt-1"
            checked={form.requiresCode ?? false}
            onChange={(e) => set('requiresCode', e.target.checked)}
          />
          <span>
            Has to be asked for by name
            <span className="block text-xs font-normal text-muted">
              Otherwise it applies to every qualifying basket, without anybody asking.
            </span>
          </span>
        </label>
        <label className="flex items-start gap-2 text-sm font-bold">
          <input
            type="checkbox"
            className="mt-1"
            checked={form.exclusive ?? false}
            onChange={(e) => set('exclusive', e.target.checked)}
          />
          <span>
            Cannot combine with other offers
            <span className="block text-xs font-normal text-muted">
              The till gives the customer this alone or everything else together, whichever saves
              them more. Without it, generous offers can stack into a sale below cost.
            </span>
          </span>
        </label>
        <label className="flex items-center gap-2 text-sm font-bold">
          <input
            type="checkbox"
            checked={form.active ?? true}
            onChange={(e) => set('active', e.target.checked)}
          />
          Running
        </label>

        <div className="mt-2 flex justify-end gap-2">
          <button className="a-btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            className="a-btn"
            disabled={save.isPending || !form.code || !form.name}
            onClick={() => save.mutate()}
          >
            {save.isPending ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </Modal>
  );
}
