import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createMockServer } from '../msw/node';

const { server } = createMockServer();
const BASE = 'http://localhost';

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterAll(() => server.close());

async function call(
  method: string,
  path: string,
  opts: { token?: string; body?: unknown } = {},
): Promise<{ status: number; data: any }> {
  const res = await fetch(`${BASE}${path}`, {
    method,
    headers: {
      'content-type': 'application/json',
      ...(opts.token ? { authorization: `Bearer ${opts.token}` } : {}),
    },
    body: opts.body === undefined ? undefined : JSON.stringify(opts.body),
  });
  return { status: res.status, data: await res.json().catch(() => null) };
}

const demo = 'member:mem_demo';

/** Straight-line 5:00/km run of `minutes` minutes. */
function runTrack(minutes: number) {
  const points = [];
  for (let s = 0; s <= minutes * 60; s += 5) {
    points.push({ t: s * 1000, lat: -6.21 + ((10 / 3) * s) / 111_320, lng: 106.82 });
  }
  return points;
}

describe('athlete module (Strava-style)', () => {
  it('serves a seeded feed with kudos and comments', async () => {
    const feed = await call('GET', '/api/athlete/feed?scope=everyone', { token: demo });
    expect(feed.status).toBe(200);
    expect(feed.data.length).toBeGreaterThan(5);
    const withKudos = feed.data.find((a: any) => a.kudosCount > 0);
    expect(withKudos).toBeTruthy();
  });

  it('the following scope is narrower than everyone', async () => {
    const everyone = await call('GET', '/api/athlete/feed?scope=everyone', { token: demo });
    const following = await call('GET', '/api/athlete/feed?scope=following', { token: demo });
    expect(following.data.length).toBeLessThanOrEqual(everyone.data.length);
  });

  let activityId = '';

  it('saves a recorded activity and computes stats server-side', async () => {
    const res = await call('POST', '/api/athlete/activities', {
      token: demo,
      body: {
        type: 'RUN',
        title: 'Test Tempo',
        description: 'integration test',
        startedAt: new Date().toISOString(),
        points: runTrack(30), // ~6 km at 5:00/km
        manualElapsedSec: null,
        gearId: 'gear_shoes1',
        visibility: 'EVERYONE',
      },
    });
    expect(res.status).toBe(201);
    activityId = res.data.id;
    expect(res.data.distanceM).toBeGreaterThan(5800);
    expect(res.data.avgPaceSecPerKm).toBeGreaterThan(290);
    expect(res.data.avgPaceSecPerKm).toBeLessThan(310);
  });

  it('matched segments and shows splits + efforts on the detail view', async () => {
    const res = await call('GET', `/api/athlete/activities/${activityId}`, { token: demo });
    expect(res.status).toBe(200);
    expect(res.data.splits.filter((s: any) => s.full).length).toBeGreaterThanOrEqual(5);
    expect(res.data.efforts.length).toBeGreaterThan(0);
    expect(res.data.gearName).toBe('Asics Novablast 4');
    for (const effort of res.data.efforts) {
      expect(effort.rank).toBeGreaterThan(0);
      expect(effort.totalEfforts).toBeGreaterThanOrEqual(effort.rank);
    }
  });

  it('gear mileage accumulated from the save', async () => {
    const stats = await call('GET', '/api/athlete/stats', { token: demo });
    const shoes = stats.data.gear.find((g: any) => g.id === 'gear_shoes1');
    expect(shoes.distanceM).toBeGreaterThan(5800);
    expect(stats.data.thisWeekKm).toBeGreaterThan(0);
    expect(stats.data.prs.best5kSec).not.toBeNull();
  });

  it('kudos toggles and notifies the owner', async () => {
    const other = 'member:mem_s1';
    const on = await call('POST', `/api/athlete/activities/${activityId}/kudos`, { token: other });
    expect(on.data.kudoed).toBe(true);
    const off = await call('POST', `/api/athlete/activities/${activityId}/kudos`, { token: other });
    expect(off.data.kudoed).toBe(false);

    await call('POST', `/api/athlete/activities/${activityId}/comments`, {
      token: other,
      body: { text: 'Nice one!' },
    });
    const notifications = await call('GET', '/api/me/notifications', { token: demo });
    expect(
      notifications.data.some((n: any) => n.title === 'New comment' || n.title === 'Kudos received'),
    ).toBe(true);
  });

  it('private activities are hidden from others', async () => {
    const saved = await call('POST', '/api/athlete/activities', {
      token: demo,
      body: {
        type: 'RUN',
        title: 'Secret Run',
        description: '',
        startedAt: new Date().toISOString(),
        points: runTrack(10),
        manualElapsedSec: null,
        gearId: null,
        visibility: 'PRIVATE',
      },
    });
    const asOther = await call('GET', `/api/athlete/activities/${saved.data.id}`, {
      token: 'member:mem_s1',
    });
    expect(asOther.status).toBe(403);
    const otherFeed = await call('GET', '/api/athlete/feed?scope=everyone', {
      token: 'member:mem_s1',
    });
    expect(otherFeed.data.some((a: any) => a.id === saved.data.id)).toBe(false);
  });

  it('segment leaderboard ranks best effort per athlete', async () => {
    const segments = await call('GET', '/api/athlete/segments', { token: demo });
    const seg = segments.data.find((s: any) => s.myRank !== null);
    expect(seg).toBeTruthy();
    const detail = await call('GET', `/api/athlete/segments/${seg.segment.id}`, { token: demo });
    const ranks = detail.data.leaderboard.map((r: any) => r.rank);
    expect(ranks).toEqual([...ranks].sort((a: number, b: number) => a - b));
    const ids = detail.data.leaderboard.map((r: any) => r.memberId);
    expect(new Set(ids).size).toBe(ids.length); // one row per athlete
  });

  it('challenges track progress and clubs toggle membership', async () => {
    const challenges = await call('GET', '/api/athlete/challenges', { token: demo });
    const joined = challenges.data.find((c: any) => c.joined);
    expect(joined.progressKm).toBeGreaterThan(0);

    const clubs = await call('GET', '/api/athlete/clubs', { token: demo });
    const target = clubs.data.find((c: any) => !c.joined);
    const toggled = await call('POST', `/api/athlete/clubs/${target.club.id}/toggle`, { token: demo });
    expect(toggled.data.joined).toBe(true);
  });

  it('follow toggles update the social view', async () => {
    const before = await call('GET', '/api/athlete/social', { token: demo });
    const suggestion = before.data.suggestions[0];
    expect(suggestion).toBeTruthy();
    await call('POST', `/api/athlete/follow/${suggestion.memberId}`, { token: demo });
    const after = await call('GET', '/api/athlete/social', { token: demo });
    expect(after.data.following.some((f: any) => f.memberId === suggestion.memberId)).toBe(true);
  });

  it('settings persist and booking reminders respect the toggle', async () => {
    const updated = await call('PUT', '/api/me/settings', {
      token: demo,
      body: { units: 'IMPERIAL', weeklyGoalKm: 30 },
    });
    expect(updated.data.units).toBe('IMPERIAL');
    expect(updated.data.weeklyGoalKm).toBe(30);
    const fetched = await call('GET', '/api/me/settings', { token: demo });
    expect(fetched.data.units).toBe('IMPERIAL');
  });

  it('generates a booking reminder for a class starting within 24h', async () => {
    // Demo member books tomorrow-ish session via admin (guaranteed CONFIRMED).
    const admin = await call('POST', '/api/admin/auth/login', { body: { userId: 'adm_super' } });
    const sessions = await call('GET', '/api/admin/sessions', { token: admin.data.token });
    const soon = sessions.data.find(
      (v: any) =>
        v.session.status === 'PUBLISHED' &&
        new Date(v.session.startsAt).getTime() > Date.now() + 3600_000 &&
        new Date(v.session.startsAt).getTime() < Date.now() + 20 * 3600_000 &&
        v.spotsLeft > 0 &&
        v.myBooking === null,
    );
    expect(soon).toBeTruthy();
    const booked = await call('POST', '/api/admin/bookings', {
      token: admin.data.token,
      body: { memberId: 'mem_demo', sessionId: soon.session.id },
    });
    expect(booked.status).toBe(201);

    const notifications = await call('GET', '/api/me/notifications', { token: demo });
    const reminders = notifications.data.filter((n: any) => n.title === 'Upcoming class');
    expect(reminders.length).toBeGreaterThanOrEqual(1);

    // Idempotent: fetching again must not duplicate the reminder.
    const again = await call('GET', '/api/me/notifications', { token: demo });
    expect(again.data.filter((n: any) => n.title === 'Upcoming class').length).toBe(reminders.length);
  });
});
