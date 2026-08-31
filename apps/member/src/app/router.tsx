import { createBrowserRouter } from 'react-router';
import { AppLayout, RequireAuth } from '../components/layout';
import { LoginPage } from '../features/auth/login-page';
import { RegisterPage } from '../features/auth/register-page';
import { BookingsPage } from '../features/classes/bookings-page';
import { SchedulePage } from '../features/classes/schedule-page';
import { SessionDetailPage } from '../features/classes/session-detail-page';
import { HomePage } from '../features/home/home-page';
import { NotificationsPage } from '../features/notifications/notifications-page';
import { EmergencyContactPage } from '../features/profile/emergency-page';
import { GearPage } from '../features/profile/gear-page';
import { TutorialsPage } from '../features/train/tutorials-page';
import { AnnouncementDetailPage } from '../features/home/announcement-detail-page';
import { PromoDetailPage } from '../features/home/promo-detail-page';
import { ChallengeDetailPage } from '../features/train/challenge-detail-page';
import { AthleteProfilePage } from '../features/train/athlete-profile-page';
import { RaceDetailPage } from '../features/races/race-detail-page';
import { ProfilePage } from '../features/profile/profile-page';
import { SettingsPage } from '../features/profile/settings-page';
import { QrPage } from '../features/qr/qr-page';
import { VisitsPage } from '../features/qr/visits-page';
import { RacesPage } from '../features/races/races-page';
import { ActivityDetailPage } from '../features/train/activity-detail-page';
import { ExplorePage } from '../features/train/explore-page';
import { FeedPage } from '../features/train/feed-page';
import { HeatmapPage } from '../features/train/heatmap-page';
import { RecordPage } from '../features/train/record-page';
import { SegmentPage } from '../features/train/segment-page';
import { YouPage } from '../features/train/you-page';
import { WorkoutActivePage } from '../features/workout/active-page';
import { WorkoutGeneratorPage } from '../features/workout/generator-page';
import { WorkoutPreviewPage } from '../features/workout/preview-page';
import { PaymentPage } from '../features/wallet/payment-page';
import { TopUpPage } from '../features/wallet/topup-page';
import { WalletPage } from '../features/wallet/wallet-page';

export const router = createBrowserRouter([
  { path: '/auth/login', element: <LoginPage /> },
  { path: '/auth/register', element: <RegisterPage /> },
  {
    element: (
      <RequireAuth>
        <AppLayout />
      </RequireAuth>
    ),
    children: [
      { path: '/', element: <HomePage /> },
      { path: '/classes', element: <SchedulePage /> },
      { path: '/classes/:sessionId', element: <SessionDetailPage /> },
      { path: '/bookings', element: <BookingsPage /> },
      { path: '/qr', element: <QrPage /> },
      { path: '/visits', element: <VisitsPage /> },
      { path: '/wallet', element: <WalletPage /> },
      { path: '/wallet/topup', element: <TopUpPage /> },
      { path: '/wallet/pay/:paymentId', element: <PaymentPage /> },
      { path: '/notifications', element: <NotificationsPage /> },
      { path: '/profile', element: <ProfilePage /> },
      { path: '/profile/settings', element: <SettingsPage /> },
      { path: '/profile/emergency', element: <EmergencyContactPage /> },
      { path: '/profile/gear', element: <GearPage /> },
      { path: '/train', element: <FeedPage /> },
      { path: '/train/record', element: <RecordPage /> },
      { path: '/train/you', element: <YouPage /> },
      { path: '/train/explore', element: <ExplorePage /> },
      { path: '/train/tutorials', element: <TutorialsPage /> },
      { path: '/train/challenges/:challengeId', element: <ChallengeDetailPage /> },
      { path: '/train/athletes/:memberId', element: <AthleteProfilePage /> },
      { path: '/announcements/:announcementId', element: <AnnouncementDetailPage /> },
      { path: '/promos/:code', element: <PromoDetailPage /> },
      { path: '/train/activities/:activityId', element: <ActivityDetailPage /> },
      { path: '/train/segments/:segmentId', element: <SegmentPage /> },
      { path: '/train/heatmap', element: <HeatmapPage /> },
      { path: '/workout', element: <WorkoutGeneratorPage /> },
      { path: '/workout/preview/:workoutId', element: <WorkoutPreviewPage /> },
      { path: '/workout/active/:sessionId', element: <WorkoutActivePage /> },
      { path: '/races', element: <RacesPage /> },
      { path: '/races/:raceId', element: <RaceDetailPage /> },
    ],
  },
]);
