/** Per-class-type session rundowns, scaled to the session's real duration. */

interface Segment {
  label: string;
  minutes: number;
}

const BASE: Record<string, Segment[]> = {
  cls_fund: [
    { label: 'Check-in & briefing', minutes: 5 },
    { label: 'Warm-up', minutes: 10 },
    { label: 'Technique stations', minutes: 20 },
    { label: 'Mini workout', minutes: 15 },
    { label: 'Cooldown & stretch', minutes: 10 },
  ],
  cls_sim: [
    { label: 'Briefing & lane setup', minutes: 5 },
    { label: 'Warm-up', minutes: 10 },
    { label: 'Full race simulation', minutes: 40 },
    { label: 'Cooldown', minutes: 5 },
  ],
  cls_str: [
    { label: 'Warm-up & activation', minutes: 10 },
    { label: 'Strength blocks', minutes: 35 },
    { label: 'Accessory work', minutes: 10 },
    { label: 'Stretch', minutes: 5 },
  ],
  cls_eng: [
    { label: 'Warm-up', minutes: 10 },
    { label: 'Engine intervals', minutes: 40 },
    { label: 'Cooldown', minutes: 10 },
  ],
  cls_open: [
    { label: 'Check-in', minutes: 5 },
    { label: 'Open floor', minutes: 50 },
    { label: 'Cooldown', minutes: 5 },
  ],
  cls_mob: [
    { label: 'Breathwork', minutes: 5 },
    { label: 'Mobility flow', minutes: 45 },
    { label: 'Relaxation', minutes: 10 },
  ],
  cls_wod: [
    { label: 'Briefing', minutes: 5 },
    { label: 'Warm-up', minutes: 10 },
    { label: 'Team WOD', minutes: 35 },
    { label: 'Cooldown', minutes: 10 },
  ],
  cls_test: [
    { label: 'Briefing', minutes: 5 },
    { label: 'Warm-up', minutes: 10 },
    { label: 'Benchmark test', minutes: 40 },
    { label: 'Cooldown', minutes: 5 },
  ],
};

const DEFAULT: Segment[] = [
  { label: 'Check-in & briefing', minutes: 5 },
  { label: 'Warm-up', minutes: 10 },
  { label: 'Main workout', minutes: 35 },
  { label: 'Cooldown & stretch', minutes: 10 },
];

export interface RundownItem {
  startsAt: Date;
  label: string;
  minutes: number;
}

export function sessionRundown(
  classTypeId: string,
  startsAtIso: string,
  endsAtIso: string,
): RundownItem[] {
  const template = BASE[classTypeId] ?? DEFAULT;
  const totalTemplate = template.reduce((sum, seg) => sum + seg.minutes, 0);
  const actualMin = Math.max(
    10,
    Math.round((new Date(endsAtIso).getTime() - new Date(startsAtIso).getTime()) / 60_000),
  );
  let cursor = new Date(startsAtIso).getTime();
  return template.map((seg, i) => {
    const minutes =
      i === template.length - 1
        ? Math.max(1, Math.round(actualMin - (cursor - new Date(startsAtIso).getTime()) / 60_000))
        : Math.max(1, Math.round((seg.minutes / totalTemplate) * actualMin));
    const item = { startsAt: new Date(cursor), label: seg.label, minutes };
    cursor += minutes * 60_000;
    return item;
  });
}
