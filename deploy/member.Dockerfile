# Member PWA (Vite, served at the domain root) — build from the repo root:
#   docker build -f deploy/member.Dockerfile .
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

# Baked in at build time by Vite. Empty means "/api on my own origin", which
# is what the nginx front door serves; VITE_OFFLINE_DEMO=1 answers in-process
# from the bundled seed instead, with no server behind it.
ARG VITE_API_BASE_URL=""
ARG VITE_OFFLINE_DEMO=""
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL \
    VITE_OFFLINE_DEMO=$VITE_OFFLINE_DEMO
RUN pnpm --filter @nuhabit/member build

FROM nginx:alpine
COPY deploy/nginx/member.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/member/dist /usr/share/nginx/html

# The proxy waits for this rather than starting and serving 502s.
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=5 \
    CMD wget -qO- http://localhost/ >/dev/null || exit 1
