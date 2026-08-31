/** All domain timestamps are ISO 8601 strings so they serialize losslessly through JSON. */
export type IsoDate = string;

export const msOf = (iso: IsoDate): number => Date.parse(iso);

export const isBefore = (a: IsoDate, b: IsoDate): boolean => msOf(a) < msOf(b);
export const isAfter = (a: IsoDate, b: IsoDate): boolean => msOf(a) > msOf(b);
export const isSameOrBefore = (a: IsoDate, b: IsoDate): boolean => msOf(a) <= msOf(b);

export const addSecondsIso = (iso: IsoDate, seconds: number): IsoDate =>
  new Date(msOf(iso) + seconds * 1000).toISOString();
export const addMinutesIso = (iso: IsoDate, minutes: number): IsoDate =>
  addSecondsIso(iso, minutes * 60);
export const addHoursIso = (iso: IsoDate, hours: number): IsoDate => addMinutesIso(iso, hours * 60);
export const addDaysIso = (iso: IsoDate, days: number): IsoDate => addHoursIso(iso, days * 24);

/** Minutes from `a` to `b` (positive when b is later). */
export const minutesBetween = (a: IsoDate, b: IsoDate): number => (msOf(b) - msOf(a)) / 60_000;
export const secondsBetween = (a: IsoDate, b: IsoDate): number => (msOf(b) - msOf(a)) / 1000;
