import { Spinner, formatDayTime } from '@hyrox/ui';
import { useQuery } from '@tanstack/react-query';
import { ArrowLeft, Megaphone } from 'lucide-react';
import { Link, useNavigate, useParams } from 'react-router';
import { api } from '../../lib/api';

export function AnnouncementDetailPage() {
  const { announcementId = '' } = useParams();
  const navigate = useNavigate();
  const { data: a, isLoading } = useQuery({
    queryKey: ['announcement', announcementId],
    queryFn: () => api.me.announcement(announcementId),
  });

  if (isLoading || !a) return <Spinner label="Loading announcement…" />;

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>

      {a.imageUrl ? (
        <div className="card relative overflow-hidden !border-0 !p-0">
          <img src={a.imageUrl} alt="" className="h-52 w-full object-cover" />
        </div>
      ) : (
        <div className="card surface-ink relative overflow-hidden !border-0 !p-6 text-white">
          <div className="pointer-events-none absolute -right-16 -top-24 h-56 w-56 rounded-full bg-brand/25 blur-3xl" />
          <span className="relative flex h-12 w-12 items-center justify-center rounded-2xl bg-white/10">
            <Megaphone size={22} className="text-[#ff4348]" />
          </span>
        </div>
      )}

      <div>
        <p className="text-[11px] font-extrabold uppercase tracking-[0.16em] text-muted">
          Announcement · {formatDayTime(a.createdAt)}
        </p>
        <h1 className="display mt-1 text-3xl leading-tight">{a.title}</h1>
      </div>

      <p className="text-[15px] leading-relaxed text-ink/80">{a.message}</p>

      {a.deepLink ? (
        <Link to={a.deepLink} className="btn-brand">
          Open in the app
        </Link>
      ) : null}
    </div>
  );
}
