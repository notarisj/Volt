FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.* ./
RUN go mod download

COPY . ./

ARG VERSION=dev
RUN go build -ldflags="-X volt/internal/version.Version=${VERSION}" -o volt main.go

FROM alpine:3.21

# su-exec: lightweight privilege-drop tool (replaces gosu, used by official Docker images)
RUN apk --no-cache add ca-certificates tzdata su-exec

# Create a dedicated non-root user; the entrypoint script drops to this user at runtime
RUN addgroup -S volt && adduser -S volt -G volt

WORKDIR /app

COPY --from=builder /app/volt .
COPY --from=builder /app/views ./views
COPY --from=builder /app/static ./static
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN sed -i 's/\r//' /usr/local/bin/docker-entrypoint.sh && chmod +x /usr/local/bin/docker-entrypoint.sh

# Pre-create volume directories; chown happens at runtime in the entrypoint
RUN mkdir -p /app/uploads /app/data

VOLUME ["/app/data", "/app/uploads"]

ENV PORT=8686
ENV DB_PATH=/app/data/volt.db

# Start as root so the entrypoint can fix bind-mount ownership, then drop to volt
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["./volt"]
