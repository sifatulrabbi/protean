FROM debian:bookworm-slim

ARG NODE_VERSION=20.19.0
ARG BUN_VERSION=1.2.15

RUN apt-get update && \
	apt-get install -y --no-install-recommends \
	bash \
	ca-certificates \
	coreutils \
	curl \
	git \
	python3 \
	ripgrep \
	unzip \
	libreoffice \
	xz-utils && \
	rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz" \
	-o /tmp/node.tar.xz && \
	tar -xJf /tmp/node.tar.xz -C /usr/local --strip-components=1 && \
	rm /tmp/node.tar.xz

RUN curl -fsSL "https://github.com/oven-sh/bun/releases/download/bun-v${BUN_VERSION}/bun-linux-x64.zip" \
	-o /tmp/bun.zip && \
	unzip /tmp/bun.zip -d /tmp && \
	mv /tmp/bun-linux-x64/bun /usr/local/bin/bun && \
	rm -rf /tmp/bun.zip /tmp/bun-linux-x64

RUN groupadd -g 1000 sandbox && \
	useradd -m -u 1000 -g 1000 -s /bin/bash sandbox

USER sandbox
WORKDIR /workspace

CMD ["sleep", "infinity"]
