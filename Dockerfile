FROM golang:1.26-alpine AS builder
#RUN apk update && apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/api ./internal/cmd/api

FROM alpine:latest
RUN apk update && apk add --no-cache ca-certificates tzdata
#RUN rm -rf /var/cache/apk/*

# (選用) 設定預設時區
#ENV TZ=Asia/Taipei

WORKDIR /app
COPY --from=builder /app/api .
COPY config.yaml ./config.yaml

ENTRYPOINT ["./api"]
