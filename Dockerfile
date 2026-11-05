# Build stage: go-sqlite3 needs cgo, so build with a C toolchain.
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /out/datatrader .

# Runtime stage
FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/datatrader /app/datatrader
# style.css is served from disk by main.go
COPY public /app/public
ENV DB_PATH=/data/database.db
CMD ["/app/datatrader"]
