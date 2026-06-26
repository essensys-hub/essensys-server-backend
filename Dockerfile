# ============================================
# Stage 1: Build Go binaries
# ============================================
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
COPY migrations/ ./migrations/

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

# Build server binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o /server ./cmd/server/

# Build mcp-server binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${COMMIT}" \
    -o /mcp-server ./cmd/mcp-server/

# ============================================
# Stage 2: Runtime (shared base image)
# ============================================
FROM essensyshub/essensys-base:raspberry.2026.02

COPY --from=builder /server /usr/local/bin/server
COPY --from=builder /mcp-server /usr/local/bin/mcp-server
COPY migrations/ /opt/essensys/migrations/
COPY config.yaml.example /opt/essensys/config.yaml.example

# Run as non-root for least privilege (Trivy DS-0002, SCRUM-7).
# Ports are non-privileged (7070/8083) so no extra capabilities are required.
RUN addgroup -S app && adduser -S -G app app \
    && chown -R app:app /data /opt/essensys

EXPOSE 7070 8083

VOLUME ["/data"]

USER app

ENTRYPOINT ["server"]
CMD ["-config", "/etc/essensys/config.yaml"]
