FROM golang:1.24.4 AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y gcc g++

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o ndb-server ./cmd/main.go

FROM golang:1.24.4

WORKDIR /app
COPY --from=builder /app/ndb-server .
COPY --from=builder /app/third_party_licenses .
EXPOSE 80

ENTRYPOINT ["/app/ndb-server"]