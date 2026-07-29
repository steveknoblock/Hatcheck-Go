# syntax=docker/dockerfile:1

# --- Build stage ---
#
# GO_VERSION must match (or exceed) the `go` directive in go.mod (1.25.0).
# go.mod originally landed on 1.25.0 as a side effect of commit 048ab8b
# (adding golang.org/x/time/rate via `go mod tidy`); that version has since
# been adopted as the project's floor rather than pinned back down. Bump
# this ARG in lockstep if go.mod's `go` directive is ever raised again —
# using a builder image older than go.mod's declared version will fail the
# build with "go.mod requires go >= X".
ARG GO_VERSION=1.25.0

FROM golang:${GO_VERSION}-alpine AS builder

# Needed to fetch modules and for `go mod download` over HTTPS.
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Cache dependency downloads separately from source changes.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# CGO is disabled since Hatcheck only uses stdlib crypto/md5, crypto/sha256,
# and crypto/hmac — no cgo dependency — so both binaries can be fully static
# and run in a minimal (non-libc) final image.
ENV CGO_ENABLED=0

# Build the HTTP server (server/main.go) and the CLI (cmd/hatcheck/main.go).
RUN go build -trimpath -ldflags="-s -w" -o /out/hatcheck-server ./server
RUN go build -trimpath -ldflags="-s -w" -o /out/hatcheck ./cmd/hatcheck

# --- Runtime stage ---
FROM alpine:3.20 AS runtime

# ca-certificates: needed for outbound TLS to the Stytch API.
# tzdata: capability Expires/Created timestamps are UTC already, but tzdata
# avoids surprises if that ever changes.
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S hatcheck && adduser -S hatcheck -G hatcheck

WORKDIR /app

COPY --from=builder /out/hatcheck-server /app/hatcheck-server
COPY --from=builder /out/hatcheck /usr/local/bin/hatcheck
COPY --from=builder /src/ui /app/ui

# Runtime data directories. HATCHECK_DATA and HATCHECK_META point here by
# default — mount volumes at these paths to persist the CAS and metadata
# log across container restarts. Do not skip volumes: the metadata log is
# the source of truth and losing it loses the capability/index state too.
RUN mkdir -p /app/objects /app/metadata && chown -R hatcheck:hatcheck /app

USER hatcheck

# Defaults match the env vars main.go already reads via os.Getenv, so the
# server runs out of the box; override at `docker run`/compose time.
# HATCHECK_SIGNING_KEY, STYTCH_PROJECT_ID, STYTCH_SECRET, and
# STYTCH_REDIRECT_URL have no safe defaults and MUST be supplied — the
# server calls log.Fatal on startup if HATCHECK_SIGNING_KEY is unset.
ENV HATCHECK_DATA=/app/objects \
    HATCHECK_META=/app/metadata \
    HATCHECK_UI=/app/ui

EXPOSE 8090

VOLUME ["/app/objects", "/app/metadata"]

ENTRYPOINT ["/app/hatcheck-server"]