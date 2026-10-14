# syntax=docker/dockerfile:1
FROM python:3.12-slim-bookworm

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    HOME=/tmp

WORKDIR /srv
COPY pyproject.toml ./
COPY app/ app/
RUN pip install . \
 && playwright install --with-deps --only-shell chromium \
 && rm -rf /var/lib/apt/lists/* /root/.cache \
 && groupadd --gid 10001 sandbox \
 && useradd --uid 10001 --gid 10001 --no-create-home --home-dir /tmp --shell /usr/sbin/nologin sandbox \
 && chmod -R a+rX /ms-playwright

# The pod runs with a read-only root filesystem; Chromium writes only to
# /tmp and /dev/shm, which the operator mounts as emptyDir volumes.
USER 10001:10001
ENTRYPOINT ["python", "-m", "app"]
