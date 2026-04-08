# Stage 1: Build
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY user-service/ /user-service/
COPY gateway/go.mod gateway/go.sum ./gateway/

COPY gateway/ ./gateway/
WORKDIR /app/gateway
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/gateway-service .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/gateway-service .

EXPOSE 8080
CMD ["./gateway-service"]