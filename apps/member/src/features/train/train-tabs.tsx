import { NavLink } from 'react-router';

const TABS = [
  { to: '/train', label: 'Feed', end: true },
  { to: '/train/record', label: 'Record', end: false },
  { to: '/train/you', label: 'You', end: false },
  { to: '/train/explore', label: 'Explore', end: false },
  { to: '/train/tutorials', label: 'Guides', end: false },
];

export function TrainTabs() {
  return (
    <div className="mb-4 flex rounded-xl bg-surface p-1 shadow-[0_1px_2px_rgb(0_0_0/0.04)]">
      {TABS.map((t) => (
        <NavLink
          key={t.to}
          to={t.to}
          end={t.end}
          className={({ isActive }) =>
            `flex-1 rounded-lg py-2 text-center text-xs font-black uppercase tracking-wide ${
              isActive ? 'bg-brand text-white' : 'text-muted'
            }`
          }
        >
          {t.label}
        </NavLink>
      ))}
    </div>
  );
}
