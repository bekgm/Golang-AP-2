FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY notification-service/go.mod notification-service/go.sum ./
RUN go mod download
COPY notification-service/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o notification-service ./cmd/notification-service

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/notification-service .
CMD ["./notification-service"]
