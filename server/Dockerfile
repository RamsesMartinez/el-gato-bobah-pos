# syntax=docker/dockerfile:1
# Imágenes base clavadas por digest (A08): el tag es mutable, el digest no. Dependabot
# (docker) las mantiene al día.
FROM golang:1.27-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build
WORKDIR /src
# Descarga de módulos cacheada: solo se rehace si cambian go.mod/go.sum.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
# Versión del build inyectada por ldflags (la pasan make build / docker-compose). Default "dev".
ARG GIT_SHA=dev
ARG BUILT_AT=
# Cache de módulos + cache de compilación → rebuilds en segundos.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags="-s -w -X main.version=${GIT_SHA} -X main.builtAt=${BUILT_AT}" \
      -o /out/api ./cmd/api
# /logs con dueño 65532 (UID/GID de "nonroot" en distroless, ver abajo): un volumen NUEVO
# monta ahí heredando el contenido/permisos de la imagen la primera vez. Sin esto, Docker crea
# el punto de montaje como root:root y el proceso nonroot nunca puede escribir su log de archivo
# (slog traga ese error en silencio — logging.Setup queda "funcionando" solo por stdout).
RUN mkdir -p /out/logs && chown 65532:65532 /out/logs

# distroless: ca-certs + tzdata + usuario nonroot, imagen ~24MB.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/api /api
COPY --from=build --chown=65532:65532 /out/logs /logs
EXPOSE 8080
ENTRYPOINT ["/api"]
