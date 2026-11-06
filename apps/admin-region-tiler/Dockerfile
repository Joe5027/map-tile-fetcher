FROM node:24.22.2-alpine AS assets
WORKDIR /assets
COPY package.json package-lock.json ./
COPY scripts/prepare_assets.mjs scripts/prepare_assets.mjs
RUN npm ci --omit=dev --no-audit --no-fund

FROM golang:1.27.3-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

ARG VERSION=unknown
ARG COMMIT=unknown
ARG BUILT_AT=unknown
ARG DIRTY=unknown
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "-buildid= -X main.buildVersion=${VERSION} -X main.buildCommit=${COMMIT} -X main.buildTime=${BUILT_AT} -X main.buildDirty=${DIRTY}" -o tiler .

FROM alpine:3.24.4

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/tiler .
COPY --from=builder /app/static ./static
COPY --from=assets /assets/static/vendor/cesium ./static/vendor/cesium
COPY --from=builder /app/geojson ./geojson
COPY --from=builder /app/conf.toml .

RUN mkdir -p /app/data /app/output

EXPOSE 8081

CMD ["./tiler"]
