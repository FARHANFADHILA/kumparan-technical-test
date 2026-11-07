# Tahap 1: build, pakai image Go Alpine yang ringan
FROM golang:1.27.3-alpine AS builder

WORKDIR /app

# Copy dulu go.mod/go.sum supaya cache download dependency tidak terulang
COPY go.mod go.sum ./
RUN go mod download

# Baru copy semua source
COPY . .

# Build binary statis, tanpa CGO, supaya bisa jalan di scratch
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/server .

# Tahap 2: image final, scratch itu image kosong, paling ringan
FROM scratch

# Cuma binary-nya saja yang dibawa
COPY --from=builder /app/server /server

EXPOSE 8080
ENV PORT=8080

ENTRYPOINT ["/server"]
