import type { CalendarDate, Holiday, Shift, WallClock } from '@nuhabit/domain';

/**
 * The HR arithmetic, in TypeScript, for the offline demo only.
 *
 * The Go backend owns these rules — `internal/domain/hris.go` is the source of
 * truth and the place they are tested exhaustively. This file exists so the
 * admin panel's People pages work when it is answering its own requests with
 * no server behind it. It is deliberately the same behaviour, deliberately not
 * the same code: if the two ever disagree, the backend is right.
 */

/** A calendar date in the studio's timezone, free of the browser's. */
export function studioDate(iso: string): CalendarDate {
  return new Date(iso).toLocaleDateString('en-CA', { timeZone: 'Asia/Jakarta' });
}

export function addDays(date: CalendarDate, days: number): CalendarDate {
  const d = new Date(`${date}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

/** ISO weekday: 1 is Monday, 7 is Sunday. */
export function isoDayOfWeek(date: CalendarDate): number {
  const day = new Date(`${date}T00:00:00Z`).getUTCDay();
  return day === 0 ? 7 : day;
}

export function isWeekend(date: CalendarDate): boolean {
  return isoDayOfWeek(date) >= 6;
}

export function datesBetween(start: CalendarDate, end: CalendarDate): CalendarDate[] {
  if (end < start) return [];
  const out: CalendarDate[] = [];
  for (let cursor = start; cursor <= end; cursor = addDays(cursor, 1)) out.push(cursor);
  return out;
}

/** Jakarta is UTC+7 the whole year, which is what makes this a constant. */
const STUDIO_OFFSET = '+07:00';

/** The instant a wall-clock time happens on a date in the studio. */
export function instantAt(date: CalendarDate, clock: WallClock): string {
  return new Date(`${date}T${clock.slice(0, 8)}${STUDIO_OFFSET}`).toISOString();
}

export function scheduledWindow(date: CalendarDate, shift: Shift): { start: string; end: string } {
  const endDate = shift.isOvernight ? addDays(date, 1) : date;
  return { start: instantAt(date, shift.startTime), end: instantAt(endDate, shift.endTime) };
}

/**
 * How late an arrival was.
 *
 * The tolerance decides *whether* someone is late; the count is measured from
 * the shift's start time, not from the end of the tolerance. Twelve minutes
 * into a shift with ten minutes' grace is twelve minutes late, not two.
 */
export function computeLateness(
  clockInIso: string,
  date: CalendarDate,
  shift: Shift,
): { isLate: boolean; lateMinutes: number } {
  const { start } = scheduledWindow(date, shift);
  const startMs = new Date(start).getTime();
  const clockInMs = new Date(clockInIso).getTime();
  if (clockInMs <= startMs + shift.lateToleranceMinutes * 60_000) {
    return { isLate: false, lateMinutes: 0 };
  }
  return { isLate: true, lateMinutes: Math.ceil((clockInMs - startMs) / 60_000) };
}

/** Time on the clock minus the unpaid break, never negative. */
export function workHours(clockIn: string, clockOut: string, breakMinutes: number): number {
  const worked =
    new Date(clockOut).getTime() - new Date(clockIn).getTime() - breakMinutes * 60_000;
  if (worked <= 0) return 0;
  return Math.round((worked / 3_600_000) * 100) / 100;
}

/** The span between two wall-clock times, rolling past midnight. */
export function overtimeHours(start: WallClock, end: WallClock): number {
  const minutes = (clock: WallClock) => {
    const [h = '0', m = '0'] = clock.split(':');
    return Number(h) * 60 + Number(m);
  };
  const from = minutes(start);
  let to = minutes(end);
  if (to <= from) to += 24 * 60;
  return Math.round(((to - from) / 60) * 100) / 100;
}

export interface LeaveDaysBreakdown {
  totalDays: number;
  excludedHolidays: { date: CalendarDate; name: string }[];
}

/**
 * What a leave request actually costs.
 *
 * A date is free if it carries a holiday that does not deduct leave, or if it
 * falls on a weekend. Where a national holiday and a collective leave day land
 * on the same date the national one wins: it genuinely is a public holiday,
 * and that reading also favours the employee.
 */
export function describeLeaveDays(
  start: CalendarDate,
  end: CalendarDate,
  holidays: Holiday[],
): LeaveDaysBreakdown {
  const byDate = new Map<CalendarDate, Holiday[]>();
  for (const holiday of holidays) {
    byDate.set(holiday.date, [...(byDate.get(holiday.date) ?? []), holiday]);
  }

  const breakdown: LeaveDaysBreakdown = { totalDays: 0, excludedHolidays: [] };
  for (const date of datesBetween(start, end)) {
    const free = (byDate.get(date) ?? []).find((h) => !h.deductsLeave);
    if (free) {
      breakdown.excludedHolidays.push({ date, name: free.name });
      continue;
    }
    if (isWeekend(date)) continue;
    breakdown.totalDays += 1;
  }
  return breakdown;
}
