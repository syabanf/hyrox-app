/**
 * Class-type photos, bundled with the app (originally free Unsplash photos,
 * committed under public/img so the demo works fully offline).
 */
const CLASS_IMAGES: Record<string, string> = {
  cls_fund: '/img/class-fund.jpg',
  cls_sim: '/img/class-sim.jpg',
  cls_str: '/img/class-str.jpg',
  cls_eng: '/img/class-eng.jpg',
  cls_open: '/img/class-open.jpg',
  cls_mob: '/img/class-mob.jpg',
  cls_wod: '/img/class-wod.jpg',
  cls_test: '/img/class-test.jpg',
};

export function classImage(classTypeId: string): string | null {
  return CLASS_IMAGES[classTypeId] ?? null;
}
