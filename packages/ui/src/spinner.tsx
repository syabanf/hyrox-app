export function Spinner({ label }: { label?: string }) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '0.75rem',
        padding: '2rem',
        color: 'inherit',
      }}
      role="status"
    >
      <span className="hx-spinner" />
      {label ? <span style={{ fontSize: '0.875rem', opacity: 0.7 }}>{label}</span> : null}
    </div>
  );
}
