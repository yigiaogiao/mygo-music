# Builds the app in two halves: the native server and the browser UI
# compiled to WebAssembly, which the server serves.
FROM golang:1.27-alpine AS build

WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN go build -trimpath -ldflags='-s -w' -o /out/music-server ./cmd/server
RUN GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o /out/web/app.wasm .
RUN cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" /out/web/wasm_exec.js \
 && gzip -9 -c /out/web/app.wasm > /out/web/app.wasm.gz \
 && cp web/index.html /out/web/index.html

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/music-server /app/music-server
COPY --from=build /out/web /app/web

# /lzcapp/var survives restarts; the netdisk is mounted read-only by the
# LazyCat runtime when the manifest asks for it.
ENV ADDR=:8090 \
    DATA_DIR=/lzcapp/var/data \
    MUSIC_DIR=/lzcapp/media/RemoteFS \
    WEB_DIR=/app/web \
    ONLINE_LYRICS=true

EXPOSE 8090
ENTRYPOINT ["/app/music-server"]
