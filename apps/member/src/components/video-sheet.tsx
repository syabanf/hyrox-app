import { X } from 'lucide-react';

/** Bottom sheet playing an exercise how-to video (YouTube embed). */
export function VideoSheet({
  title,
  videoUrl,
  onClose,
}: {
  title: string;
  videoUrl: string;
  onClose: () => void;
}) {
  const embedUrl = videoUrl.replace('watch?v=', 'embed/');
  return (
    <div className="sheet-backdrop fixed inset-0 z-40 flex items-end justify-center bg-black/60" onClick={onClose}>
      <div
        className="sheet-panel w-full max-w-md rounded-t-3xl bg-surface p-5 pb-8"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-3 flex items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="text-[11px] font-extrabold uppercase tracking-[0.16em] text-muted">
              How to perform
            </p>
            <h2 className="display truncate text-xl">{title}</h2>
          </div>
          <button
            onClick={onClose}
            className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-surface-raised"
            aria-label="Close"
          >
            <X size={16} />
          </button>
        </div>
        <div className="overflow-hidden rounded-2xl bg-ink">
          <iframe
            src={embedUrl}
            title={`How to perform ${title}`}
            className="aspect-video w-full"
            allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
            allowFullScreen
          />
        </div>
        <p className="mt-2.5 text-center text-xs text-muted">
          Video opens from YouTube — technique first, speed second.
        </p>
      </div>
    </div>
  );
}
