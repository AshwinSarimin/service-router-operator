FROM golang:1.20 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o service-router-operator ./cmd/main.go

FROM alpine:latest

WORKDIR /root/

COPY --from=builder /app/service-router-operator .

CMD ["./service-router-operator"]