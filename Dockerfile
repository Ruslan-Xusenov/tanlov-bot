FROM golang:1.21-alpine AS builder

WORKDIR /app

# Keshlashni optimallashtirish uchun avval go.mod va go.sum o'qiladi
COPY go.mod go.sum ./
RUN go mod download

# Qolgan kodlarni nusxalash
COPY . .

# Dasturni yig'ish (build)
RUN CGO_ENABLED=0 GOOS=linux go build -o bot_app main.go

# Kichik hajm uchun minimal alpine image
FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Qurilgan dasturni nusxalash
COPY --from=builder /app/bot_app .

# Dasturni ishga tushirish
CMD ["./bot_app"]
