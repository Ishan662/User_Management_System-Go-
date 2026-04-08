# Stage 1: Build (The "Heavy" part)
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY user-service/go.mod user-service/go.sum ./
RUN go mod download

COPY user-service/ .

RUN CGO_ENABLED=0 GOOS=linux go build -o /user-service-bin main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /user-service-bin .

CMD ["./user-service-bin"]