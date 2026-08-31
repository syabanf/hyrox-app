import type { EmergencyContact, Member, MemberStatus, Result } from '@hyrox/domain';
import { MEMBER_TRANSITIONS, err, ok, transition } from '@hyrox/domain';
import type { Actor, AppError } from '../common';
import { appError, recordAudit } from '../common';
import type { UseCaseDeps } from '../ports';

export interface RegisterMemberInput {
  fullName: string;
  email: string;
  phone: string;
  dateOfBirth: string | null;
  gender: 'MALE' | 'FEMALE' | 'OTHER' | null;
  emergencyContact: EmergencyContact | null;
  preferredBranchId: string | null;
}

export function registerMember(
  deps: UseCaseDeps,
  input: RegisterMemberInput,
): Result<Member, AppError> {
  const existing =
    deps.members.byIdentifier(input.email) ?? deps.members.byIdentifier(input.phone);
  if (existing) {
    return err(appError('IDENTIFIER_TAKEN', 'A member with this email or phone already exists.', 409));
  }
  const now = deps.clock.now();
  const member: Member = {
    id: deps.ids.next('mem'),
    fullName: input.fullName,
    email: input.email,
    phone: input.phone,
    dateOfBirth: input.dateOfBirth,
    gender: input.gender,
    emergencyContact: input.emergencyContact,
    preferredBranchId: input.preferredBranchId,
    avatarUrl: null,
    status: 'ACTIVE',
    waiverVersion: 'v1.0',
    waiverAcceptedAt: now,
    notes: null,
    createdAt: now,
    updatedAt: now,
  };
  deps.members.save(member);
  return ok(member);
}

export function updateProfile(
  deps: UseCaseDeps,
  memberId: string,
  patch: Partial<
    Pick<
      Member,
      'fullName' | 'email' | 'phone' | 'emergencyContact' | 'preferredBranchId' | 'avatarUrl'
    >
  >,
): Result<Member, AppError> {
  const member = deps.members.byId(memberId);
  if (!member) return err(appError('NOT_FOUND', 'Member not found.', 404));
  Object.assign(member, patch, { updatedAt: deps.clock.now() });
  deps.members.save(member);
  return ok(member);
}

export function setMemberStatus(
  deps: UseCaseDeps,
  args: { memberId: string; status: MemberStatus; actor: Actor; reason?: string | null },
): Result<Member, AppError> {
  const member = deps.members.byId(args.memberId);
  if (!member) return err(appError('NOT_FOUND', 'Member not found.', 404));
  if (member.status === args.status) return ok(member);
  const res = transition(MEMBER_TRANSITIONS, member.status, args.status);
  if (!res.ok) {
    return err(
      appError('INVALID_TRANSITION', `Cannot move member from ${member.status} to ${args.status}.`),
    );
  }
  const previous = member.status;
  member.status = res.value;
  member.updatedAt = deps.clock.now();
  deps.members.save(member);
  recordAudit(deps, {
    entityType: 'MEMBER',
    entityId: member.id,
    action: 'STATUS_CHANGE',
    previousValue: previous,
    newValue: member.status,
    actor: args.actor,
    reason: args.reason,
  });
  return ok(member);
}
