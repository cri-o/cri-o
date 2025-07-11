FROM quay.io/fedora/fedora-minimal:38
RUN microdnf install -y coreutils \
		gcc \
		gzip \
		hostname \
		iproute \
		iputils \
		make \
		nmap-ncat \
		openssl-devel \
		procps \
		shadow \
		tar \
		wget \
		&&  microdnf clean all \
		&&  rm -rf /var/cache/yum

# Needed to verify the `/etc` folder permission
RUN groupadd -g 5000 test
RUN useradd -m -d /home/test -s /bin/bash -g test -u 5000 test

RUN chown test:test /etc/ && \
    chgrp test /etc/ && \
    chmod 755 /etc/

# This image is largely based off https://github.com/docker-library/redis/blob/master/6.0/alpine/Dockerfile
# for redis-specific changes.
RUN groupadd -r -g 998 redis && useradd -g redis -u 998 redis
COPY docker-entrypoint.sh /usr/local/bin/
ARG SU_EXEC_VERSION=0.2
ARG SU_EXEC_URL="https://github.com/ncopa/su-exec/archive/v${SU_EXEC_VERSION}.tar.gz"
RUN curl -sL "${SU_EXEC_URL}" | tar -C /tmp -zxf - \
&& make -C "/tmp/su-exec-${SU_EXEC_VERSION}" \
&& cp "/tmp/su-exec-${SU_EXEC_VERSION}/su-exec" /usr/bin \
&& rm -fr "/tmp/su-exec-${SU_EXEC_VERSION}"

# Pull the latest version of Redis
ENV REDIS_VERSION 6.0.18
ENV REDIS_DOWNLOAD_URL http://download.redis.io/releases/redis-6.0.18.tar.gz
ENV REDIS_DOWNLOAD_SHA d7b4f2a97fcab96727284092b0a4aa854af47d570803fa0e7a0345359743836e

RUN set -eux; \
	wget -O redis.tar.gz "$REDIS_DOWNLOAD_URL"; \
	echo "$REDIS_DOWNLOAD_SHA *redis.tar.gz" | sha256sum -c -; \
	mkdir -p /usr/src/redis; \
	tar -xzf redis.tar.gz -C /usr/src/redis --strip-components=1; \
	rm redis.tar.gz; \
	export BUILD_TLS=yes; \
	make -C /usr/src/redis -j "$(nproc)" all; \
	make -C /usr/src/redis install; \
	rm -r /usr/src/redis; \
	redis-cli --version; \
	redis-server --version
RUN mkdir /data && chown redis:redis /data
VOLUME /data
WORKDIR /data

ENTRYPOINT ["docker-entrypoint.sh"]
EXPOSE 6379
CMD ["redis-server"]
# Placing it here to avoid saving its contents by podman.
# Ideally, we should not mark an empty directory as a volume.
VOLUME /imagevolume
