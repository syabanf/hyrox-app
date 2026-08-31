import type { MemberNotificationType } from '@hyrox/domain';
import { EmptyState, Spinner, formatDayTime } from '@hyrox/ui';
import { useMutation } from '@tanstack/react-query';
import {
  AlertTriangle,
  ArrowUpCircle,
  Bell,
  CalendarDays,
  CheckCircle2,
  Clock,
  Dumbbell,
  Hourglass,
  Megaphone,
} from 'lucide-react';
import { api } from '../../lib/api';
import { useInvalidateAll, useNotifications } from '../../lib/queries';

const TYPE_ICON: Record<MemberNotificationType, typeof Bell> = {
  BOOKING_CONFIRMED: CheckCircle2,
  BOOKING_REMINDER: Clock,
  WAITLIST_PROMOTED: ArrowUpCircle,
  LOW_BALANCE: AlertTriangle,
  CREDIT_EXPIRY: Hourglass,
  VISIT_LOGGED: Dumbbell,
  SESSION_CHANGED: CalendarDays,
  ANNOUNCEMENT: Megaphone,
};

export function NotificationsPage() {
  const { data: notifications, isLoading } = useNotifications();
  const invalidate = useInvalidateAll();
  const readAll = useMutation({
    mutationFn: api.me.readAllNotifications,
    onSuccess: invalidate,
  });

  if (isLoading) return <Spinner label="Loading notifications…" />;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="display text-2xl font-black">Notifications</h1>
        <button
          className="text-sm font-bold text-brand"
          onClick={() => readAll.mutate()}
          disabled={readAll.isPending}
        >
          Mark all read
        </button>
      </div>
      {!notifications || notifications.length === 0 ? (
        <EmptyState title="Nothing here yet" hint="Booking updates and reminders will show up here." />
      ) : (
        <div className="flex flex-col gap-2">
          {notifications.map((n) => {
            const Icon = TYPE_ICON[n.type] ?? Bell;
            return (
              <div
                key={n.id}
                className={`card flex gap-3 !p-3 ${n.readAt === null ? '!border-brand/50' : 'opacity-70'}`}
              >
                <Icon size={20} className="mt-0.5 shrink-0 text-brand" />
                <div className="min-w-0">
                  <p className="text-sm font-black">{n.title}</p>
                  <p className="text-sm text-muted">{n.body}</p>
                  <p className="mt-1 text-xs text-muted/60">{formatDayTime(n.createdAt)}</p>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
