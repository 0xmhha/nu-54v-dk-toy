# opsd image. Build context: repository root (go.work spans packages and products).
FROM golang:1.26.11-bookworm@sha256:dc40a13b61304647ed7e5d581d246b86413dfc29d63c6d018310feb93cd98fc0 AS build
ENV GOTOOLCHAIN=local
WORKDIR /src
COPY go.work go.work.sum ./
COPY products/p03-cloud-mpc-wallet/go.mod products/p03-cloud-mpc-wallet/go.sum products/p03-cloud-mpc-wallet/
COPY products/p08-market-services/go.mod products/p08-market-services/go.sum products/p08-market-services/
COPY products/p09-travel-ai/go.mod products/p09-travel-ai/go.sum products/p09-travel-ai/
COPY packages/contracts-abi/go/ packages/contracts-abi/go/
COPY packages/protocol/go/ packages/protocol/go/
COPY products/p05-operations-backoffice/ products/p05-operations-backoffice/
COPY products/p07-indexer/ products/p07-indexer/
RUN cd products/p05-operations-backoffice && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/opsd ./cmd/opsd

FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931
COPY --from=build /out/opsd /opsd
EXPOSE 8090
ENTRYPOINT ["/opsd"]
