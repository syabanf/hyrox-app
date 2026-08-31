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
const superAdmin = async () =>
  (await call('POST', '/api/admin/auth/login', { body: { userId: 'adm_super' } })).data.token;

describe('HYROX workout generator (phase 3)', () => {
  let workoutId = '';
  let sessionId = '';

  it('serves the exercise library with substitutions', async () => {
    const res = await call('GET', '/api/exercises', { token: demo });
    expect(res.data.exercises.filter((e: any) => e.hyroxStationOrder !== null)).toHaveLength(8);
    expect(res.data.substitutions.length).toBeGreaterThan(4);
  });

  it('generates a full simulation with a substituted excluded station', async () => {
    const res = await call('POST', '/api/workouts/generate', {
      token: demo,
      body: {
        type: 'FULL_SIMULATION',
        division: 'WOMEN_OPEN',
        stationOrders: [],
        excludedExerciseIds: ['ex_sledpush'],
      },
    });
    expect(res.status).toBe(201);
    workoutId = res.data.id;
    expect(res.data.blocks).toHaveLength(16);
    const sub = res.data.blocks.find((b: any) => b.originalExerciseName === 'Sled Push');
    expect(sub.exerciseName).toBe('Plate Push');
    const wallBalls = res.data.blocks.find((b: any) => b.exerciseName === 'Wall Balls');
    expect(wallBalls.reps).toBe(75); // women open
  });

  it('customize: replace a station block with a substitute', async () => {
    const wk = await call('GET', `/api/workouts/${workoutId}`, { token: demo });
    const skiBlock = wk.data.blocks.find((b: any) => b.exerciseName === 'SkiErg');
    const res = await call('POST', `/api/workouts/${workoutId}/replace`, {
      token: demo,
      body: { order: skiBlock.order, exerciseId: 'ex_row' },
    });
    const replaced = res.data.blocks.find((b: any) => b.order === skiBlock.order);
    expect(replaced.exerciseName).toBe('Row');
    expect(replaced.originalExerciseName).toBe('SkiErg');
  });

  it('runs the active session: start → blocks → pause/resume → finish partial', async () => {
    const started = await call('POST', `/api/workouts/${workoutId}/start`, { token: demo });
    expect(started.status).toBe(201);
    sessionId = started.data.session.id;
    expect(started.data.session.status).toBe('STARTED');

    await call('POST', `/api/workout-sessions/${sessionId}/block`, {
      token: demo,
      body: { order: 1, durationSec: 320 },
    });
    const afterTwo = await call('POST', `/api/workout-sessions/${sessionId}/block`, {
      token: demo,
      body: { order: 2, durationSec: 260 },
    });
    expect(afterTwo.data.session.currentBlock).toBe(3);
    expect(afterTwo.data.activeSec).toBe(580);

    const paused = await call('POST', `/api/workout-sessions/${sessionId}/pause`, { token: demo });
    expect(paused.data.session.status).toBe('PAUSED');
    const resumed = await call('POST', `/api/workout-sessions/${sessionId}/resume`, {
      token: demo,
      body: { pausedSec: 45 },
    });
    expect(resumed.data.session.status).toBe('STARTED');
    expect(resumed.data.session.totalPauseSec).toBe(45);

    const finished = await call('POST', `/api/workout-sessions/${sessionId}/finish`, {
      token: demo,
      body: { partial: true },
    });
    expect(finished.data.session.status).toBe('PARTIAL');
    expect(finished.data.completionPct).toBe(13); // 2/16
    expect(finished.data.activityId).toBeTruthy();

    // The effort landed in the training log as a WORKOUT activity.
    const mine = await call('GET', '/api/athlete/activities', { token: demo });
    expect(mine.data.some((a: any) => a.id === finished.data.activityId)).toBe(true);
  });

  it('history lists the seeded completed simulation + the partial one', async () => {
    const res = await call('GET', '/api/workout-sessions', { token: demo });
    expect(res.data.length).toBeGreaterThanOrEqual(2);
    expect(res.data.some((s: any) => s.session.status === 'COMPLETED')).toBe(true);
  });
});

describe('Race ecosystem (phase 4)', () => {
  it('discovers races with region filter and results scope', async () => {
    const upcoming = await call('GET', '/api/races?scope=upcoming', { token: demo });
    expect(upcoming.data.length).toBeGreaterThan(4);
    const asia = await call('GET', '/api/races?region=ASIA', { token: demo });
    expect(asia.data.every((r: any) => r.event.region === 'ASIA')).toBe(true);
    const results = await call('GET', '/api/races?scope=results', { token: demo });
    expect(results.data.every((r: any) => r.event.status === 'COMPLETED')).toBe(true);
  });

  it('my races carry prediction from the seeded full simulation + analysis of the raced one', async () => {
    const res = await call('GET', '/api/me/races', { token: demo });
    const training = res.data.find((r: any) => r.userRace.status === 'TRAINING');
    expect(training.predictionSec).not.toBeNull();
    expect(training.readinessScore).toBeGreaterThan(0);
    expect(training.daysToRace).toBeGreaterThan(0);
    const raced = res.data.find((r: any) => r.userRace.status === 'RACED');
    expect(raced.analysis.achievedGoal).toBe(true); // 93:41 < 95:00 goal
  });

  it('registering twice is rejected; sold-out races refuse registration', async () => {
    const dup = await call('POST', '/api/races/race_jkt/register', {
      token: demo,
      body: { division: 'MEN_OPEN', goalSec: null },
    });
    expect(dup.status).toBe(409);
    const soldOut = await call('POST', '/api/races/race_sgp/register', {
      token: demo,
      body: { division: 'MEN_OPEN', goalSec: null },
    });
    expect(soldOut.status).toBe(422);
  });

  it('entering a result flips status to RACED and yields analysis', async () => {
    await call('POST', '/api/races/race_bkk/register', {
      token: demo,
      body: { division: 'MEN_OPEN', goalSec: 5400 },
    });
    const mine = await call('GET', '/api/me/races', { token: demo });
    const bkk = mine.data.find((r: any) => r.event.id === 'race_bkk');
    const updated = await call('PATCH', `/api/me/races/${bkk.userRace.id}`, {
      token: demo,
      body: { resultSec: 5300 },
    });
    expect(updated.data.status).toBe('RACED');
    const after = await call('GET', '/api/me/races', { token: demo });
    const analysed = after.data.find((r: any) => r.event.id === 'race_bkk');
    expect(analysed.analysis.vsGoalSec).toBe(-100);
  });
});

describe('Train gap features', () => {
  it('activity edit and delete (owner-only, gear mileage restored)', async () => {
    const before = await call('GET', '/api/athlete/stats', { token: demo });
    const shoesBefore = before.data.gear.find((g: any) => g.id === 'gear_shoes1').distanceM;

    const mine = await call('GET', '/api/athlete/activities', { token: demo });
    const target = mine.data.find((a: any) => a.title === 'Lunch Shakeout');

    const edited = await call('PATCH', `/api/athlete/activities/${target.id}`, {
      token: demo,
      body: { title: 'Renamed Run', visibility: 'PRIVATE' },
    });
    expect(edited.data.title).toBe('Renamed Run');

    const forbidden = await call('PATCH', `/api/athlete/activities/${target.id}`, {
      token: 'member:mem_s1',
      body: { title: 'hack' },
    });
    expect(forbidden.status).toBe(403);

    const deleted = await call('DELETE', `/api/athlete/activities/${target.id}`, { token: demo });
    expect(deleted.data.deleted).toBe(true);
    const after = await call('GET', '/api/athlete/stats', { token: demo });
    const shoesAfter = after.data.gear.find((g: any) => g.id === 'gear_shoes1').distanceM;
    expect(shoesAfter).toBeLessThan(shoesBefore);
  });

  it('routes: seeded route exists, save-from-activity works, recorder can fetch it', async () => {
    const routes = await call('GET', '/api/athlete/routes', { token: demo });
    expect(routes.data.some((r: any) => r.name === 'Sudirman Out & Back')).toBe(true);

    const mine = await call('GET', '/api/athlete/activities', { token: demo });
    const src = mine.data.find((a: any) => a.title === 'Tempo Thursday');
    const saved = await call('POST', '/api/athlete/routes', {
      token: demo,
      body: { activityId: src.id, name: 'Tempo Loop' },
    });
    expect(saved.status).toBe(201);
    const full = await call('GET', `/api/athlete/routes/${saved.data.id}`, { token: demo });
    expect(full.data.points.length).toBeGreaterThan(50);
  });

  it('heatmap returns my tracks; grouped activities appear on the detail view', async () => {
    const heat = await call('GET', '/api/athlete/heatmap', { token: demo });
    expect(heat.data.tracks.length).toBeGreaterThan(2);

    const feed = await call('GET', '/api/athlete/feed?scope=everyone', { token: demo });
    const anyDetail = await call('GET', `/api/athlete/activities/${feed.data[0].id}`, {
      token: demo,
    });
    expect(Array.isArray(anyDetail.data.groupedWith)).toBe(true);
  });

  it('segment efforts come from real GPS timestamps now', async () => {
    const segs = await call('GET', '/api/athlete/segments', { token: demo });
    const oneK = segs.data.find((s: any) => s.segment.id === 'seg_1k');
    expect(oneK.effortCount).toBeGreaterThan(3);
    expect(oneK.segment.path.length).toBeGreaterThan(10);
  });
});

describe('Admin deepening', () => {
  it('voucher full edit via PATCH', async () => {
    const token = await superAdmin();
    const res = await call('PATCH', '/api/admin/vouchers/vch_hyrox100', {
      token,
      body: { value: 150_000, perMemberLimit: 3 },
    });
    expect(res.data.voucher.value).toBe(150_000);
    expect(res.data.voucher.perMemberLimit).toBe(3);
  });

  it('gate + user + branch CRUD from the API', async () => {
    const token = await superAdmin();
    const gate = await call('POST', '/api/admin/gates', {
      token,
      body: { name: 'Senopati Gate C', branchId: 'brn_senopati', status: 'ONLINE' },
    });
    expect(gate.status).toBe(201);
    const toggled = await call('PATCH', `/api/admin/gates/${gate.data.id}`, {
      token,
      body: { status: 'OFFLINE' },
    });
    expect(toggled.data.status).toBe('OFFLINE');

    const user = await call('POST', '/api/admin/users', {
      token,
      body: { name: 'New Staff', email: 'staff@hyrox.id', role: 'FRONT_DESK', branchId: 'brn_pik' },
    });
    expect(user.status).toBe(201);

    const branch = await call('POST', '/api/admin/branches', {
      token,
      body: { name: 'BSD', address: 'BSD City', operatingHours: '06:00 – 22:00', timezone: 'Asia/Jakarta', managerName: null },
    });
    expect(branch.status).toBe(201);

    // users.manage is super-admin only.
    const fd = await call('POST', '/api/admin/auth/login', { body: { userId: 'adm_fd' } });
    const denied = await call('POST', '/api/admin/users', {
      token: fd.data.token,
      body: { name: 'X', email: 'x@x.id', role: 'FINANCE', branchId: null },
    });
    expect(denied.status).toBe(403);
  });

  it('custom segment preview counts a criteria-based audience', async () => {
    const token = await superAdmin();
    const all = await call('POST', '/api/admin/segments/preview', {
      token,
      body: { segment: 'ALL_ACTIVE', customFilter: null },
    });
    const custom = await call('POST', '/api/admin/segments/preview', {
      token,
      body: {
        segment: 'CUSTOM',
        customFilter: { branchId: 'brn_senopati', maxBalance: 5, minDaysSinceLastVisit: null, joinedWithinDays: null },
      },
    });
    expect(custom.data.count).toBeGreaterThan(0);
    expect(custom.data.count).toBeLessThan(all.data.count);
    expect(custom.data.sample.length).toBeGreaterThan(0);
  });

  it('classes report aggregates attendance and no-shows', async () => {
    const token = await superAdmin();
    const res = await call('GET', '/api/admin/reports/classes', { token });
    expect(res.data.perType.length).toBe(8);
    const withData = res.data.perType.find((t: any) => t.booked > 0);
    expect(withData.attendanceRate).toBeGreaterThan(0);
    expect(Array.isArray(res.data.recentNoShows)).toBe(true);
  });

  it('offline CONFLICT can be approved (deduction applied) and is audited', async () => {
    const token = await superAdmin();
    const logs = await call('GET', '/api/admin/access-logs?result=CONFLICT', { token });
    const conflict = logs.data[0];
    expect(conflict).toBeTruthy();
    const resolved = await call('POST', `/api/admin/access-logs/${conflict.log.id}/resolve`, {
      token,
      body: { action: 'APPROVE', reason: 'Verified against camera footage' },
    });
    expect(resolved.data.log.result).toBe('SYNCED');
    expect(resolved.data.log.creditDelta).toBe(-1);
    const audit = await call('GET', '/api/admin/audit', { token });
    expect(audit.data.some((a: any) => a.action === 'OFFLINE_APPROVE')).toBe(true);
  });
});

describe('Member kecil-kecil', () => {
  it('avatar uploads via profile PATCH and shows in the feed', async () => {
    const avatar = `data:image/png;base64,${'A'.repeat(200)}`;
    const res = await call('PATCH', '/api/me', { token: demo, body: { avatarUrl: avatar } });
    expect(res.data.avatarUrl).toBe(avatar);
    const feed = await call('GET', '/api/athlete/feed?scope=everyone', { token: demo });
    const mineCard = feed.data.find((a: any) => a.memberId === 'mem_demo');
    expect(mineCard.memberAvatarUrl).toBe(avatar);
  });

  it('manual waitlist confirmation: offer → member confirms the spot', async () => {
    const token = await superAdmin();
    // Turn off auto-promotion.
    await call('PUT', '/api/admin/rules', { token, body: { waitlistAutoPromote: false } });

    const sessions = await call('GET', '/api/admin/sessions', { token });
    const full = sessions.data.find((v: any) => v.session.status === 'FULL');
    const detail = await call('GET', `/api/admin/sessions/${full.session.id}`, { token });
    const confirmed = detail.data.roster.find((r: any) => r.booking.status === 'CONFIRMED');
    const firstWl = detail.data.roster
      .filter((r: any) => r.booking.status === 'WAITLIST')
      .sort((a: any, b: any) => a.booking.waitlistPosition - b.booking.waitlistPosition)[0];

    const cancel = await call('POST', `/api/bookings/${confirmed.booking.id}/cancel`, {
      token,
      body: {},
    });
    expect(cancel.data.promotedMemberName).toBeNull(); // offered, not auto-promoted

    const after = await call('GET', `/api/admin/sessions/${full.session.id}`, { token });
    const offered = after.data.roster.find((r: any) => r.booking.id === firstWl.booking.id);
    expect(offered.booking.status).toBe('WAITLIST');
    expect(offered.booking.promotionOfferedAt).not.toBeNull();

    const confirm = await call('POST', `/api/bookings/${firstWl.booking.id}/confirm-spot`, {
      token: `member:${firstWl.booking.memberId}`,
      body: {},
    });
    expect(confirm.status).toBe(200);
    expect(confirm.data.status).toBe('CONFIRMED');

    await call('PUT', '/api/admin/rules', { token, body: { waitlistAutoPromote: true } });
  });

  it('language preference persists in settings (i18n)', async () => {
    const res = await call('PUT', '/api/me/settings', { token: demo, body: { language: 'ID' } });
    expect(res.data.language).toBe('ID');
  });
});
