# ─── Stage 1: Build ───────────────────────────────────────────────────────────
# Compila el binario Go en una imagen con el toolchain completo
FROM golang:1.25-alpine AS builder

WORKDIR /app

ENV GOTOOLCHAIN=auto

# Copiar módulos primero para aprovechar cache de capas Docker
COPY go.mod go.sum ./
RUN go mod download

# Copiar el resto del código fuente
COPY . .

# Compilar binario estático (sin CGO) para imagen mínima
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server main.go

# ─── Stage 2: Run ─────────────────────────────────────────────────────────────
# Imagen final mínima (~15MB) solo con el binario
FROM alpine:3.20

WORKDIR /app

# Certificados CA para HTTPS/SMTP + zona horaria
RUN apk --no-cache add ca-certificates tzdata

# Copiar solo el binario compilado del stage anterior
COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]
