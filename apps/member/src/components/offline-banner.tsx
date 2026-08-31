import { WifiOff } from 'lucide-react';
import { useEffect, useState } from 'react';

/**
 * The app keeps working offline (the mock API lives in the browser and state is
 * snapshotted to localStorage) — this banner just makes the mode visible.
 */
export function OfflineBanner() {
  const [online, setOnline] = useState(() =>
    typeof navigator === 'undefined' ? true : navigator.onLine,
  );
  useEffect(() => {
    const up = () => setOnline(true);
    const down = () => setOnline(false);
    window.addEventListener('online', up);
    window.addEventListener('offline', down);
    return () => {
      window.removeEventListener('online', up);
      window.removeEventListener('offline', down);
    };
  }, []);

  if (online) return null;
  return (
    <div className="sticky top-0 z-30 flex items-center justify-center gap-2 bg-ink px-4 py-2 text-xs font-bold uppercase tracking-wider text-white">
      <WifiOff size={14} className="text-brand" />
      Offline — changes are saved on this device
    </div>
  );
}
