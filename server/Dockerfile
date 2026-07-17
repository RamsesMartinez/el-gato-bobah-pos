# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src
# Descarga de módulos cacheada: solo se rehace si cambian go.mod/go.sum.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# Cache de módulos + cache de compilación → rebuilds en segundos.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# distroless: ca-certs + tzdata + usuario nonroot, imagen ~24MB.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
