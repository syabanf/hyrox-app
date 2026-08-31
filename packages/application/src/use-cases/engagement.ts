import type { Campaign, Member, MemberSegment, Result, SegmentFilter } from '@hyrox/domain';
import { CAMPAIGN_TRANSITIONS, canTransition, err, msOf, ok } from '@hyrox/domain';
import type { AppError } from '../common';
import { appError, balanceOf, notify } from '../common';
import type { UseCaseDeps } from '../ports';
import { expiringCreditsFor } from './wallet';

const DAY_MS = 24 * 60 * 60_000;

export function segmentMembers(
  deps: UseCaseDeps,
  segment: MemberSegment,
  customFilter: SegmentFilter | null = null,
): Member[] {
  const now = msOf(deps.clock.now());
  const active = deps.members.all().filter((m) => m.status === 'ACTIVE');
  const rules = deps.rules.defaults();
  switch (segment) {
    case 'ALL_ACTIVE':
      return active;
    case 'LOW_BALANCE':
      return active.filter((m) => balanceOf(deps, m.id) < rules.lowBalanceThreshold);
    case 'EXPIRING_CREDITS':
      return active.filter((m) => expiringCreditsFor(deps, m.id) > 0);
    case 'NEW_MEMBERS':
      return active.filter((m) => now - msOf(m.createdAt) <= 14 * DAY_MS);
    case 'NO_VISIT_14D':
      return active.filter((m) => {
        const last = deps.accessLogs.lastAllowedAt(m.id);
        return last === null || now - msOf(last) > 14 * DAY_MS;
      });
    case 'CUSTOM': {
      const f = customFilter;
      if (!f) return [];
      return active.filter((m) => {
        if (f.branchId && m.preferredBranchId !== f.branchId) return false;
        if (f.maxBalance !== null && balanceOf(deps, m.id) > f.maxBalance) return false;
        if (f.minDaysSinceLastVisit !== null) {
          const last = deps.accessLogs.lastAllowedAt(m.id);
          if (last !== null && now - msOf(last) < f.minDaysSinceLastVisit * DAY_MS) return false;
        }
        if (f.joinedWithinDays !== null && now - msOf(m.createdAt) > f.joinedWithinDays * DAY_MS)
          return false;
        return true;
      });
    }
  }
}

export function sendCampaign(deps: UseCaseDeps, campaignId: string): Result<Campaign, AppError> {
  const campaign = deps.campaigns.byId(campaignId);
  if (!campaign) return err(appError('NOT_FOUND', 'Campaign not found.', 404));
  if (!canTransition(CAMPAIGN_TRANSITIONS, campaign.status, 'PROCESSING')) {
    return err(appError('INVALID_TRANSITION', `A ${campaign.status} campaign cannot be sent.`));
  }
  campaign.status = 'PROCESSING';
  const audience = segmentMembers(deps, campaign.segment, campaign.customFilter);
  for (const member of audience) {
    notify(deps, member.id, 'ANNOUNCEMENT', campaign.name, campaign.message);
  }
  campaign.status = 'SENT';
  campaign.sentCount = audience.length;
  deps.campaigns.save(campaign);
  return ok(campaign);
}
