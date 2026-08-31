import { Spinner } from '@hyrox/ui';
import { ArrowLeft } from 'lucide-react';
import { useNavigate } from 'react-router';
import { GeoMap } from '../../components/geo-map';
import { useHeatmap } from '../../lib/athlete-queries';

export function HeatmapPage() {
  const navigate = useNavigate();
  const { data, isLoading } = useHeatmap();

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>
      <div>
        <h1 className="display text-3xl">Personal heatmap</h1>
        <p className="text-sm text-muted">Every GPS track you have recorded, on one map.</p>
      </div>
      {isLoading || !data ? (
        <Spinner label="Painting your tracks…" />
      ) : data.tracks.length === 0 ? (
        <p className="card text-sm text-muted">No GPS activities yet.</p>
      ) : (
        <GeoMap
          tracks={data.tracks.map((points) => ({
            points,
            opacity: 0.4,
            width: 3.5,
            markers: false,
          }))}
          height={440}
        />
      )}
    </div>
  );
}
