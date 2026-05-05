FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o loklingo-backend .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget \
    && addgroup -S loklingo && adduser -S -G loklingo loklingo
WORKDIR /app
COPY --from=builder /app/loklingo-backend .
RUN mkdir -p /tmp/loklingo \
    && chmod 1777 /tmp/loklingo \
    && chown loklingo:loklingo /app/loklingo-backend
USER loklingo

EXPOSE 8080
CMD ["./loklingo-backend"]
