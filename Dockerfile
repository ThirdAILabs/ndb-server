FROM golang:1.24.4 AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y gcc g++

RUN go install github.com/google/go-licenses@latest
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o ndb-server ./cmd/main.go

RUN go-licenses save ./cmd/ --save_path=third_party_licenses || :

FROM golang:1.24.4

WORKDIR /app
COPY --from=builder /app/ndb-server .
COPY --from=builder /app/LICENSE .
COPY --from=builder /app/third_party_licenses third_party_licenses
COPY --from=builder /app/internal/ndb/lib/THIRD_PARTY_NOTICES.txt .
EXPOSE 80

ENTRYPOINT ["/app/ndb-server"]