import type { UseCaseDeps } from '@hyrox/application';
import { DEFAULT_ATHLETE_SETTINGS, msOf } from '@hyrox/domain';
import type { MockDb } from './db';

/**
 * In-memory implementations of the application-layer ports. Entities are
 * stored by reference, so use cases mutate + save() back into the same arrays.
 */
export function createDeps(db: MockDb): UseCaseDeps {
  const upsert = <T extends { id: string }>(list: T[], item: T): void => {
    const idx = list.findIndex((x) => x.id === item.id);
    if (idx >= 0) list[idx] = item;
    else list.push(item);
  };

  return {
    members: {
      byId: (id) => db.members.find((m) => m.id === id) ?? null,
      byIdentifier: (identifier) => {
        const q = identifier.trim().toLowerCase();
        return (
          db.members.find(
            (m) => m.email.toLowerCase() === q || m.phone.replace(/\s/g, '') === q.replace(/\s/g, ''),
          ) ?? null
        );
      },
      all: () => db.members,
      save: (m) => upsert(db.members, m),
    },
    ledger: {
      byId: (id) => db.ledger.find((e) => e.id === id) ?? null,
      forMember: (memberId) => db.ledger.filter((e) => e.memberId === memberId),
      all: () => db.ledger,
      append: (entry) => db.ledger.push(entry),
      hasReversalOf: (entryId) => db.ledger.some((e) => e.reversesEntryId === entryId),
      lotsFor: (memberId) => db.lots.filter((l) => l.memberId === memberId),
      allLots: () => db.lots,
      addLot: (lot) => db.lots.push(lot),
    },
    payments: {
      byId: (id) => db.payments.find((p) => p.id === id) ?? null,
      forMember: (memberId) => db.payments.filter((p) => p.memberId === memberId),
      all: () => db.payments,
      save: (p) => upsert(db.payments, p),
    },
    packages: {
      byId: (id) => db.packages.find((p) => p.id === id) ?? null,
      all: () => db.packages,
      save: (p) => upsert(db.packages, p),
    },
    vouchers: {
      byId: (id) => db.vouchers.find((v) => v.id === id) ?? null,
      byCode: (code) =>
        db.vouchers.find((v) => v.code.toLowerCase() === code.trim().toLowerCase()) ?? null,
      all: () => db.vouchers,
      save: (v) => upsert(db.vouchers, v),
      redemptionCount: (voucherId) =>
        db.redemptions.filter((r) => r.voucherId === voucherId).length,
      memberRedemptionCount: (voucherId, memberId) =>
        db.redemptions.filter((r) => r.voucherId === voucherId && r.memberId === memberId).length,
      addRedemption: (r) => db.redemptions.push(r),
      redemptions: () => db.redemptions,
    },
    classTypes: {
      byId: (id) => db.classTypes.find((c) => c.id === id) ?? null,
      all: () => db.classTypes,
      save: (c) => upsert(db.classTypes, c),
    },
    sessions: {
      byId: (id) => db.sessions.find((s) => s.id === id) ?? null,
      all: () => db.sessions,
      save: (s) => upsert(db.sessions, s),
    },
    bookings: {
      byId: (id) => db.bookings.find((b) => b.id === id) ?? null,
      forMember: (memberId) => db.bookings.filter((b) => b.memberId === memberId),
      forSession: (sessionId) => db.bookings.filter((b) => b.sessionId === sessionId),
      all: () => db.bookings,
      save: (b) => upsert(db.bookings, b),
    },
    qrTokens: {
      byToken: (token) => db.qrTokens.find((t) => t.token === token) ?? null,
      save: (t) => {
        const idx = db.qrTokens.findIndex((x) => x.token === t.token);
        if (idx >= 0) db.qrTokens[idx] = t;
        else db.qrTokens.push(t);
        // Keep the ephemeral token table small.
        if (db.qrTokens.length > 200) db.qrTokens.splice(0, db.qrTokens.length - 200);
      },
    },
    accessLogs: {
      forMember: (memberId) => db.accessLogs.filter((l) => l.memberId === memberId),
      all: () => db.accessLogs,
      append: (log) => db.accessLogs.push(log),
      lastAllowedAt: (memberId) => {
        const allowed = db.accessLogs
          .filter(
            (l) =>
              l.memberId === memberId && (l.result === 'ALLOWED' || l.result === 'OFFLINE_ALLOWED'),
          )
          .sort((a, b) => msOf(b.createdAt) - msOf(a.createdAt));
        return allowed[0]?.createdAt ?? null;
      },
    },
    notifications: {
      forMember: (memberId) => db.notifications.filter((n) => n.memberId === memberId),
      append: (n) => db.notifications.push(n),
      markAllRead: (memberId, now) => {
        for (const n of db.notifications) {
          if (n.memberId === memberId && n.readAt === null) n.readAt = now;
        }
      },
    },
    campaigns: {
      byId: (id) => db.campaigns.find((c) => c.id === id) ?? null,
      all: () => db.campaigns,
      save: (c) => upsert(db.campaigns, c),
    },
    rules: {
      defaults: () => db.rules,
      saveDefaults: (rules) => {
        db.rules = rules;
      },
      overrideFor: (branchId) =>
        db.branches.find((b) => b.id === branchId)?.rulesOverride ?? null,
    },
    audit: {
      append: (event) => db.audit.push(event),
      all: () => db.audit,
    },
    branches: {
      byId: (id) => db.branches.find((b) => b.id === id) ?? null,
      all: () => db.branches,
      save: (b) => upsert(db.branches, b),
    },
    gates: {
      byId: (id) => db.gates.find((g) => g.id === id) ?? null,
      all: () => db.gates,
      save: (g) => upsert(db.gates, g),
    },
    coaches: {
      byId: (id) => db.coaches.find((c) => c.id === id) ?? null,
      all: () => db.coaches,
      save: (c) => upsert(db.coaches, c),
    },
    adminUsers: {
      byId: (id) => db.adminUsers.find((u) => u.id === id) ?? null,
      all: () => db.adminUsers,
      save: (u) => upsert(db.adminUsers, u),
    },
    athlete: {
      activities: {
        byId: (id) => db.activities.find((a) => a.id === id) ?? null,
        forMember: (memberId) => db.activities.filter((a) => a.memberId === memberId),
        all: () => db.activities,
        save: (a) => upsert(db.activities, a),
      },
      follows: {
        all: () => db.follows,
        isFollowing: (followerId, followeeId) =>
          db.follows.some((f) => f.followerId === followerId && f.followeeId === followeeId),
        toggle: (followerId, followeeId) => {
          const idx = db.follows.findIndex(
            (f) => f.followerId === followerId && f.followeeId === followeeId,
          );
          if (idx >= 0) {
            db.follows.splice(idx, 1);
            return false;
          }
          db.follows.push({ followerId, followeeId });
          return true;
        },
      },
      kudos: {
        forActivity: (activityId) => db.kudos.filter((k) => k.activityId === activityId),
        has: (activityId, memberId) =>
          db.kudos.some((k) => k.activityId === activityId && k.memberId === memberId),
        toggle: (activityId, memberId, now) => {
          const idx = db.kudos.findIndex(
            (k) => k.activityId === activityId && k.memberId === memberId,
          );
          if (idx >= 0) {
            db.kudos.splice(idx, 1);
            return false;
          }
          db.kudos.push({ activityId, memberId, createdAt: now });
          return true;
        },
      },
      comments: {
        forActivity: (activityId) =>
          db.activityComments
            .filter((c) => c.activityId === activityId)
            .sort((a, b) => msOf(a.createdAt) - msOf(b.createdAt)),
        add: (c) => db.activityComments.push(c),
      },
      segments: {
        all: () => db.segments,
        byId: (id) => db.segments.find((s) => s.id === id) ?? null,
      },
      efforts: {
        forSegment: (segmentId) => db.segmentEfforts.filter((e) => e.segmentId === segmentId),
        forMember: (memberId) => db.segmentEfforts.filter((e) => e.memberId === memberId),
        forActivity: (activityId) => db.segmentEfforts.filter((e) => e.activityId === activityId),
        add: (e) => db.segmentEfforts.push(e),
      },
      challenges: {
        all: () => db.challenges,
        byId: (id) => db.challenges.find((c) => c.id === id) ?? null,
        participants: (challengeId) =>
          db.challengeJoins.filter((j) => j.challengeId === challengeId).map((j) => j.memberId),
        isJoined: (challengeId, memberId) =>
          db.challengeJoins.some((j) => j.challengeId === challengeId && j.memberId === memberId),
        join: (challengeId, memberId) => {
          if (!db.challengeJoins.some((j) => j.challengeId === challengeId && j.memberId === memberId))
            db.challengeJoins.push({ challengeId, memberId });
        },
      },
      clubs: {
        all: () => db.clubs,
        byId: (id) => db.clubs.find((c) => c.id === id) ?? null,
        save: (c) => upsert(db.clubs, c),
      },
      gear: {
        byId: (id) => db.gear.find((g) => g.id === id) ?? null,
        forMember: (memberId) => db.gear.filter((g) => g.memberId === memberId),
        save: (g) => upsert(db.gear, g),
      },
      settings: {
        get: (memberId) => db.athleteSettings[memberId] ?? { ...DEFAULT_ATHLETE_SETTINGS },
        save: (memberId, settings) => {
          db.athleteSettings[memberId] = settings;
        },
      },
      remindersSent: {
        has: (bookingId) => db.remindersSent.includes(bookingId),
        add: (bookingId) => db.remindersSent.push(bookingId),
      },
      routes: {
        byId: (id) => db.routes.find((r) => r.id === id) ?? null,
        forMember: (memberId) => db.routes.filter((r) => r.memberId === memberId),
        save: (r) => upsert(db.routes, r),
        remove: (id) => {
          const idx = db.routes.findIndex((r) => r.id === id);
          if (idx >= 0) db.routes.splice(idx, 1);
        },
      },
    },
    workout: {
      exercises: {
        all: () => db.exercises,
        byId: (id) => db.exercises.find((e) => e.id === id) ?? null,
      },
      substitutions: { all: () => db.substitutions },
      workouts: {
        byId: (id) => db.workouts.find((w) => w.id === id) ?? null,
        forMember: (memberId) => db.workouts.filter((w) => w.memberId === memberId),
        save: (w) => upsert(db.workouts, w),
      },
      sessions: {
        byId: (id) => db.workoutSessions.find((s) => s.id === id) ?? null,
        forMember: (memberId) => db.workoutSessions.filter((s) => s.memberId === memberId),
        save: (s) => upsert(db.workoutSessions, s),
      },
    },
    races: {
      events: {
        all: () => db.raceEvents,
        byId: (id) => db.raceEvents.find((e) => e.id === id) ?? null,
      },
      userRaces: {
        byId: (id) => db.userRaces.find((r) => r.id === id) ?? null,
        forMember: (memberId) => db.userRaces.filter((r) => r.memberId === memberId),
        forEvent: (raceEventId) => db.userRaces.filter((r) => r.raceEventId === raceEventId),
        save: (r) => upsert(db.userRaces, r),
      },
    },
    clock: {
      now: () => new Date().toISOString(),
    },
    ids: {
      next: (prefix) => {
        const n = (db.counters[prefix] ?? 1000) + 1;
        db.counters[prefix] = n;
        return `${prefix}_${n}`;
      },
    },
  };
}
