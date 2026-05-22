FROM golang:1.24.3-alpine AS backend-builder
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/create-admin ./cmd/create-admin
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/preview-worker ./cmd/preview-worker

FROM node:22-alpine AS frontend-deps
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN if [ -f package-lock.json ]; then npm ci; else npm install; fi

FROM node:22-alpine AS frontend-builder
WORKDIR /src/frontend
COPY --from=frontend-deps /src/frontend/node_modules ./node_modules
COPY frontend/ ./
ENV NEXT_TELEMETRY_DISABLED=1
RUN mkdir -p public && npm run build

FROM node:22-alpine AS runtime
RUN apk add --no-cache \
    libreoffice \
    ffmpeg \
    ttf-dejavu \
    font-noto
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_TELEMETRY_DISABLED=1
ENV PORT=3000
ENV HOSTNAME=0.0.0.0
ENV BACKEND_HTTP_ADDR=:8080

COPY --from=backend-builder /out/server /app/bin/server
COPY --from=backend-builder /out/migrate /app/bin/migrate
COPY --from=backend-builder /out/create-admin /app/bin/create-admin
COPY --from=backend-builder /out/preview-worker /app/bin/preview-worker
COPY --from=backend-builder /src/backend/migrations /app/migrations

COPY --from=frontend-builder /src/frontend/.next/standalone /app/frontend
COPY --from=frontend-builder /src/frontend/.next/static /app/frontend/.next/static
COPY --from=frontend-builder /src/frontend/public /app/frontend/public

COPY scripts/container/start-app.sh /app/bin/start-app.sh
RUN chmod +x /app/bin/start-app.sh && mkdir -p /data/storage && chown -R app:app /app /data

USER app
EXPOSE 3000
ENTRYPOINT ["/app/bin/start-app.sh"]
