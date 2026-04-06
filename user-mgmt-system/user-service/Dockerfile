FROM golang:1.23-alphine AS builder

WORKDIR /app

COPY go.work go.work.sum ./
COPY user-service/go.mod user-service/go.sum ./user-service/

RUN cd user-service && go mod download

COPY user-service/ ./user-service/

RUN cd user-service && go build -o /user-service-bin main.go

FROM alphine:latest
WORKDIR /root/
COPY --from=builder /user-service-bin .

CMD ["./user-service-bin"]