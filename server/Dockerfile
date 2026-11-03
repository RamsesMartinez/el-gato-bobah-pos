# syntax=docker/dockerfile:1
# PREBUILT (spec 044): ruta, dentro del contexto, de un binario ya compilado para linux/amd64. CI lo
# compila con setup-go, cuyo caché sí persiste entre corridas; el de los --mount=type=cache de abajo
# no viaja en el caché type=gha y cada merge recompilaba el módulo entero (79 s de 102 s). Sin el
# argumento (make deploy, compose), se compila aquí como siempre. Solo se construye la etapa que se
# usa: con PREBUILT la de compilar ni corre.
ARG PREBUILT=
# Imágenes base clavadas por digest (A08): el tag es mutable, el digest no. Dependabot
# (docker) las mantiene al día.
FROM golang:1.27-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS bin
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

FROM golang:1.27-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS bin-prebuilt
ARG PREBUILT
COPY ${PREBUILT} /out/api
RUN mkdir -p /out/logs && chown 65532:65532 /out/logs

FROM bin${PREBUILT:+-prebuilt} AS build

# distroless: ca-certs + tzdata + usuario nonroot, imagen ~24MB.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/api /api
COPY --from=build --chown=65532:65532 /out/logs /logs
EXPOSE 8080
ENTRYPOINT ["/api"]
