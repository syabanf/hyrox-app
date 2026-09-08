/**
 * A small picture beside a row.
 *
 * Falls back to the first letter of the name rather than a broken-image icon:
 * plenty of rows have no picture, and a grey square with a letter in it reads
 * as "no photo yet" where a broken frame reads as "this page is broken".
 */
export function Thumb({
  src,
  name,
  size = 40,
}: {
  src?: string | null;
  name: string;
  size?: number;
}) {
  if (!src) {
    return (
      <span
        className="flex shrink-0 items-center justify-center rounded-xl bg-surface-raised text-sm font-black text-muted"
        style={{ width: size, height: size }}
        aria-hidden
      >
        {name.trim().charAt(0).toUpperCase() || '?'}
      </span>
    );
  }
  return (
    <img
      src={src}
      alt=""
      width={size}
      height={size}
      loading="lazy"
      className="shrink-0 rounded-xl bg-surface-raised object-cover"
      style={{ width: size, height: size }}
    />
  );
}
