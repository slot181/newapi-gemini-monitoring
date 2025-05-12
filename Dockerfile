# 使用Go官方镜像作为构建环境
FROM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /app

# 复制go.mod和go.sum文件（如果存在）
COPY go.mod go.sum* ./

# 复制源代码
COPY . .

# 下载依赖并生成go.sum
RUN go mod tidy

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -o gemini-monitor .

# 使用轻量级的alpine镜像作为运行环境
FROM alpine:latest

# 安装ca-certificates，用于HTTPS请求
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# 从构建阶段复制编译好的二进制文件
COPY --from=builder /app/gemini-monitor .

# 暴露端口
EXPOSE 8080

# 设置容器启动命令
CMD ["./gemini-monitor"]
