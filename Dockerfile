# Build stage. Dependencies are downloaded in their own layer so a source-only
# change does not re-resolve the module graph.
FROM golang:1.26-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO off gives a static binary; trimpath and -s -w keep the image small and
# the binary free of local filesystem paths.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

# Runtime stage: scratch, because a static Go binary needs nothing else. The
# certificate bundle is for outbound TLS (the payment gateway), and the
# timezone database is what makes "Asia/Jakarta" resolvable.
FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/api /out/migrate /out/seed /usr/local/bin/

# An unprivileged, well-known uid: scratch has no /etc/passwd to name it in.
USER 65532:65532

ENV HTTP_ADDR=:8080 \
    APP_ENV=production \
    LOG_FORMAT=json \
    STUDIO_TIMEZONE=Asia/Jakarta

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/api"]
