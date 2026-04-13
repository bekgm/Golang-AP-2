FROM golang:1.22-alpine AS builder

WORKDIR /generated
COPY generated/go.mod generated/go.sum ./
COPY generated/ .

WORKDIR /app
COPY order-service/go.mod order-service/go.sum ./
RUN go mod download
COPY order-service/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o order-service ./cmd/order-service

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/order-service .
EXPOSE 8080
EXPOSE 9090
CMD ["./order-service"]
