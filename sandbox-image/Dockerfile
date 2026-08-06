FROM debian:trixie-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      bash \
      ca-certificates \
      coreutils \
      curl \
      fd-find \
      git \
      ripgrep \
 && rm -rf /var/lib/apt/lists/* \
 && ln -s /usr/bin/fdfind /usr/local/bin/fd

# Fixed UID so host-side mount ownership stays predictable across rebuilds.
RUN useradd --create-home --uid 10001 --shell /bin/bash agent \
 && mkdir -p /workspace \
 && chown agent:agent /workspace

USER agent
WORKDIR /workspace

CMD ["sleep", "infinity"]
