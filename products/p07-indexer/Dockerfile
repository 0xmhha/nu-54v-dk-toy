# Build context: repository root (go.work spans packages and products).
FROM golang:1.25 AS build
WORKDIR /src
COPY . .
RUN cd products/p07-indexer && CGO_ENABLED=0 go build -o /out/indexer ./cmd/indexer

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/indexer /indexer
EXPOSE 8080
ENTRYPOINT ["/indexer"]
