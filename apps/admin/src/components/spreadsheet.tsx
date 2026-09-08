'use client';

import type { ImportResult } from '@nuhabit/domain';
import { useMutation } from '@tanstack/react-query';
import { Download, Upload } from 'lucide-react';
import { useRef, useState } from 'react';
import { ErrorNote, Modal } from './ui';
import { ApiError } from '../lib/api';

/**
 * Spreadsheets in and out.
 *
 * Shared because the fiddly parts are the same everywhere: an import must say
 * what it would do before it does it, and a bad row has to be findable in the
 * file it came from.
 */

/** Downloads a blob the client fetched with its credentials attached. */
export function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  // Revoked on the next tick: releasing it immediately can cancel the download
  // in some browsers before it has started reading.
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export function ExportButton({
  label = 'Export',
  filename,
  fetcher,
}: {
  label?: string;
  filename: string;
  fetcher: () => Promise<Blob>;
}) {
  const [error, setError] = useState<string | null>(null);
  const run = useMutation({
    mutationFn: fetcher,
    onSuccess: (blob) => saveBlob(blob, `${filename}.csv`),
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That export failed.'),
  });

  return (
    <>
      <button className="a-btn-ghost" disabled={run.isPending} onClick={() => run.mutate()}>
        <Download size={15} /> {run.isPending ? 'Preparing…' : label}
      </button>
      {error ? <span className="text-xs text-danger">{error}</span> : null}
    </>
  );
}

/**
 * Upload, see what it would do, then apply it.
 *
 * Two steps on purpose. A three-hundred-row mistake is not something anybody
 * wants to find afterwards, and the dry run is the only chance to catch it.
 */
export function ImportButton({
  label = 'Import',
  title,
  hint,
  columns,
  importer,
  onDone,
}: {
  label?: string;
  title: string;
  hint: string;
  columns: string[];
  importer: (csv: string, apply: boolean) => Promise<ImportResult>;
  onDone: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [csv, setCsv] = useState('');
  const [filename, setFilename] = useState('');
  const [result, setResult] = useState<ImportResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const input = useRef<HTMLInputElement>(null);

  const run = useMutation({
    mutationFn: (apply: boolean) => importer(csv, apply),
    onSuccess: (next) => {
      setError(null);
      setResult(next);
      if (!next.dryRun) onDone();
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : 'That file could not be read.'),
  });

  const reset = () => {
    setCsv('');
    setFilename('');
    setResult(null);
    setError(null);
  };

  return (
    <>
      <button
        className="a-btn-ghost"
        onClick={() => {
          reset();
          setOpen(true);
        }}
      >
        <Upload size={15} /> {label}
      </button>

      {open ? (
        <Modal
          title={title}
          wide
          onClose={() => {
            setOpen(false);
            reset();
          }}
        >
          <div className="grid gap-3">
            <ErrorNote message={error} />
            <p className="text-sm text-muted">{hint}</p>
            <p className="rounded-lg bg-surface-raised px-3 py-2 text-xs text-muted">
              Columns it reads: <span className="font-mono">{columns.join(', ')}</span>. Order does
              not matter and extra columns are ignored. Numbers may be written 1.500.000 or
              1,500,000.
            </p>

            <input
              ref={input}
              type="file"
              accept=".csv,text/csv"
              className="a-input"
              onChange={async (e) => {
                const file = e.target.files?.[0];
                if (!file) return;
                setFilename(file.name);
                setResult(null);
                setCsv(await file.text());
              }}
            />

            {result ? (
              <div className="grid gap-2">
                <div className="grid gap-2 sm:grid-cols-4">
                  <Tally label="Rows" value={result.rows} />
                  <Tally label="New" value={result.created} tone="brand" />
                  <Tally label="Updated" value={result.updated} />
                  <Tally label="Skipped" value={result.skipped} tone={result.skipped > 0 ? 'danger' : undefined} />
                </div>
                {result.failures.length > 0 ? (
                  <div className="a-card !p-0">
                    <p className="px-3 pt-3 text-xs font-bold text-danger">
                      These rows will be skipped. The rest still import.
                    </p>
                    <table className="a-table mt-1">
                      <thead>
                        <tr>
                          <th>Line</th>
                          <th>Row</th>
                          <th>Why</th>
                        </tr>
                      </thead>
                      <tbody>
                        {result.failures.slice(0, 20).map((failure, i) => (
                          <tr key={i}>
                            <td className="tabular-nums">{failure.line}</td>
                            <td className="font-mono text-xs">{failure.subject || '—'}</td>
                            <td className="text-sm">{failure.message}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : null}
                {result.dryRun ? (
                  <p className="text-xs text-muted">
                    Nothing has been written yet. This is what would happen.
                  </p>
                ) : (
                  <p className="text-xs font-bold text-brand">Applied.</p>
                )}
              </div>
            ) : null}

            <div className="mt-1 flex justify-end gap-2">
              <button
                className="a-btn-ghost"
                onClick={() => {
                  setOpen(false);
                  reset();
                }}
              >
                {result && !result.dryRun ? 'Done' : 'Cancel'}
              </button>
              {!result || result.dryRun ? (
                <>
                  <button
                    className="a-btn-ghost"
                    disabled={!csv || run.isPending}
                    onClick={() => run.mutate(false)}
                  >
                    {run.isPending ? 'Checking…' : 'Check it'}
                  </button>
                  <button
                    className="a-btn"
                    disabled={!result || run.isPending}
                    title={result ? undefined : 'Check the file first'}
                    onClick={() => run.mutate(true)}
                  >
                    {run.isPending ? 'Importing…' : 'Import it'}
                  </button>
                </>
              ) : null}
            </div>
            {filename ? <p className="text-xs text-muted">{filename}</p> : null}
          </div>
        </Modal>
      ) : null}
    </>
  );
}

function Tally({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone?: 'brand' | 'danger';
}) {
  return (
    <div className="rounded-xl bg-surface-raised px-3 py-2">
      <p className="text-[11px] font-bold uppercase tracking-wider text-muted">{label}</p>
      <p
        className={`display text-xl font-black tabular-nums ${
          tone === 'danger' ? 'text-danger' : tone === 'brand' ? 'text-brand' : ''
        }`}
      >
        {value}
      </p>
    </div>
  );
}
