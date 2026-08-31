import { describe, expect, it } from 'vitest';
import type { Activity, Challenge, Segment, TrackPoint } from '../index';
import {
  canViewActivity,
  challengeProgressKm,
  computeActivityStats,
  findGroupedActivities,
  haversineM,
  matchSegments,
  personalRecords,
  weeklyBuckets,
} from '../index';

/** Straight-line track heading north at constant speed. */
function track(speedMps: number, seconds: number, sampleSec = 5): TrackPoint[] {
  const points: TrackPoint[] = [];
  const mPerDegLat = 111_320;
  for (let s = 0; s <= seconds; s += sampleSec) {
    points.push({ t: s * 1000, lat: -6.2 + (speedMps * s) / mPerDegLat, lng: 106.8 });
  }
  return points;
}

describe('haversineM', () => {
  it('measures ~111km per degree of latitude', () => {
    const d = haversineM({ lat: 0, lng: 0 }, { lat: 1, lng: 0 });
    expect(d).toBeGreaterThan(110_000);
    expect(d).toBeLessThan(112_000);
  });
  it('is zero for identical points', () => {
    expect(haversineM({ lat: -6.2, lng: 106.8 }, { lat: -6.2, lng: 106.8 })).toBe(0);
  });
});

describe('computeActivityStats', () => {
  it('computes distance, moving time and pace for a steady run', () => {
    // 3.333 m/s ≈ 5:00 /km for 10 minutes → ~2 km
    const stats = computeActivityStats(track(10 / 3, 600));
    expect(stats.distanceM).toBeGreaterThan(1950);
    expect(stats.distanceM).toBeLessThan(2050);
    expect(stats.elapsedSec).toBe(600);
    expect(stats.movingSec).toBe(600);
    expect(stats.avgPaceSecPerKm).toBeGreaterThan(290);
    expect(stats.avgPaceSecPerKm).toBeLessThan(310);
  });

  it('produces one full split per km plus a partial tail', () => {
    const stats = computeActivityStats(track(10 / 3, 750)); // ~2.5 km
    const full = stats.splits.filter((s) => s.full);
    expect(full.length).toBe(2);
    expect(stats.splits[stats.splits.length - 1]!.full).toBe(false);
    for (const s of full) {
      expect(s.paceSecPerKm).toBeGreaterThan(290);
      expect(s.paceSecPerKm).toBeLessThan(310);
    }
    expect(stats.bestSplitPaceSec).toBe(Math.min(...full.map((s) => s.paceSecPerKm)));
  });

  it('excludes stopped time from moving time', () => {
    const moving = track(10 / 3, 300); // 5 min moving
    const lastT = moving[moving.length - 1]!.t;
    const last = moving[moving.length - 1]!;
    // 2 minutes standing still, then nothing.
    const stopped: TrackPoint[] = [
      { t: lastT + 60_000, lat: last.lat, lng: last.lng },
      { t: lastT + 120_000, lat: last.lat, lng: last.lng },
    ];
    const stats = computeActivityStats([...moving, ...stopped]);
    expect(stats.elapsedSec).toBe(420);
    expect(stats.movingSec).toBe(300);
  });

  it('handles empty and single-point tracks', () => {
    expect(computeActivityStats([]).distanceM).toBe(0);
    expect(computeActivityStats([{ t: 0, lat: 0, lng: 0 }]).avgPaceSecPerKm).toBeNull();
  });
});

describe('visibility', () => {
  const activity = { memberId: 'mem_a', visibility: 'FOLLOWERS' as const };
  it('owner always sees their own activity', () => {
    expect(canViewActivity(activity, 'mem_a', () => false)).toBe(true);
  });
  it('followers-only requires following', () => {
    expect(canViewActivity(activity, 'mem_b', () => false)).toBe(false);
    expect(canViewActivity(activity, 'mem_b', () => true)).toBe(true);
  });
  it('private is owner-only, everyone is public', () => {
    expect(canViewActivity({ memberId: 'mem_a', visibility: 'PRIVATE' }, 'mem_b', () => true)).toBe(false);
    expect(canViewActivity({ memberId: 'mem_a', visibility: 'EVERYONE' }, 'mem_b', () => false)).toBe(true);
  });
});

describe('matchSegments (GPS polyline matching)', () => {
  // Segment: straight 1 km heading north from the corridor base.
  const segPath = track(10 / 3, 300); // 1000m at 3.33 m/s over 300s
  const segments: Segment[] = [
    { id: 's1', name: '1k Sprint', type: 'RUN', distanceM: 1000, location: 'Senopati', path: segPath },
    { id: 's3', name: '10k Ride', type: 'RIDE', distanceM: 10_000, location: 'PIK', path: track(8, 1250) },
  ];

  it('matches when the track passes both gates with ~the segment distance between', () => {
    const activityPoints = track(10 / 3, 600); // 2 km straight through the segment
    const matches = matchSegments(segments, { type: 'RUN' }, activityPoints);
    expect(matches).toHaveLength(1);
    expect(matches[0]!.segment.id).toBe('s1');
    // 1 km at 3.33 m/s ≈ 300s from real timestamps.
    expect(matches[0]!.elapsedSec).toBeGreaterThan(280);
    expect(matches[0]!.elapsedSec).toBeLessThan(320);
  });

  it('does not match a track that never reaches the end gate', () => {
    const shortRun = track(10 / 3, 120); // only 400m
    expect(matchSegments(segments, { type: 'RUN' }, shortRun)).toHaveLength(0);
  });

  it('does not match a track far away from the segment', () => {
    const elsewhere = track(10 / 3, 600).map((p) => ({ ...p, lng: p.lng + 0.05 })); // ~5.5km east
    expect(matchSegments(segments, { type: 'RUN' }, elsewhere)).toHaveLength(0);
  });

  it('respects the activity type', () => {
    const activityPoints = track(10 / 3, 600);
    expect(matchSegments(segments, { type: 'RIDE' }, activityPoints)).toHaveLength(0);
  });
});

describe('elevation gain', () => {
  it('accumulates only meaningful climbs', () => {
    const points = track(10 / 3, 300).map((p, i) => ({ ...p, ele: 20 + Math.floor(i / 10) * 2 }));
    const stats = computeActivityStats(points);
    expect(stats.elevationGainM).toBeGreaterThan(8);
  });
  it('is zero without altitude data', () => {
    expect(computeActivityStats(track(10 / 3, 300)).elevationGainM).toBe(0);
  });
});

describe('findGroupedActivities', () => {
  const base = {
    id: 'a1',
    memberId: 'm1',
    type: 'RUN' as const,
    startedAt: '2026-08-30T06:00:00.000Z',
    points: track(10 / 3, 60),
  };
  it('groups same-type nearby activities within the time window', () => {
    const other = { ...base, id: 'a2', memberId: 'm2', startedAt: '2026-08-30T06:20:00.000Z' };
    const far = {
      ...base,
      id: 'a3',
      memberId: 'm3',
      points: base.points.map((p) => ({ ...p, lng: p.lng + 0.05 })),
    };
    const late = { ...base, id: 'a4', memberId: 'm4', startedAt: '2026-08-30T09:00:00.000Z' };
    expect(findGroupedActivities(base, [other, far, late])).toEqual(['a2']);
  });
});

describe('challengeProgressKm', () => {
  const challenge: Challenge = {
    id: 'c1',
    name: '100k',
    description: '',
    type: 'RUN',
    targetKm: 100,
    startsAt: '2026-08-01T00:00:00.000Z',
    endsAt: '2026-08-31T23:59:59.000Z',
  };
  it('sums only matching activities inside the window', () => {
    const progress = challengeProgressKm(challenge, [
      { type: 'RUN', distanceM: 5000, startedAt: '2026-08-10T06:00:00.000Z' },
      { type: 'RUN', distanceM: 7500, startedAt: '2026-08-20T06:00:00.000Z' },
      { type: 'RIDE', distanceM: 20_000, startedAt: '2026-08-15T06:00:00.000Z' }, // wrong type
      { type: 'RUN', distanceM: 9000, startedAt: '2026-09-01T06:00:00.000Z' }, // outside
    ]);
    expect(progress).toBe(12.5);
  });
});

describe('weeklyBuckets & personalRecords', () => {
  it('buckets activities into the requested number of weeks', () => {
    const now = '2026-08-31T12:00:00.000Z';
    const buckets = weeklyBuckets(
      [
        { startedAt: '2026-08-31T06:00:00.000Z', distanceM: 5000, movingSec: 1500 },
        { startedAt: '2026-08-25T06:00:00.000Z', distanceM: 10_000, movingSec: 3000 },
      ],
      now,
      4,
    );
    expect(buckets).toHaveLength(4);
    expect(buckets[3]!.distanceKm).toBe(5);
    expect(buckets[2]!.distanceKm).toBe(10);
    expect(buckets[0]!.activities).toBe(0);
  });

  it('derives PRs from runs', () => {
    const mkActivity = (distanceM: number, movingSec: number): Activity => ({
      id: 'a',
      memberId: 'm',
      type: 'RUN',
      title: '',
      description: '',
      startedAt: '2026-08-01T00:00:00.000Z',
      elapsedSec: movingSec,
      movingSec,
      distanceM,
      avgPaceSecPerKm: Math.round(movingSec / (distanceM / 1000)),
      elevationGainM: 0,
      points: track(distanceM / movingSec, movingSec),
      photos: [],
      visibility: 'EVERYONE',
      gearId: null,
      createdAt: '2026-08-01T00:00:00.000Z',
    });
    const prs = personalRecords([mkActivity(6000, 1800), mkActivity(12_000, 4200)]);
    expect(prs.best5kSec).toBe(1500); // 6k in 1800s → 5k est 1500s
    expect(prs.best10kSec).toBe(3500); // 12k in 4200s → 10k est 3500s
    expect(prs.longestDistanceM).toBe(12_000);
    expect(prs.best1kPaceSec).not.toBeNull();
  });
});
