FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.work go.work.sum ./
COPY gateway/go.mod gateway/go.sum ./gateway/
COPY user-service/go.mod user-service/go.sum ./user-service/

RUN cd gateway && go mod download

COPY gateway/ ./gateway/
COPY user-service/ ./user-service/

RUN cd gateway && go build -o /gateway-bin main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /gateway-bin .

CMD ["./gateway-bin"]