# ── build stage ────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags="-s -w -X github.com/tgifai/friday.VERSION=${VERSION}" \
    -o /friday \
    ./cmd/friday

# ── runtime stage ─────────────────────────────────────────────────
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    # core
    ca-certificates tzdata locales \
    # shell & editor
    fish vim \
    # network
    curl wget mtr-tiny iputils-ping dnsutils net-tools iproute2 openssh-client \
    # dev
    git jq tree unzip zip tar gzip \
    # python
    python3 python3-pip python3-venv \
    # system
    htop procps lsof strace less file \
    && locale-gen en_US.UTF-8 \
    && rm -rf /var/lib/apt/lists/*

ENV LANG=en_US.UTF-8
ENV SHELL=/usr/bin/fish

COPY --from=builder /friday /usr/local/bin/friday

# friday stores all data under $HOME/.friday (/root/.friday)
VOLUME /root/.friday

EXPOSE 8088

ENTRYPOINT ["friday"]
CMD ["gateway", "run"]
