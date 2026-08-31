# Member PWA (Vite, served at the domain root) — build from the repo root:
#   docker build -f deploy/member.Dockerfile .
FROM node:22-alpine AS build
RUN corepack enable
WORKDIR /repo
COPY . .
RUN pnpm install --frozen-lockfile
RUN pnpm --filter @hyrox/member build

FROM nginx:alpine
COPY deploy/nginx/member.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/member/dist /usr/share/nginx/html
