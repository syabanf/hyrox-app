# Admin panel (Next.js, served under /admin) — build from the repo root:
#   docker build -f deploy/admin.Dockerfile .
FROM node:22-alpine AS build
RUN corepack enable
WORKDIR /repo

# The manifests first: a source-only change then reuses the install layer
# instead of re-resolving every dependency in the workspace.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/member/package.json apps/member/
COPY apps/admin/package.json apps/admin/
COPY apps/backend/package.json apps/backend/
COPY packages/domain/package.json packages/domain/
COPY packages/application/package.json packages/application/
COPY packages/contracts/package.json packages/contracts/
COPY packages/api-client/package.json packages/api-client/
COPY packages/mock-api/package.json packages/mock-api/
COPY packages/ui/package.json packages/ui/
COPY packages/tsconfig/package.json packages/tsconfig/
RUN pnpm install --frozen-lockfile

COPY . .

# Baked in at build time by Next. Empty means "/api on my own origin", which
# is what the nginx front door serves; NEXT_PUBLIC_OFFLINE_DEMO=1 answers
# in-process from the bundled seed instead.
ARG NEXT_PUBLIC_API_BASE_URL=""
ARG NEXT_PUBLIC_OFFLINE_DEMO=""
ENV NEXT_PUBLIC_API_BASE_URL=$NEXT_PUBLIC_API_BASE_URL \
    NEXT_PUBLIC_OFFLINE_DEMO=$NEXT_PUBLIC_OFFLINE_DEMO
RUN pnpm --filter @nuhabit/admin build

FROM node:22-alpine
ENV NODE_ENV=production
WORKDIR /app
COPY --from=build /repo/apps/admin/.next/standalone ./
COPY --from=build /repo/apps/admin/.next/static ./apps/admin/.next/static
COPY --from=build /repo/apps/admin/public ./apps/admin/public

# Next's standalone server runs as root by default; it needs nothing that
# root gives it.
USER node

EXPOSE 3000

# The panel lives under /admin, so that is where it answers.
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=6 \
    CMD wget -qO- http://localhost:3000/admin/login >/dev/null || exit 1

CMD ["node", "apps/admin/server.js"]
