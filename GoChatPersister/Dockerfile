# GoChatPersister —— 聊天消息异步落库消费者(RabbitMQ → MySQL)
FROM golang:1.25-alpine AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct GOSUMDB=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -ldflags "-s -w" -o /out/persister ./cmd/persister

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=build /out/persister /app/persister
ENTRYPOINT ["/app/persister"]
