# Build context is the repository root.
FROM golang:1.26.11-bookworm@sha256:dc40a13b61304647ed7e5d581d246b86413dfc29d63c6d018310feb93cd98fc0 AS build
ENV GOTOOLCHAIN=local GOWORK=off
WORKDIR /src
COPY products/p09-travel-ai/go.mod products/p09-travel-ai/go.sum ./
RUN go mod download
COPY products/p09-travel-ai/cmd/ cmd/
COPY products/p09-travel-ai/internal/ internal/
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -buildvcs=false -o /out/placesd ./cmd/placesd

FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931
COPY --from=build /out/placesd /placesd
EXPOSE 8098
ENTRYPOINT ["/placesd", "-listen", ":8098"]
