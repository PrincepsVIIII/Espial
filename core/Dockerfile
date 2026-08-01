FROM golang:1.26@sha256:3aff6657219a4d9c14e27fb1d8976c49c29fddb70ba835014f477e1c70636647 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/espial ./cmd/espial \
    && CGO_ENABLED=0 go build -trimpath -o /out/espial-sample-adapter ./cmd/espial-sample-adapter \
    && CGO_ENABLED=0 go build -trimpath -o /out/espial-webcheck-adapter ./cmd/espial-webcheck-adapter

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/espial /espial
COPY --from=build /out/espial-sample-adapter /espial-sample-adapter
COPY --from=build /out/espial-webcheck-adapter /espial-webcheck-adapter
USER 65532:65532
ENTRYPOINT ["/espial"]
CMD ["serve"]
