/**
 * Free stock photos (Unsplash License) for class types, loaded from the
 * Unsplash CDN and runtime-cached by the service worker for offline use.
 */
const unsplash = (photoId: string) => `https://images.unsplash.com/${photoId}?w=640&q=60&fit=crop`;

const CLASS_IMAGES: Record<string, string> = {
  cls_fund: unsplash('photo-1534438327276-14e5300c3a48'),
  cls_sim: unsplash('photo-1571019613454-1cb2f99b2d8b'),
  cls_str: unsplash('photo-1526506118085-60ce8714f8c5'),
  cls_eng: unsplash('photo-1519505907962-0a6cb0167c73'),
  cls_open: unsplash('photo-1540497077202-7c8a3999166f'),
  cls_mob: unsplash('photo-1544367567-0f2fcb009e0b'),
  cls_wod: unsplash('photo-1476480862126-209bfaa8edc8'),
  cls_test: unsplash('photo-1552674605-db6ffd4facb5'),
};

export function classImage(classTypeId: string): string | null {
  return CLASS_IMAGES[classTypeId] ?? null;
}
