# opsd image. Build context: repository root (go.work spans packages and products).
FROM golang:1.25 AS build
WORKDIR /src
COPY . .
RUN cd products/p05-operations-backoffice && CGO_ENABLED=0 go build -o /out/opsd ./cmd/opsd

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/opsd /opsd
EXPOSE 8090
ENTRYPOINT ["/opsd"]
