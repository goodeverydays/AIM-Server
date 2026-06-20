# GoMediaServer —— 语音/视频媒体服务(HTTP 直传 + 静态服务)
FROM golang:1.25-alpine AS build
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct GOSUMDB=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -ldflags "-s -w" -o /out/mediaserver ./cmd/mediaserver

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=build /out/mediaserver /app/mediaserver
ENTRYPOINT ["/app/mediaserver"]
