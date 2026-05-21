FROM golang:1.24.3-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/create-admin ./cmd/create-admin

FROM alpine:3.20
RUN adduser -D -g '' app
WORKDIR /app
COPY --from=builder /out/server /app/server
COPY --from=builder /out/migrate /app/migrate
COPY --from=builder /out/create-admin /app/create-admin
COPY migrations /app/migrations
RUN mkdir -p /data/storage && chown -R app:app /data /app
USER app
EXPOSE 8080
ENTRYPOINT ["/app/server"]