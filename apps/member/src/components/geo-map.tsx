import type { TrackPoint } from '@hyrox/domain';
import Feature from 'ol/Feature';
import Map from 'ol/Map';
import View from 'ol/View';
import LineString from 'ol/geom/LineString';
import Point from 'ol/geom/Point';
import TileLayer from 'ol/layer/Tile';
import VectorLayer from 'ol/layer/Vector';
import 'ol/ol.css';
import { fromLonLat } from 'ol/proj';
import OSM from 'ol/source/OSM';
import VectorSource from 'ol/source/Vector';
import CircleStyle from 'ol/style/Circle';
import Fill from 'ol/style/Fill';
import Stroke from 'ol/style/Stroke';
import Style from 'ol/style/Style';
import { useEffect, useRef } from 'react';

export interface GeoTrack {
  points: TrackPoint[];
  color?: string;
  width?: number;
  opacity?: number;
  /** Draw start/end markers (default true for single-track maps). */
  markers?: boolean;
}

/**
 * Real tile map (OpenLayers + OSM). Used on detail/route/heatmap views;
 * feed thumbnails stay on the lightweight SVG renderer.
 */
export function GeoMap({
  tracks,
  height = 220,
  interactive = true,
  className = '',
}: {
  tracks: GeoTrack[];
  height?: number;
  interactive?: boolean;
  className?: string;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<Map | null>(null);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const source = new VectorSource();
    for (const track of tracks) {
      if (track.points.length < 2) continue;
      const coords = track.points.map((p) => fromLonLat([p.lng, p.lat]));
      const line = new Feature(new LineString(coords));
      line.setStyle(
        new Style({
          stroke: new Stroke({
            color: track.color ?? `rgba(237, 28, 36, ${track.opacity ?? 1})`,
            width: track.width ?? 4,
            lineCap: 'round',
            lineJoin: 'round',
          }),
        }),
      );
      source.addFeature(line);
      if (track.markers ?? tracks.length === 1) {
        const start = new Feature(new Point(coords[0]!));
        start.setStyle(
          new Style({
            image: new CircleStyle({
              radius: 6,
              fill: new Fill({ color: '#34d27b' }),
              stroke: new Stroke({ color: '#fff', width: 2 }),
            }),
          }),
        );
        const end = new Feature(new Point(coords[coords.length - 1]!));
        end.setStyle(
          new Style({
            image: new CircleStyle({
              radius: 6,
              fill: new Fill({ color: '#191919' }),
              stroke: new Stroke({ color: '#fff', width: 2 }),
            }),
          }),
        );
        source.addFeatures([start, end]);
      }
    }

    const map = new Map({
      target: container,
      layers: [new TileLayer({ source: new OSM() }), new VectorLayer({ source })],
      controls: [],
      interactions: interactive ? undefined : [],
      view: new View({ center: fromLonLat([106.82, -6.2]), zoom: 12 }),
    });
    const extent = source.getExtent();
    if (extent && Number.isFinite(extent[0])) {
      map.getView().fit(extent, { padding: [28, 28, 28, 28], maxZoom: 16 });
    }
    mapRef.current = map;
    return () => {
      map.setTarget(undefined);
      mapRef.current = null;
    };
    // Track identity churns per fetch; re-render on the serialized geometry instead.
  }, [JSON.stringify(tracks.map((t) => [t.points.length, t.points[0]?.lat, t.color])), interactive]);

  return (
    <div
      ref={containerRef}
      className={`w-full overflow-hidden rounded-xl border border-line bg-surface-raised ${className}`}
      style={{ height }}
    />
  );
}
