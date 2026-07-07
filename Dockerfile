# Multi-Arch-Image (linux/amd64, linux/arm64) — gebaut via docker buildx.
# Laufzeitdaten liegen unter /data (Volume); GPIO braucht --device /dev/gpiochip0.

FROM node:22-alpine AS webbuild
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

FROM golang:1.25-alpine AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=webbuild /src/web/dist ./web/dist
ARG VERSION=docker
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags "-X main.version=${VERSION}" -o /sprinklerd ./cmd/sprinklerd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=gobuild /sprinklerd /sprinklerd
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["/sprinklerd", "-config", "/data/config.json", "-db", "/data/zonelog.db"]
