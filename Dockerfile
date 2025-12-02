     # 第一阶段：编译 epusdt 二进制
     FROM golang:1.20 AS builder

     WORKDIR /app
     COPY . .
     WORKDIR /app/src

     RUN go mod download
     RUN CGO_ENABLED=0 GOOS=linux go build -o /app/epusdt main.go

     # 第二阶段：运行容器
     FROM debian:bookworm-slim

     WORKDIR /app
     COPY --from=builder /app/epusdt /app/epusdt
     COPY src/static /app/static

     ENV TZ=Asia/Shanghai
     EXPOSE 8000

     CMD ["/app/epusdt", "http", "start"]
