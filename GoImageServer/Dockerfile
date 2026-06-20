# GoImageServer —— 头像服务(本地存储；S3 需 -tags s3 另构建)
FROM golang:1.25-alpine AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct GOSUMDB=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -ldflags "-s -w" -o /out/avatarserver ./cmd/avatarserver

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=build /out/avatarserver /app/avatarserver
ENTRYPOINT ["/app/avatarserver"]
