FROM golang:1.27.1-alpine AS build
ARG SERVICE
WORKDIR /src
COPY go.mod go.sum* ./
RUN if [ -f go.sum ]; then go mod download; fi
COPY cmd ./cmd
COPY internal ./internal
RUN test -n "$SERVICE" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/service ./cmd/${SERVICE}

FROM scratch
COPY --from=build /out/service /service
USER 65532:65532
ENTRYPOINT ["/service"]
