# syntax=docker/dockerfile:1
# Imágenes base clavadas por digest (A08): el tag es mutable, el digest no. Dependabot
# (docker) las mantiene al día.
FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS build
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
FROM gcr.io/distroless/static-debian12:nonroot@sha256:aef9602f8710ec12bde19d593fed1f76c708531bb7aba205110f1029786ead7b
COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
