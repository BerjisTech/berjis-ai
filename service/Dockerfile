FROM golang:1.22-alpine as build
WORKDIR /app
COPY ai/service/go.mod ./go.mod
COPY ai/service/go.sum ./go.sum
COPY shared/coreauth /shared/coreauth
RUN go env -w GOPROXY=https://proxy.golang.org,direct && go mod download -x || true
COPY ai/service/ ./
RUN go mod tidy && go mod download -x || true
RUN mkdir -p /out \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/berjis-ai ./cmd/service

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /out/berjis-ai /berjis-ai
USER 65532:65532
ENV PORT=8094
EXPOSE 8094
ENTRYPOINT ["/berjis-ai"]
