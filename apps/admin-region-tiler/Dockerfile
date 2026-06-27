# 多阶段构建
FROM golang:1.25-alpine AS builder

# 设置工作目录
WORKDIR /app

# 复制go.mod和go.sum文件
COPY go.mod go.sum ./

RUN go mod download

# 复制所有代码文件
COPY . .

RUN go build -o tiler .

FROM alpine:latest

# 设置工作目录
WORKDIR /app

COPY --from=builder /app/tiler .
COPY --from=builder /app/static ./static
COPY --from=builder /app/geojson ./geojson
COPY --from=builder /app/conf.toml .

RUN mkdir -p /app/data /app/output

EXPOSE 8081

CMD ["./tiler"]
