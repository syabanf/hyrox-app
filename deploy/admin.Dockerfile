# Admin panel (Next.js, served under /admin) — build from the repo root:
#   docker build -f deploy/admin.Dockerfile .
FROM node:22-alpine AS build
RUN corepack enable
WORKDIR /repo
COPY . .
RUN pnpm install --frozen-lockfile
RUN pnpm --filter @hyrox/admin build

FROM node:22-alpine
ENV NODE_ENV=production
WORKDIR /app
COPY --from=build /repo/apps/admin/.next/standalone ./
COPY --from=build /repo/apps/admin/.next/static ./apps/admin/.next/static
COPY --from=build /repo/apps/admin/public ./apps/admin/public
EXPOSE 3000
CMD ["node", "apps/admin/server.js"]
