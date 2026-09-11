# hsdebug — multi-stage build producing a small static binary plus opencode.

# --- build stage ---
FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=docker" -o /out/hsdebug ./cmd/hsdebug

# --- runtime stage ---
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates curl bash \
 && rm -rf /var/lib/apt/lists/*

# Install the opencode agent that hsdebug drives.
RUN curl -fsSL https://opencode.ai/install | bash \
 && ln -sf /root/.opencode/bin/opencode /usr/local/bin/opencode || true
ENV PATH="/root/.opencode/bin:${PATH}"

COPY --from=build /out/hsdebug /usr/local/bin/hsdebug

# Config and secrets live under a mountable volume.
ENV HSDEBUG_CONFIG_DIR=/data
VOLUME ["/data"]

# UI/API binds to loopback by default; override host to expose on the network.
EXPOSE 7654
ENTRYPOINT ["hsdebug"]
CMD ["server", "--host", "0.0.0.0"]
