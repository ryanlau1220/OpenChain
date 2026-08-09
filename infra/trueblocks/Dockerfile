FROM golang:1.25.1-bookworm AS build

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY timestamps-download.patch /tmp/timestamps-download.patch
RUN git clone --depth 1 --branch v6.5.0 --recurse-submodules https://github.com/TrueBlocks/trueblocks-core.git . \
    && test "$(git rev-parse HEAD)" = "34840abebaf7a3854bf68fde5c8b7b14a4c1aeeb" \
    && git apply /tmp/timestamps-download.patch \
    && cd chifra \
    && go build -trimpath -ldflags='-s -w' -o /out/chifra .

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl gettext-base \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /out/chifra /usr/local/bin/chifra
COPY trueBlocks.toml.template /opt/openchain/trueBlocks.toml.template
COPY entrypoint.sh /usr/local/bin/openchain-trueblocks

RUN chmod 0755 /usr/local/bin/openchain-trueblocks

ENV XDG_CONFIG_HOME=/var/lib/trueblocks/config

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/openchain-trueblocks"]
