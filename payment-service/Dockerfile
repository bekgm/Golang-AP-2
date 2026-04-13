FROM golang:1.22-alpine AS builder

WORKDIR /generated
COPY generated/go.mod generated/go.sum ./
COPY generated/ .

# Build the payment service
WORKDIR /app
COPY payment-service/go.mod payment-service/go.sum ./
RUN go mod download
COPY payment-service/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o payment-service ./cmd/payment-service

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/payment-service .
EXPOSE 8081
EXPOSE 9091
CMD ["./payment-service"]
