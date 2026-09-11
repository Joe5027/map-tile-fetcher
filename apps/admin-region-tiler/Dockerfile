FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN apk add --no-cache build-base
RUN go mod download

COPY . .

ARG VERSION=unknown
ARG COMMIT=unknown
ARG BUILT_AT=unknown
ARG DIRTY=unknown
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "-buildid= -X main.buildVersion=${VERSION} -X main.buildCommit=${COMMIT} -X main.buildTime=${BUILT_AT} -X main.buildDirty=${DIRTY}" -o tiler .

FROM alpine:latest

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/tiler .
COPY --from=builder /app/static ./static
COPY --from=builder /app/geojson ./geojson
COPY --from=builder /app/conf.toml .

RUN mkdir -p /app/data /app/output

EXPOSE 8081

CMD ["./tiler"]
