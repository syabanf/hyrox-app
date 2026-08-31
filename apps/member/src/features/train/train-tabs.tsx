import { CircleDot, CirclePlay, Compass, Newspaper, User } from 'lucide-react';
import { NavLink } from 'react-router';

const TABS = [
  { to: '/train', label: 'Feed', end: true, icon: Newspaper },
  { to: '/train/record', label: 'Record', end: false, icon: CircleDot },
  { to: '/train/you', label: 'You', end: false, icon: User },
  { to: '/train/explore', label: 'Explore', end: false, icon: Compass },
  { to: '/train/tutorials', label: 'Guides', end: false, icon: CirclePlay },
];

export function TrainTabs() {
  return (
    <div className="mb-4 flex gap-1 rounded-2xl bg-surface p-1.5 shadow-[0_1px_2px_rgb(17_17_20/0.04),0_8px_24px_rgb(17_17_20/0.04)]">
      {TABS.map(({ to, label, end, icon: Icon }) => (
        <NavLink
          key={to}
          to={to}
          end={end}
          className={({ isActive }) =>
            `flex flex-1 flex-col items-center gap-1 rounded-xl py-2 transition-colors duration-200 ${
              isActive ? 'surface-ink text-white shadow-[0_6px_16px_rgb(13_13_16/0.25)]' : 'text-muted'
            }`
          }
        >
          {({ isActive }) => (
            <>
              <Icon size={15} strokeWidth={2.4} className={isActive ? 'text-[#ff4348]' : undefined} />
              <span className="text-[10px] font-black uppercase tracking-wide">{label}</span>
            </>
          )}
        </NavLink>
      ))}
    </div>
  );
}
