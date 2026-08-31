import type { AdminUser, Member, Permission } from '@hyrox/domain';
import { hasPermission } from '@hyrox/domain';
import { HttpResponse } from 'msw';
import type { z } from 'zod';
import type { MockDb } from '../db';

export const jsonError = (status: number, code: string, message: string) =>
  HttpResponse.json({ error: { code, message } }, { status });

export async function parseBody<S extends z.ZodType>(
  request: Request,
  schema: S,
): Promise<{ ok: true; data: z.infer<S> } | { ok: false; response: Response }> {
  let raw: unknown;
  try {
    raw = await request.json();
  } catch {
    return { ok: false, response: jsonError(400, 'INVALID_JSON', 'Body must be JSON.') };
  }
  const parsed = schema.safeParse(raw);
  if (!parsed.success) {
    const first = parsed.error.issues[0];
    return {
      ok: false,
      response: jsonError(
        400,
        'VALIDATION_ERROR',
        first ? `${first.path.join('.') || 'body'}: ${first.message}` : 'Invalid request body.',
      ),
    };
  }
  return { ok: true, data: parsed.data };
}

const bearerToken = (request: Request): string | null => {
  const header = request.headers.get('authorization');
  if (!header?.startsWith('Bearer ')) return null;
  return header.slice('Bearer '.length);
};

export function memberFromRequest(db: MockDb, request: Request): Member | null {
  const token = bearerToken(request);
  if (!token?.startsWith('member:')) return null;
  const id = token.slice('member:'.length);
  return db.members.find((m) => m.id === id) ?? null;
}

export function adminFromRequest(db: MockDb, request: Request): AdminUser | null {
  const token = bearerToken(request);
  if (!token?.startsWith('admin:')) return null;
  const id = token.slice('admin:'.length);
  return db.adminUsers.find((u) => u.id === id) ?? null;
}

export type AuthResult<T> = { ok: true; value: T } | { ok: false; response: Response };

export function requireMember(db: MockDb, request: Request): AuthResult<Member> {
  const member = memberFromRequest(db, request);
  if (!member) return { ok: false, response: jsonError(401, 'UNAUTHORIZED', 'Sign in first.') };
  return { ok: true, value: member };
}

/**
 * RBAC is enforced here — on the "server" side — so hiding a button in the
 * admin UI is never the only thing standing between a role and an action.
 */
export function requireAdmin(
  db: MockDb,
  request: Request,
  permission?: Permission,
): AuthResult<AdminUser> {
  const user = adminFromRequest(db, request);
  if (!user) return { ok: false, response: jsonError(401, 'UNAUTHORIZED', 'Sign in first.') };
  if (permission && !hasPermission(user.role, permission)) {
    return {
      ok: false,
      response: jsonError(403, 'FORBIDDEN', `Your role (${user.role}) lacks ${permission}.`),
    };
  }
  return { ok: true, value: user };
}
