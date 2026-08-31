import { faker } from '@faker-js/faker';
import type {
  AccessLog,
  Activity,
  ActivityType,
  Booking,
  ClassSession,
  CreditLedgerEntry,
  Exercise,
  GeneratedWorkout,
  Member,
  Payment,
  RaceEvent,
  TopUpLot,
  TrackPoint,
} from '@hyrox/domain';
import { computeActivityStats, generateWorkout, matchSegments } from '@hyrox/domain';
import type { MockDb } from './db';
import { createEmptyDb } from './db';

/**
 * Deterministic demo dataset (faker seed 42), anchored to boot time so the
 * schedule always spans "last week … next week" relative to today.
 */
export function createSeedDb(nowIso: string): MockDb {
  faker.seed(42);
  const db = createEmptyDb(nowIso);
  const now = new Date(nowIso);

  const at = (dayOffset: number, hour: number, minute = 0): string => {
    const d = new Date(now);
    d.setDate(d.getDate() + dayOffset);
    d.setHours(hour, minute, 0, 0);
    return d.toISOString();
  };
  const daysAgo = (days: number, hour = 10): string => at(-days, hour);
  const addDays = (iso: string, days: number): string => {
    const d = new Date(iso);
    d.setDate(d.getDate() + days);
    return d.toISOString();
  };

  // ── Branches, gates, coaches ─────────────────────────────────────────────
  db.branches = [
    {
      id: 'brn_senopati',
      organizationId: db.organization.id,
      name: 'Senopati',
      address: 'Jl. Senopati No. 88, Jakarta Selatan',
      timezone: 'Asia/Jakarta',
      operatingHours: '06:00 – 22:00',
      status: 'ACTIVE',
      managerName: 'Bima Prasetyo',
      rulesOverride: null,
    },
    {
      id: 'brn_pik',
      organizationId: db.organization.id,
      name: 'PIK',
      address: 'Pantai Indah Kapuk Blvd. 12, Jakarta Utara',
      timezone: 'Asia/Jakarta',
      operatingHours: '06:00 – 21:00',
      status: 'ACTIVE',
      managerName: 'Sari Wulandari',
      rulesOverride: { qrTtlSeconds: 60 },
    },
  ];
  db.gates = [
    { id: 'gat_sen_a', branchId: 'brn_senopati', name: 'Senopati Gate A', status: 'ONLINE' },
    { id: 'gat_sen_b', branchId: 'brn_senopati', name: 'Senopati Gate B', status: 'ONLINE' },
    { id: 'gat_pik_a', branchId: 'brn_pik', name: 'PIK Gate A', status: 'ONLINE' },
    { id: 'gat_pik_b', branchId: 'brn_pik', name: 'PIK Gate B', status: 'OFFLINE' },
  ];
  const coachNames = [
    ['Kevin Hartono', 'Engine & simulation specialist'],
    ['Maya Kusuma', 'Strength and conditioning'],
    ['Rizky Ramadhan', 'HYROX race coach'],
    ['Tara Widjaja', 'Mobility and recovery'],
    ['Dimas Nugroho', 'Functional fitness'],
    ['Livia Chandra', 'Endurance programming'],
  ] as const;
  db.coaches = coachNames.map(([name, specialization], i) => ({
    id: `coa_${i + 1}`,
    name,
    bio: faker.lorem.sentence(),
    specialization,
    branchId: i % 2 === 0 ? 'brn_senopati' : 'brn_pik',
    status: 'ACTIVE',
  }));

  // ── Class types ──────────────────────────────────────────────────────────
  const typeDefs = [
    ['cls_fund', 'HYROX Fundamentals', 60, 1, 16],
    ['cls_sim', 'HYROX Full Simulation', 90, 2, 12],
    ['cls_str', 'HYROX Strength', 60, 1, 14],
    ['cls_eng', 'HYROX Engine', 60, 1, 16],
    ['cls_open', 'Open Gym', 120, 1, 20],
    ['cls_mob', 'Mobility & Recovery', 45, 1, 12],
    ['cls_wod', 'Team WOD', 60, 1, 16],
    ['cls_test', 'Fitness Test', 75, 1, 10],
  ] as const;
  db.classTypes = typeDefs.map(([id, name, dur, cost, cap]) => ({
    id,
    name,
    description: faker.lorem.sentences(2),
    defaultDurationMin: dur,
    defaultCreditCost: cost,
    defaultCapacity: cap,
    active: true,
  }));

  // ── Sessions: −7d … +7d, both branches ───────────────────────────────────
  const slotHours = [6, 7, 12, 17, 18, 19];
  let sessionIdx = 0;
  for (let day = -7; day <= 7; day++) {
    for (const branch of db.branches) {
      for (const hour of slotHours) {
        const type = faker.helpers.arrayElement(db.classTypes);
        const startsAt = at(day, hour);
        const isPast = new Date(startsAt).getTime() < now.getTime();
        const session: ClassSession = {
          id: `ses_${++sessionIdx}`,
          classTypeId: type.id,
          branchId: branch.id,
          coachId: faker.helpers.arrayElement(
            db.coaches.filter((c) => c.branchId === branch.id),
          ).id,
          startsAt,
          endsAt: at(day, hour, type.defaultDurationMin),
          capacity: type.defaultCapacity,
          creditCost: type.defaultCreditCost,
          bookingOpensAt: addDays(startsAt, -db.rules.bookingOpensDaysBefore),
          bookingClosesAt: startsAt,
          status: isPast ? 'COMPLETED' : 'PUBLISHED',
          area: hour === 12 ? 'Studio B' : 'Main Floor',
        };
        db.sessions.push(session);
      }
    }
  }
  // One cancelled past session for realism.
  const cancelled = db.sessions.find((s) => s.branchId === 'brn_pik' && s.status === 'COMPLETED');
  if (cancelled) cancelled.status = 'CANCELLED';

  // ── Packages & vouchers ──────────────────────────────────────────────────
  db.packages = [
    {
      id: 'pkg_5',
      name: 'Starter 5',
      credits: 5,
      priceIdr: 800_000,
      validityDays: 30,
      branchId: null,
      purchaseLimitPerMember: null,
      applicableClassTypeIds: null,
      status: 'ACTIVE',
      createdAt: daysAgo(120),
    },
    {
      id: 'pkg_10',
      name: '10 Visit Pack',
      credits: 10,
      priceIdr: 1_500_000,
      validityDays: 60,
      branchId: null,
      purchaseLimitPerMember: null,
      applicableClassTypeIds: null,
      status: 'ACTIVE',
      createdAt: daysAgo(120),
    },
    {
      id: 'pkg_20',
      name: '20 Visit Pack',
      credits: 20,
      priceIdr: 2_800_000,
      validityDays: 90,
      branchId: null,
      purchaseLimitPerMember: null,
      applicableClassTypeIds: null,
      status: 'ACTIVE',
      createdAt: daysAgo(120),
    },
    {
      id: 'pkg_50',
      name: 'Athlete 50',
      credits: 50,
      priceIdr: 6_000_000,
      validityDays: 180,
      branchId: null,
      purchaseLimitPerMember: 1,
      applicableClassTypeIds: null,
      status: 'ACTIVE',
      createdAt: daysAgo(90),
    },
    {
      id: 'pkg_gym',
      name: 'Open Gym Focus',
      credits: 8,
      priceIdr: 900_000,
      validityDays: 45,
      branchId: null,
      purchaseLimitPerMember: null,
      applicableClassTypeIds: ['cls_open', 'cls_fund'],
      status: 'ACTIVE',
      createdAt: daysAgo(60),
    },
  ];
  db.vouchers = [
    {
      id: 'vch_welcome',
      code: 'WELCOME10',
      type: 'PERCENT',
      value: 10,
      startsAt: daysAgo(60),
      endsAt: addDays(nowIso, 60),
      usageLimit: null,
      perMemberLimit: 1,
      eligibleSegment: 'NEW_MEMBERS',
      applicablePackageIds: null,
      status: 'ACTIVE',
      createdAt: daysAgo(60),
    },
    {
      id: 'vch_hyrox100',
      code: 'HYROX100',
      type: 'FIXED_IDR',
      value: 100_000,
      startsAt: daysAgo(30),
      endsAt: addDays(nowIso, 30),
      usageLimit: 100,
      perMemberLimit: 2,
      eligibleSegment: 'ALL',
      applicablePackageIds: ['pkg_10', 'pkg_20'],
      status: 'ACTIVE',
      createdAt: daysAgo(30),
    },
    {
      id: 'vch_early',
      code: 'EARLYBIRD',
      type: 'PERCENT',
      value: 15,
      startsAt: addDays(nowIso, 14),
      endsAt: addDays(nowIso, 44),
      usageLimit: 50,
      perMemberLimit: 1,
      eligibleSegment: 'ALL',
      applicablePackageIds: null,
      status: 'SCHEDULED',
      createdAt: daysAgo(5),
    },
    {
      id: 'vch_legacy',
      code: 'LEGACY50',
      type: 'FIXED_IDR',
      value: 50_000,
      startsAt: daysAgo(90),
      endsAt: daysAgo(30),
      usageLimit: 100,
      perMemberLimit: 1,
      eligibleSegment: 'ALL',
      applicablePackageIds: null,
      status: 'EXPIRED',
      createdAt: daysAgo(90),
    },
    {
      id: 'vch_buddy',
      code: 'BUDDYPASS',
      type: 'FIXED_IDR',
      value: 150_000,
      startsAt: daysAgo(3),
      endsAt: addDays(nowIso, 21),
      usageLimit: 60,
      perMemberLimit: 1,
      eligibleSegment: 'ALL',
      applicablePackageIds: null,
      status: 'ACTIVE',
      createdAt: daysAgo(3),
    },
    {
      id: 'vch_paused',
      code: 'FLASHSALE',
      type: 'PERCENT',
      value: 20,
      startsAt: daysAgo(10),
      endsAt: addDays(nowIso, 10),
      usageLimit: 30,
      perMemberLimit: 1,
      eligibleSegment: 'ALL',
      applicablePackageIds: null,
      status: 'DISABLED',
      createdAt: daysAgo(10),
    },
  ];

  // ── Admin users (one per role) ───────────────────────────────────────────
  db.adminUsers = [
    { id: 'adm_super', name: 'Alya Santoso', email: 'alya@hyrox.id', role: 'SUPER_ADMIN', branchId: null },
    { id: 'adm_hq', name: 'Raka Wibowo', email: 'raka@hyrox.id', role: 'HQ_ADMIN', branchId: null },
    { id: 'adm_bm', name: 'Bima Prasetyo', email: 'bima@hyrox.id', role: 'BRANCH_MANAGER', branchId: 'brn_senopati' },
    { id: 'adm_fd', name: 'Nadia Putri', email: 'nadia@hyrox.id', role: 'FRONT_DESK', branchId: 'brn_senopati' },
    { id: 'adm_coach', name: 'Kevin Hartono', email: 'kevin@hyrox.id', role: 'COACH', branchId: 'brn_senopati' },
    { id: 'adm_fin', name: 'Sinta Halim', email: 'sinta@hyrox.id', role: 'FINANCE', branchId: null },
  ];

  // ── Members ──────────────────────────────────────────────────────────────
  const mkMember = (
    id: string,
    fullName: string,
    email: string,
    phone: string,
    status: Member['status'],
    createdDaysAgo: number,
  ): Member => ({
    id,
    fullName,
    email,
    phone,
    dateOfBirth: faker.date
      .birthdate({ min: 20, max: 45, mode: 'age', refDate: now })
      .toISOString(),
    gender: faker.helpers.arrayElement(['MALE', 'FEMALE'] as const),
    emergencyContact: {
      name: faker.person.fullName(),
      phone: `+62812${faker.string.numeric(7)}`,
      relation: faker.helpers.arrayElement(['Spouse', 'Parent', 'Sibling', 'Friend']),
    },
    preferredBranchId: faker.helpers.arrayElement(['brn_senopati', 'brn_pik']),
    avatarUrl: null,
    status,
    waiverVersion: 'v1.0',
    waiverAcceptedAt: daysAgo(createdDaysAgo),
    notes: null,
    createdAt: daysAgo(createdDaysAgo),
    updatedAt: daysAgo(Math.max(0, createdDaysAgo - 1)),
  });

  const demo = mkMember(
    'mem_demo',
    'Fahmi Syaban',
    'demo@hyrox.id',
    '+628123456789',
    'ACTIVE',
    70,
  );
  demo.preferredBranchId = 'brn_senopati';
  db.members.push(demo);

  const statuses: Member['status'][] = [
    ...Array<Member['status']>(25).fill('ACTIVE'),
    'SUSPENDED',
    'SUSPENDED',
    'INACTIVE',
    'ARCHIVED',
  ];
  statuses.forEach((status, i) => {
    const name = faker.person.fullName();
    db.members.push(
      mkMember(
        `mem_s${i + 1}`,
        name,
        faker.internet.email({ firstName: name.split(' ')[0], provider: 'example.com' }).toLowerCase(),
        `+62813${faker.string.numeric(7)}`,
        status,
        faker.number.int({ min: 5, max: 180 }),
      ),
    );
  });

  // ── Financial + activity history ─────────────────────────────────────────
  let payN = 0;
  let ledN = 0;
  let lotN = 0;
  let bokN = 0;
  let accN = 0;

  const addPayment = (
    memberId: string,
    pkgId: string,
    status: Payment['status'],
    createdDaysAgo: number,
    voucherCode: string | null = null,
    discountIdr = 0,
  ): Payment => {
    const pkg = db.packages.find((p) => p.id === pkgId)!;
    const payment: Payment = {
      id: `pay_${++payN}`,
      memberId,
      packageId: pkgId,
      credits: pkg.credits,
      amountIdr: pkg.priceIdr,
      discountIdr,
      totalIdr: pkg.priceIdr - discountIdr,
      voucherCode,
      channel: faker.helpers.arrayElement(['QRIS', 'EWALLET', 'VIRTUAL_ACCOUNT', 'CARD'] as const),
      status,
      createdAt: daysAgo(createdDaysAgo),
      paidAt: status === 'PAID' || status === 'REFUNDED' ? daysAgo(createdDaysAgo, 11) : null,
      refundedAt: status === 'REFUNDED' ? daysAgo(Math.max(0, createdDaysAgo - 2)) : null,
    };
    db.payments.push(payment);
    return payment;
  };

  const addTopUpEntry = (payment: Payment, expiresInDaysFromNow: number): CreditLedgerEntry => {
    const pkg = db.packages.find((p) => p.id === payment.packageId)!;
    const entry: CreditLedgerEntry = {
      id: `led_${++ledN}`,
      memberId: payment.memberId,
      type: 'TOP_UP',
      amount: payment.credits,
      description: `Top up — ${pkg.name}`,
      sourceType: 'PAYMENT',
      sourceId: payment.id,
      reversesEntryId: null,
      actorId: null,
      reason: null,
      createdAt: payment.paidAt ?? payment.createdAt,
    };
    db.ledger.push(entry);
    const lot: TopUpLot = {
      id: `lot_${++lotN}`,
      memberId: payment.memberId,
      ledgerEntryId: entry.id,
      packageId: payment.packageId,
      credits: payment.credits,
      expiresAt: addDays(nowIso, expiresInDaysFromNow),
      createdAt: entry.createdAt,
    };
    db.lots.push(lot);
    return entry;
  };

  const addEntry = (partial: Partial<CreditLedgerEntry> & Pick<CreditLedgerEntry, 'memberId' | 'type' | 'amount' | 'description'>): CreditLedgerEntry => {
    const entry: CreditLedgerEntry = {
      id: `led_${++ledN}`,
      sourceType: null,
      sourceId: null,
      reversesEntryId: null,
      actorId: null,
      reason: null,
      createdAt: daysAgo(faker.number.int({ min: 1, max: 20 })),
      ...partial,
    };
    db.ledger.push(entry);
    return entry;
  };

  const addAccess = (
    memberId: string,
    gateId: string,
    result: AccessLog['result'],
    createdAt: string,
    creditDelta: number,
    reasonCode: AccessLog['reasonCode'] = null,
    mode: AccessLog['mode'] = 'ONLINE',
    bookingId: string | null = null,
  ): AccessLog => {
    const gate = db.gates.find((g) => g.id === gateId)!;
    const log: AccessLog = {
      id: `acc_${++accN}`,
      memberId,
      gateId,
      branchId: gate.branchId,
      result,
      reasonCode,
      creditDelta,
      mode,
      bookingId,
      createdAt,
    };
    db.accessLogs.push(log);
    if (creditDelta !== 0) {
      addEntry({
        memberId,
        type: 'VISIT_DEDUCTION',
        amount: creditDelta,
        description: `Studio entry — ${gate.name}`,
        sourceType: 'ACCESS',
        sourceId: log.id,
        createdAt,
      });
    }
    return log;
  };

  const addBooking = (
    memberId: string,
    session: ClassSession,
    status: Booking['status'],
    waitlistPosition: number | null = null,
  ): Booking => {
    const booking: Booking = {
      id: `bok_${++bokN}`,
      memberId,
      sessionId: session.id,
      status,
      waitlistPosition,
      source: 'MEMBER',
      createdAt: addDays(session.startsAt, -2),
      updatedAt: addDays(session.startsAt, -2),
      cancelledAt: status === 'CANCELLED' ? addDays(session.startsAt, -1) : null,
      checkedInAt: status === 'CHECKED_IN' || status === 'COMPLETED' ? session.startsAt : null,
      promotionOfferedAt: null,
    };
    db.bookings.push(booking);
    return booking;
  };

  const pastSessions = db.sessions.filter((s) => s.status === 'COMPLETED');
  const futureSessions = db.sessions
    .filter((s) => s.status === 'PUBLISHED')
    .sort((a, b) => new Date(a.startsAt).getTime() - new Date(b.startsAt).getTime());

  // Demo member: rich, coherent history.
  const demoPay1 = addPayment('mem_demo', 'pkg_10', 'PAID', 35);
  addTopUpEntry(demoPay1, 25);
  const demoPay2 = addPayment('mem_demo', 'pkg_5', 'PAID', 57);
  addTopUpEntry(demoPay2, 3); // expiring soon → drives the expiry warning UI
  addEntry({
    memberId: 'mem_demo',
    type: 'PROMO',
    amount: 2,
    description: 'Opening week promo',
    sourceType: 'SYSTEM',
    createdAt: daysAgo(20),
  });
  const badAdj = addEntry({
    memberId: 'mem_demo',
    type: 'ADJUSTMENT',
    amount: -2,
    description: 'Manual adjustment (-2)',
    sourceType: 'ADMIN',
    actorId: 'adm_fd',
    reason: 'Front desk keying error',
    createdAt: daysAgo(10),
  });
  addEntry({
    memberId: 'mem_demo',
    type: 'REVERSAL',
    amount: 2,
    description: `Reversal of ADJUSTMENT (${badAdj.id})`,
    sourceType: 'ADMIN',
    reversesEntryId: badAdj.id,
    actorId: 'adm_super',
    reason: 'Correcting keying error',
    createdAt: daysAgo(9),
  });
  db.audit.push({
    id: 'aud_seed_1',
    entityType: 'LEDGER',
    entityId: badAdj.id,
    action: 'REVERSAL',
    previousValue: '-2',
    newValue: '2',
    actorId: 'adm_super',
    actorName: 'Alya Santoso',
    reason: 'Correcting keying error',
    createdAt: daysAgo(9),
  });

  // Demo visits: 5 past check-ins (with matching bookings on past sessions) + 1 denied + 1 offline.
  const demoPast = pastSessions.filter((s) => s.branchId === 'brn_senopati').slice(0, 6);
  demoPast.slice(0, 5).forEach((s, i) => {
    const b = addBooking('mem_demo', s, 'COMPLETED');
    addAccess('mem_demo', 'gat_sen_a', 'ALLOWED', s.startsAt, -s.creditCost, null, 'ONLINE', b.id);
    if (i === 2) {
      // Double-scan attempt 30 minutes after entry that day.
      const later = new Date(new Date(s.startsAt).getTime() + 30 * 60_000).toISOString();
      addAccess('mem_demo', 'gat_sen_a', 'DENIED', later, 0, 'ANTI_PASSBACK');
    }
  });
  const offlineSession = demoPast[5];
  if (offlineSession) {
    const b = addBooking('mem_demo', offlineSession, 'COMPLETED');
    addAccess(
      'mem_demo',
      'gat_sen_b',
      'SYNCED',
      offlineSession.startsAt,
      -offlineSession.creditCost,
      null,
      'OFFLINE',
      b.id,
    );
  }
  const demoNoShow = pastSessions.filter((s) => s.branchId === 'brn_senopati')[7];
  if (demoNoShow) addBooking('mem_demo', demoNoShow, 'NO_SHOW');

  // Demo upcoming: a confirmed booking on the next evening session.
  const demoUpcoming = futureSessions.find(
    (s) => s.branchId === 'brn_senopati' && new Date(s.startsAt).getHours() >= 17,
  );
  if (demoUpcoming) addBooking('mem_demo', demoUpcoming, 'CONFIRMED');

  // A FULL session in ~2 days with a waitlist (demo is #2).
  const fullSession = futureSessions.find(
    (s) => s.branchId === 'brn_senopati' && s.id !== demoUpcoming?.id && new Date(s.startsAt).getTime() > now.getTime() + 36 * 3600_000,
  );
  if (fullSession) {
    fullSession.capacity = 4;
    fullSession.status = 'FULL';
    ['mem_s1', 'mem_s2', 'mem_s3', 'mem_s4'].forEach((m) => addBooking(m, fullSession, 'CONFIRMED'));
    addBooking('mem_s5', fullSession, 'WAITLIST', 1);
    addBooking('mem_demo', fullSession, 'WAITLIST', 2);
  }

  // Other members: payments, visits, upcoming bookings.
  const activeSeeded = db.members.filter((m) => m.id !== 'mem_demo' && m.status === 'ACTIVE');
  activeSeeded.forEach((member, i) => {
    const pkg = faker.helpers.arrayElement(['pkg_5', 'pkg_10', 'pkg_20'] as const);
    const paidAgo = faker.number.int({ min: 5, max: 50 });
    const pay = addPayment(member.id, pkg, 'PAID', paidAgo);
    addTopUpEntry(pay, faker.number.int({ min: 10, max: 80 }));

    const visits = faker.number.int({ min: 0, max: 4 });
    const memberPast = faker.helpers.arrayElements(pastSessions, visits);
    for (const s of memberPast) {
      const b = addBooking(member.id, s, 'COMPLETED');
      const gate = s.branchId === 'brn_senopati' ? 'gat_sen_a' : 'gat_pik_a';
      addAccess(member.id, gate, 'ALLOWED', s.startsAt, -s.creditCost, null, 'ONLINE', b.id);
    }
    const upcoming = futureSessions[i + 3];
    if (i % 5 === 0 && upcoming && upcoming.id !== fullSession?.id) {
      addBooking(member.id, upcoming, 'CONFIRMED');
    }
    if (i === 3) addPayment(member.id, 'pkg_10', 'PENDING', 1);
    if (i === 4) addPayment(member.id, 'pkg_5', 'FAILED', 3);
    if (i === 6) addPayment(member.id, 'pkg_20', 'EXPIRED', 6);
  });

  // One refunded payment with its reversal pair.
  const refundMember = activeSeeded[7];
  if (refundMember) {
    const pay = addPayment(refundMember.id, 'pkg_5', 'REFUNDED', 12);
    const top = addTopUpEntry(pay, 20);
    addEntry({
      memberId: refundMember.id,
      type: 'REVERSAL',
      amount: -top.amount,
      description: `Reversal of TOP_UP (${top.id})`,
      sourceType: 'ADMIN',
      reversesEntryId: top.id,
      actorId: 'adm_fin',
      reason: 'Payment refunded',
      createdAt: daysAgo(10),
    });
  }

  // A stale OFFLINE CONFLICT row for the sync monitor.
  const conflictMember = activeSeeded[9];
  if (conflictMember) {
    addAccess(conflictMember.id, 'gat_pik_b', 'CONFLICT', daysAgo(2, 18), 0, null, 'OFFLINE');
  }

  // A BONUS entry for someone (all 8 ledger types now appear in the dataset).
  const bonusMember = activeSeeded[2];
  if (bonusMember) {
    addEntry({
      memberId: bonusMember.id,
      type: 'BONUS',
      amount: 1,
      description: 'Referral bonus',
      sourceType: 'ADMIN',
      actorId: 'adm_hq',
      createdAt: daysAgo(8),
    });
  }

  // An expired-lot member: EXPIRATION entry already recorded.
  const expiredMember = activeSeeded[5];
  if (expiredMember) {
    const pay = addPayment(expiredMember.id, 'pkg_5', 'PAID', 70);
    const top = addTopUpEntry(pay, -10); // expired 10 days ago
    const lot = db.lots.find((l) => l.ledgerEntryId === top.id)!;
    addEntry({
      memberId: expiredMember.id,
      type: 'EXPIRATION',
      amount: -3,
      description: `Credits expired (lot ${lot.id})`,
      sourceType: 'SYSTEM',
      sourceId: lot.id,
      createdAt: daysAgo(10),
    });
    addEntry({
      memberId: expiredMember.id,
      type: 'VISIT_DEDUCTION',
      amount: -2,
      description: 'Studio entry — PIK Gate A',
      sourceType: 'ACCESS',
      createdAt: daysAgo(30),
    });
  }

  // ── Notifications & campaigns ────────────────────────────────────────────
  db.notifications.push(
    {
      id: 'ntf_seed_1',
      memberId: 'mem_demo',
      type: 'BOOKING_REMINDER',
      title: 'Class tomorrow',
      body: demoUpcoming
        ? 'Reminder: your booked class starts tomorrow. Bring your QR!'
        : 'Reminder: you have an upcoming class.',
      createdAt: daysAgo(0, 8),
      readAt: null,
    },
    {
      id: 'ntf_seed_2',
      memberId: 'mem_demo',
      type: 'CREDIT_EXPIRY',
      title: 'Credits expiring soon',
      body: 'Some of your credits expire within 3 days. Use them or lose them!',
      createdAt: daysAgo(1, 9),
      readAt: null,
    },
    {
      id: 'ntf_seed_3',
      memberId: 'mem_demo',
      type: 'ANNOUNCEMENT',
      title: 'New Full Simulation slots',
      body: 'Saturday race simulations are now open for booking.',
      createdAt: daysAgo(3, 12),
      readAt: daysAgo(2, 12),
    },
  );
  db.campaigns.push(
    {
      id: 'cmp_1',
      name: 'Race season is here',
      segment: 'ALL_ACTIVE',
      customFilter: null,
      message: 'Book your HYROX Full Simulation this weekend — limited slots!',
      deepLink: '/classes',
      imageUrl: null,
      scheduledAt: null,
      status: 'SENT',
      sentCount: 26,
      createdAt: daysAgo(6),
    },
    {
      id: 'cmp_2',
      name: 'Win-back: quiet members',
      segment: 'NO_VISIT_14D',
      customFilter: null,
      message: 'We miss you! Your credits are waiting.',
      deepLink: '/wallet',
      imageUrl: null,
      scheduledAt: addDays(nowIso, 2),
      status: 'DRAFT',
      sentCount: null,
      createdAt: daysAgo(1),
    },
    {
      id: 'cmp_3',
      name: 'New: Mobility & Recovery hour',
      segment: 'ALL_ACTIVE',
      customFilter: null,
      message:
        'Every Wednesday 12:00 at Senopati — undo your sled-push sins. First session is on us.',
      deepLink: '/classes',
      imageUrl: null,
      scheduledAt: null,
      status: 'SENT',
      sentCount: 26,
      createdAt: daysAgo(3),
    },
    {
      id: 'cmp_4',
      name: 'Holiday opening hours',
      segment: 'ALL_ACTIVE',
      customFilter: null,
      message: 'Both branches open 08:00–18:00 on the public holiday next Monday.',
      deepLink: null,
      imageUrl: null,
      scheduledAt: null,
      status: 'SENT',
      sentCount: 26,
      createdAt: daysAgo(1, 9),
    },
    {
      id: 'cmp_5',
      name: 'Coach Livia joins Senopati',
      segment: 'ALL_ACTIVE',
      customFilter: null,
      message: 'Endurance specialist Livia Chandra now coaches Tuesday Engine sessions.',
      deepLink: '/classes',
      imageUrl: null,
      scheduledAt: null,
      status: 'SENT',
      sentCount: 26,
      createdAt: daysAgo(9),
    },
    // Photo announcements — these are the ones the Home card leads with.
    {
      id: 'cmp_6',
      name: 'New assault bikes have landed',
      segment: 'ALL_ACTIVE',
      customFilter: null,
      message: 'Twelve brand-new bikes on the Senopati floor — come break them in this week.',
      deepLink: '/classes',
      imageUrl: '/img/ann-equipment.jpg',
      scheduledAt: null,
      status: 'SENT',
      sentCount: 26,
      createdAt: daysAgo(0, 7),
    },
    {
      id: 'cmp_7',
      name: 'Sunday community run, 5K + coffee',
      segment: 'ALL_ACTIVE',
      customFilter: null,
      message: 'Easy pace from the PIK studio, 06:30. Log it in Train and earn challenge kilometres.',
      deepLink: '/train',
      imageUrl: '/img/ann-community.jpg',
      scheduledAt: null,
      status: 'SENT',
      sentCount: 26,
      createdAt: daysAgo(1, 15),
    },
    {
      id: 'cmp_8',
      name: 'Recovery corner now open',
      segment: 'ALL_ACTIVE',
      customFilter: null,
      message: 'Foam rollers, massage guns, and a stretch zone next to the turf — free for members.',
      deepLink: null,
      imageUrl: '/img/ann-recovery.jpg',
      scheduledAt: null,
      status: 'SENT',
      sentCount: 26,
      createdAt: daysAgo(2, 11),
    },
  );

  db.audit.push({
    id: 'aud_seed_2',
    entityType: 'BUSINESS_RULES',
    entityId: 'defaults',
    action: 'UPDATE',
    previousValue: null,
    newValue: JSON.stringify(db.rules),
    actorId: 'adm_super',
    actorName: 'Alya Santoso',
    reason: 'Initial configuration',
    createdAt: daysAgo(30),
  });

  seedAthleteModule(db, { nowIso, daysAgo, addDays });

  return db;
}

// ── Athlete module (Strava-style) ─────────────────────────────────────────────

const M_PER_DEG_LAT = 111_320;

/** Points along a straight corridor with small lateral jitter + gentle elevation. */
function corridorTrack(
  base: { lat: number; lng: number },
  bearingRad: number,
  distanceM: number,
  speedMps: number,
  jitterM = 8,
): TrackPoint[] {
  const points: TrackPoint[] = [];
  const stepSec = 10;
  const totalSec = Math.round(distanceM / speedMps);
  for (let s = 0; s <= totalSec; s += stepSec) {
    const along = Math.min(distanceM, speedMps * s);
    const jitter = faker.number.float({ min: -jitterM, max: jitterM });
    const dLat = (along * Math.cos(bearingRad) + jitter * -Math.sin(bearingRad)) / M_PER_DEG_LAT;
    const dLng =
      (along * Math.sin(bearingRad) + jitter * Math.cos(bearingRad)) /
      (M_PER_DEG_LAT * Math.cos((base.lat * Math.PI) / 180));
    points.push({
      t: s * 1000,
      lat: base.lat + dLat,
      lng: base.lng + dLng,
      ele: 18 + 6 * Math.sin((along / distanceM) * Math.PI * 3) + faker.number.float({ min: -0.2, max: 0.2 }),
    });
  }
  return points;
}

const RUN_BASE = { lat: -6.21, lng: 106.82 };
const RIDE_BASE = { lat: -6.11, lng: 106.74 };
const NORTH = 0;
const WEST = -Math.PI / 2;

function seedAthleteModule(
  db: MockDb,
  helpers: {
    nowIso: string;
    daysAgo: (days: number, hour?: number) => string;
    addDays: (iso: string, days: number) => string;
  },
): void {
  const { nowIso, daysAgo, addDays } = helpers;

  // Segment polylines live on shared "corridors", so tracks that follow the
  // corridor genuinely pass the start/end gates (GPS matching, not shortcuts).
  db.segments = [
    { id: 'seg_1k', name: 'Senopati Sprint', type: 'RUN', distanceM: 1000, location: 'Senopati', path: corridorTrack(RUN_BASE, NORTH, 1000, 10 / 3, 0) },
    { id: 'seg_3k', name: 'GBK Loop', type: 'RUN', distanceM: 3000, location: 'Gelora Bung Karno', path: corridorTrack(RUN_BASE, NORTH, 3000, 10 / 3, 0) },
    { id: 'seg_5k', name: 'Sudirman Stretch', type: 'RUN', distanceM: 5000, location: 'Jl. Sudirman', path: corridorTrack(RUN_BASE, NORTH, 5000, 10 / 3, 0) },
    { id: 'seg_ride', name: 'PIK Coastal Ride', type: 'RIDE', distanceM: 10_000, location: 'PIK', path: corridorTrack(RIDE_BASE, WEST, 10_000, 8, 0) },
  ];

  db.challenges = [
    {
      id: 'chal_run50',
      name: 'Monthly Run 50K',
      description: 'Run 50 kilometers this month.',
      type: 'RUN',
      targetKm: 50,
      startsAt: daysAgo(20),
      endsAt: addDays(nowIso, 10),
    },
    {
      id: 'chal_any100',
      name: 'Monthly Distance 100K',
      description: 'Cover 100 kilometers any way you like.',
      type: 'ANY',
      targetKm: 100,
      startsAt: daysAgo(20),
      endsAt: addDays(nowIso, 10),
    },
  ];

  const athleteIds = ['mem_demo', ...Array.from({ length: 9 }, (_, i) => `mem_s${i + 1}`)];
  db.clubs = [
    {
      id: 'club_run',
      name: 'HYROX Studio Run Club',
      description: 'Tuesday intervals, Sunday long runs. All paces welcome.',
      location: 'Senopati',
      memberIds: [...athleteIds.slice(0, 7)],
    },
    {
      id: 'club_ride',
      name: 'PIK Riders',
      description: 'Coastal loops before the city wakes up.',
      location: 'PIK',
      memberIds: [...athleteIds.slice(5)],
    },
  ];

  db.follows = [
    ...['mem_s1', 'mem_s2', 'mem_s3', 'mem_s4', 'mem_s5'].map((followeeId) => ({
      followerId: 'mem_demo',
      followeeId,
    })),
    { followerId: 'mem_s1', followeeId: 'mem_demo' },
    { followerId: 'mem_s2', followeeId: 'mem_demo' },
    { followerId: 'mem_s6', followeeId: 'mem_demo' },
  ];

  db.gear = [
    { id: 'gear_shoes1', memberId: 'mem_demo', name: 'Asics Novablast 4', kind: 'SHOES', distanceM: 0, retired: false },
    { id: 'gear_shoes2', memberId: 'mem_demo', name: 'Pegasus 39 (retired)', kind: 'SHOES', distanceM: 412_000, retired: true },
    { id: 'gear_bike', memberId: 'mem_demo', name: 'Canyon Endurace', kind: 'BIKE', distanceM: 0, retired: false },
  ];
  db.athleteSettings['mem_demo'] = {
    units: 'METRIC',
    bookingReminders: true,
    weeklyGoalKm: 20,
    language: 'EN',
  };

  let actN = 0;
  let effN = 0;
  const addActivity = (
    memberId: string,
    type: ActivityType,
    title: string,
    startDaysAgo: number,
    startHour: number,
    distanceM: number,
    speedMps: number,
    gearId: string | null = null,
  ): Activity => {
    const base = type === 'RIDE' ? RIDE_BASE : RUN_BASE;
    const bearing = type === 'RIDE' ? WEST : NORTH;
    const points = type === 'WORKOUT' ? [] : corridorTrack(base, bearing, distanceM, speedMps);
    const stats = computeActivityStats(points);
    const activity: Activity = {
      id: `act_${++actN}`,
      memberId,
      type,
      title,
      description: '',
      startedAt: daysAgo(startDaysAgo, startHour),
      elapsedSec: type === 'WORKOUT' ? Math.round(distanceM / Math.max(speedMps, 1)) : stats.elapsedSec,
      movingSec: type === 'WORKOUT' ? Math.round(distanceM / Math.max(speedMps, 1)) : stats.movingSec,
      distanceM: stats.distanceM,
      avgPaceSecPerKm: stats.avgPaceSecPerKm,
      elevationGainM: stats.elevationGainM,
      points,
      photos: [],
      visibility: 'EVERYONE',
      gearId,
      createdAt: daysAgo(startDaysAgo, startHour + 1),
    };
    db.activities.push(activity);
    for (const match of matchSegments(db.segments, activity, points)) {
      db.segmentEfforts.push({
        id: `eff_${++effN}`,
        segmentId: match.segment.id,
        activityId: activity.id,
        memberId,
        elapsedSec: match.elapsedSec,
        createdAt: activity.createdAt,
      });
    }
    if (gearId) {
      const gear = db.gear.find((g) => g.id === gearId);
      if (gear) gear.distanceM += activity.distanceM;
    }
    return activity;
  };

  // Demo member: a believable training log along the corridors.
  addActivity('mem_demo', 'RUN', 'Easy Morning Run', 18, 6, 5600, 2.9, 'gear_shoes1');
  addActivity('mem_demo', 'RUN', 'Tempo Thursday', 14, 6, 5700, 3.4, 'gear_shoes1');
  addActivity('mem_demo', 'RIDE', 'PIK Coastal Spin', 11, 7, 20_000, 7.5, 'gear_bike');
  addActivity('mem_demo', 'RUN', 'Sunday Long Run', 7, 6, 11_200, 3.0, 'gear_shoes1');
  const demoLatest = addActivity('mem_demo', 'RUN', 'Lunch Shakeout', 1, 12, 4600, 3.2, 'gear_shoes1');

  // Other athletes: 2–4 activities each over the past two weeks.
  for (const memberId of athleteIds.slice(1)) {
    const count = faker.number.int({ min: 2, max: 4 });
    for (let i = 0; i < count; i++) {
      const type: ActivityType = faker.helpers.weightedArrayElement([
        { value: 'RUN', weight: 6 },
        { value: 'RIDE', weight: 2 },
        { value: 'WALK', weight: 1 },
      ]);
      const speed =
        type === 'RUN'
          ? faker.number.float({ min: 2.6, max: 3.6 })
          : type === 'RIDE'
            ? faker.number.float({ min: 6, max: 9 })
            : 1.5;
      const distance =
        type === 'RIDE'
          ? faker.number.int({ min: 11_000, max: 26_000 })
          : faker.number.int({ min: 3200, max: 9000 });
      addActivity(
        memberId,
        type,
        faker.helpers.arrayElement(['Morning Run', 'Evening Run', 'Intervals', 'Recovery Spin', 'Long Run', 'Commute']),
        faker.number.int({ min: 0, max: 14 }),
        faker.helpers.arrayElement([6, 7, 17, 19]),
        distance,
        speed,
      );
    }
  }

  // Kudos + comments.
  for (const activity of db.activities) {
    const fans = faker.helpers.arrayElements(
      athleteIds.filter((id) => id !== activity.memberId),
      faker.number.int({ min: 0, max: 5 }),
    );
    for (const memberId of fans) {
      db.kudos.push({ activityId: activity.id, memberId, createdAt: activity.createdAt });
    }
  }
  db.activityComments.push(
    {
      id: 'cmt_seed_1',
      activityId: demoLatest.id,
      memberId: 'mem_s1',
      text: 'Strong pace! See you at Full Simulation Saturday?',
      createdAt: daysAgo(1, 14),
    },
    {
      id: 'cmt_seed_2',
      activityId: demoLatest.id,
      memberId: 'mem_s2',
      text: 'That midday heat though. Respect.',
      createdAt: daysAgo(1, 15),
    },
  );

  // Challenge participation.
  for (const memberId of athleteIds) {
    db.challengeJoins.push({ challengeId: 'chal_any100', memberId });
    if (faker.datatype.boolean()) db.challengeJoins.push({ challengeId: 'chal_run50', memberId });
  }
  if (!db.challengeJoins.some((j) => j.challengeId === 'chal_run50' && j.memberId === 'mem_demo')) {
    db.challengeJoins.push({ challengeId: 'chal_run50', memberId: 'mem_demo' });
  }

  // A saved route for the demo member.
  db.routes.push({
    id: 'rte_seed_1',
    memberId: 'mem_demo',
    name: 'Sudirman Out & Back',
    points: demoLatest.points,
    distanceM: demoLatest.distanceM,
    createdAt: daysAgo(1, 13),
  });

  seedHyroxModule(db, helpers);
  seedRaceModule(db, helpers);
}

// ── HYROX workout module (phase 3) ────────────────────────────────────────────
function seedHyroxModule(
  db: MockDb,
  helpers: { daysAgo: (days: number, hour?: number) => string },
): void {
  const { daysAgo } = helpers;
  const station = (
    id: string,
    name: string,
    order: number,
    category: Exercise['category'],
    equipment: string[],
    spec: { distanceM?: number; reps?: number },
    difficulty: 1 | 2 | 3,
  ): Omit<Exercise, 'videoUrl'> => ({
    id,
    name,
    category,
    equipment,
    hyroxStationOrder: order,
    difficulty,
    defaultSpec: { distanceM: spec.distanceM ?? null, reps: spec.reps ?? null },
  });

  // Real how-to videos (YouTube), one per exercise.
  const HOW_TO: Record<string, string> = {
    ex_run: 'https://www.youtube.com/watch?v=CpdciSr5gnQ',
    ex_ski: 'https://www.youtube.com/watch?v=9RJiSvgaiJU',
    ex_sledpush: 'https://www.youtube.com/watch?v=SAh6C_QluJE',
    ex_sledpull: 'https://www.youtube.com/watch?v=9lTv65mWPHA',
    ex_bbj: 'https://www.youtube.com/watch?v=UTO-GzRXF-Q',
    ex_row: 'https://www.youtube.com/watch?v=cGy29PC4oPk',
    ex_carry: 'https://www.youtube.com/watch?v=o8bdBfBvWdc',
    ex_lunge: 'https://www.youtube.com/watch?v=YlFsbfK5Doc',
    ex_wb: 'https://www.youtube.com/watch?v=bm7QLEOx26c',
    ex_slam: 'https://www.youtube.com/watch?v=6vXHh-Lhb2o',
    ex_plate: 'https://www.youtube.com/watch?v=oRyDt3ivTag',
    ex_band: 'https://www.youtube.com/watch?v=aA2yvYz6xs8',
    ex_dblunge: 'https://www.youtube.com/watch?v=Tc1TsAdoDRo',
    ex_thruster: 'https://www.youtube.com/watch?v=FVKHh-sotqY',
  };
  const baseExercises: Omit<Exercise, 'videoUrl'>[] = [
    { id: 'ex_run', name: 'Running', category: 'RUN', equipment: [], hyroxStationOrder: null, difficulty: 1, defaultSpec: { distanceM: 1000, reps: null } },
    station('ex_ski', 'SkiErg', 1, 'ERG', ['SkiErg'], { distanceM: 1000 }, 2),
    station('ex_sledpush', 'Sled Push', 2, 'SLED', ['Sled'], { distanceM: 50 }, 3),
    station('ex_sledpull', 'Sled Pull', 3, 'SLED', ['Sled'], { distanceM: 50 }, 3),
    station('ex_bbj', 'Burpee Broad Jump', 4, 'JUMP', [], { distanceM: 80 }, 3),
    station('ex_row', 'Row', 5, 'ERG', ['Rower'], { distanceM: 1000 }, 2),
    station('ex_carry', 'Farmers Carry', 6, 'CARRY', ['Kettlebells'], { distanceM: 200 }, 2),
    station('ex_lunge', 'Sandbag Lunge', 7, 'LUNGE', ['Sandbag'], { distanceM: 100 }, 3),
    station('ex_wb', 'Wall Balls', 8, 'THROW', ['Wall ball'], { reps: 100 }, 2),
    { id: 'ex_slam', name: 'Medicine Ball Slam', category: 'CONDITIONING', equipment: ['Medicine ball'], hyroxStationOrder: null, difficulty: 2, defaultSpec: { distanceM: null, reps: 40 } },
    { id: 'ex_plate', name: 'Plate Push', category: 'SLED', equipment: ['Plate'], hyroxStationOrder: null, difficulty: 2, defaultSpec: { distanceM: 50, reps: null } },
    { id: 'ex_band', name: 'Banded Row Pull', category: 'SLED', equipment: ['Band'], hyroxStationOrder: null, difficulty: 2, defaultSpec: { distanceM: 50, reps: null } },
    { id: 'ex_dblunge', name: 'Dumbbell Lunge', category: 'LUNGE', equipment: ['Dumbbells'], hyroxStationOrder: null, difficulty: 2, defaultSpec: { distanceM: 100, reps: null } },
    { id: 'ex_thruster', name: 'Dumbbell Thruster', category: 'THROW', equipment: ['Dumbbells'], hyroxStationOrder: null, difficulty: 2, defaultSpec: { distanceM: null, reps: 60 } },
  ];
  db.exercises = baseExercises.map((ex) => ({ ...ex, videoUrl: HOW_TO[ex.id] ?? null }));

  db.substitutions = [
    { originalExerciseId: 'ex_ski', alternativeExerciseId: 'ex_row', similarity: 0.9, conversionNote: '1:1 distance' },
    { originalExerciseId: 'ex_ski', alternativeExerciseId: 'ex_slam', similarity: 0.75, conversionNote: '1000 m → 40 slams' },
    { originalExerciseId: 'ex_sledpush', alternativeExerciseId: 'ex_plate', similarity: 0.85, conversionNote: 'Same distance, heavy plate' },
    { originalExerciseId: 'ex_sledpull', alternativeExerciseId: 'ex_band', similarity: 0.8, conversionNote: 'Same distance, heavy band' },
    { originalExerciseId: 'ex_row', alternativeExerciseId: 'ex_ski', similarity: 0.9, conversionNote: '1:1 distance' },
    { originalExerciseId: 'ex_lunge', alternativeExerciseId: 'ex_dblunge', similarity: 0.85, conversionNote: 'Bag → dumbbells' },
    { originalExerciseId: 'ex_wb', alternativeExerciseId: 'ex_thruster', similarity: 0.8, conversionNote: '100 wall balls → 60 thrusters' },
  ];

  // One completed full simulation for the demo member → race prediction works.
  const { blocks, totalTargetSec } = generateWorkout({
    type: 'FULL_SIMULATION',
    division: 'MEN_OPEN',
    stationOrders: [],
    excludedExerciseIds: [],
    exercises: db.exercises,
    substitutions: db.substitutions,
    pick: () => 0,
  });
  const workout: GeneratedWorkout = {
    id: 'wko_seed_1',
    memberId: 'mem_demo',
    type: 'FULL_SIMULATION',
    division: 'MEN_OPEN',
    blocks,
    excludedExerciseIds: [],
    totalTargetSec,
    createdAt: daysAgo(9, 8),
  };
  db.workouts.push(workout);
  db.workoutSessions.push({
    id: 'wses_seed_1',
    workoutId: workout.id,
    memberId: 'mem_demo',
    status: 'COMPLETED',
    currentBlock: blocks[blocks.length - 1]!.order,
    startedAt: daysAgo(9, 8),
    endedAt: daysAgo(9, 10),
    blockResults: blocks.map((b) => ({
      order: b.order,
      durationSec: Math.round(b.targetSec * faker.number.float({ min: 0.95, max: 1.15 })),
    })),
    pauseCount: 1,
    totalPauseSec: 140,
    createdAt: daysAgo(9, 8),
  });
}

// ── Race ecosystem (phase 4) ──────────────────────────────────────────────────
function seedRaceModule(
  db: MockDb,
  helpers: { nowIso: string; addDays: (iso: string, days: number) => string; daysAgo: (days: number, hour?: number) => string },
): void {
  const { nowIso, addDays, daysAgo } = helpers;
  // Photos are bundled with both apps under public/img (originally free
  // Unsplash photos), so cards render with zero external requests.
  const race = (
    id: string,
    name: string,
    country: string,
    region: RaceEvent['region'],
    city: string,
    venue: string,
    inDays: number,
    status: RaceEvent['status'],
    image: string,
  ): RaceEvent => ({
    id,
    name,
    country,
    region,
    city,
    venue,
    startsAt: inDays >= 0 ? addDays(nowIso, inDays) : daysAgo(-inDays, 8),
    endsAt: inDays >= 0 ? addDays(nowIso, inDays + 1) : daysAgo(-inDays - 1, 20),
    registrationUrl: 'https://hyrox.com/find-my-race',
    imageUrl: `/img/${image}.jpg`,
    status,
  });

  db.raceEvents = [
    race('race_jkt', 'HYROX Jakarta', 'Indonesia', 'ASIA', 'Jakarta', 'JIExpo Kemayoran', 41, 'REGISTRATION_OPEN', 'race-jakarta'),
    race('race_sgp', 'HYROX Singapore', 'Singapore', 'ASIA', 'Singapore', 'Expo Hall 5', 20, 'SOLD_OUT', 'race-singapore'),
    race('race_bkk', 'HYROX Bangkok', 'Thailand', 'ASIA', 'Bangkok', 'IMPACT Arena', 69, 'REGISTRATION_OPEN', 'race-bangkok'),
    race('race_hkg', 'HYROX Hong Kong', 'China', 'ASIA', 'Hong Kong', 'AsiaWorld-Expo', 97, 'ANNOUNCED', 'race-hongkong'),
    race('race_syd', 'HYROX Sydney', 'Australia', 'OCEANIA', 'Sydney', 'ICC Sydney', 55, 'REGISTRATION_OPEN', 'race-sydney'),
    race('race_ber', 'HYROX Berlin', 'Germany', 'EUROPE', 'Berlin', 'Messe Berlin', 76, 'REGISTRATION_OPEN', 'race-berlin'),
    race('race_nyc', 'HYROX New York', 'USA', 'AMERICAS', 'New York', 'Pier 76', 112, 'ANNOUNCED', 'race-newyork'),
    race('race_kul', 'HYROX Kuala Lumpur', 'Malaysia', 'ASIA', 'Kuala Lumpur', 'MITEC', -35, 'COMPLETED', 'race-kualalumpur'),
  ];

  db.userRaces = [
    {
      id: 'urc_seed_1',
      memberId: 'mem_demo',
      raceEventId: 'race_jkt',
      division: 'MEN_OPEN',
      goalSec: 90 * 60,
      status: 'TRAINING',
      resultSec: null,
      createdAt: daysAgo(12),
    },
    {
      id: 'urc_seed_2',
      memberId: 'mem_demo',
      raceEventId: 'race_kul',
      division: 'MEN_OPEN',
      goalSec: 95 * 60,
      status: 'RACED',
      resultSec: 93 * 60 + 41,
      createdAt: daysAgo(80),
    },
    {
      id: 'urc_seed_3',
      memberId: 'mem_s1',
      raceEventId: 'race_jkt',
      division: 'WOMEN_OPEN',
      goalSec: null,
      status: 'TRAINING',
      resultSec: null,
      createdAt: daysAgo(5),
    },
  ];
}
