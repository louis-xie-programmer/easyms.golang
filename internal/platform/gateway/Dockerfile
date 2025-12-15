FROM golang:1.25.2 AS builder
WORKDIR /src
COPY . .
# Build -s 去除符号表 -w 去除debug调试信息，减少二进制文件大小
RUN apt-get update && apt-get install -y git ca-certificates && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/service ./internal/platform/gateway/main/main.go

FROM scratch
#COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /bin/service /bin/service
EXPOSE 10000
ENTRYPOINT ["/bin/service"]