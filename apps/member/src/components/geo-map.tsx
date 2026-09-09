import type { TrackPoint } from '@nuhabit/domain';
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
import Circle from 'ol/geom/Circle';
import { Crosshair, LoaderCircle } from 'lucide-react';
import { useEffect, useRef } from 'react';
import { useMyLocation } from '../lib/geolocation';

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
  showMe = false,
}: {
  tracks: GeoTrack[];
  height?: number;
  interactive?: boolean;
  className?: string;
  /**
   * Offer a "centre on me" control. Off by default: a map of last Tuesday's
   * run has no business asking where the reader is standing.
   */
  showMe?: boolean;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<Map | null>(null);
  const meRef = useRef<VectorSource | null>(null);
  const me = useMyLocation();

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
            color: track.color ?? `rgba(0, 40, 26, ${track.opacity ?? 1})`,
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
              fill: new Fill({ color: '#abde67' }),
              stroke: new Stroke({ color: '#fff', width: 2 }),
            }),
          }),
        );
        const end = new Feature(new Point(coords[coords.length - 1]!));
        end.setStyle(
          new Style({
            image: new CircleStyle({
              radius: 6,
              fill: new Fill({ color: '#131a1c' }),
              stroke: new Stroke({ color: '#fff', width: 2 }),
            }),
          }),
        );
        source.addFeatures([start, end]);
      }
    }

    // A separate source for the device's own position, so redrawing the
    // tracks never wipes the dot and vice versa.
    const mine = new VectorSource();
    meRef.current = mine;

    const map = new Map({
      target: container,
      layers: [
        new TileLayer({ source: new OSM() }),
        new VectorLayer({ source }),
        new VectorLayer({ source: mine }),
      ],
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
      meRef.current = null;
    };
    // Track identity churns per fetch; re-render on the serialized geometry instead.
  }, [JSON.stringify(tracks.map((t) => [t.points.length, t.points[0]?.lat, t.color])), interactive]);

  // The blue dot, and the circle the browser is prepared to stand behind.
  // Drawing the accuracy is not decoration: a 2km fix off a wifi lookup looks
  // exactly like a 5m one off GPS until you show the radius.
  useEffect(() => {
    const source = meRef.current;
    const map = mapRef.current;
    if (!source || !map || !me.fix) return;

    source.clear();
    const centre = fromLonLat([me.fix.lng, me.fix.lat]);
    const halo = new Feature(new Circle(centre, me.fix.accuracyM));
    halo.setStyle(
      new Style({
        fill: new Fill({ color: 'rgba(37, 99, 235, 0.12)' }),
        stroke: new Stroke({ color: 'rgba(37, 99, 235, 0.35)', width: 1 }),
      }),
    );
    const dot = new Feature(new Point(centre));
    dot.setStyle(
      new Style({
        image: new CircleStyle({
          radius: 7,
          fill: new Fill({ color: '#2563eb' }),
          stroke: new Stroke({ color: '#fff', width: 2.5 }),
        }),
      }),
    );
    source.addFeatures([halo, dot]);
    map.getView().animate({ center: centre, zoom: Math.max(map.getView().getZoom() ?? 14, 15), duration: 400 });
  }, [me.fix]);

  return (
    <div
      className={`relative w-full overflow-hidden rounded-xl border border-line bg-surface-raised ${className}`}
      style={{ height }}
    >
      <div ref={containerRef} className="h-full w-full" />
      {showMe ? (
        <button
          type="button"
          onClick={() => void me.request()}
          disabled={me.locating}
          title={me.error ?? 'Centre on my location'}
          aria-label="Centre on my location"
          className="absolute bottom-3 right-3 flex h-10 w-10 items-center justify-center rounded-full border border-line bg-surface text-ink shadow-md transition active:scale-95 disabled:opacity-60"
        >
          {me.locating ? (
            <LoaderCircle size={18} className="animate-spin" />
          ) : (
            <Crosshair size={18} className={me.fix ? 'text-brand' : ''} />
          )}
        </button>
      ) : null}
      {/* A refusal has to say something, or the button just looks broken. */}
      {showMe && me.error ? (
        <p className="absolute bottom-3 left-3 right-16 rounded-lg bg-ink/85 px-2.5 py-1.5 text-[11px] leading-tight text-white">
          {me.error}
        </p>
      ) : null}
    </div>
  );
}
