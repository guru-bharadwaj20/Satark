# syntax=docker/dockerfile:1.7
# Build context: apps/qr-decoder

FROM python:3.12-slim-bookworm AS build
ENV PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1
RUN python -m venv /venv
WORKDIR /src
COPY pyproject.toml ./
COPY app ./app
# zxing-cpp and Pillow ship manylinux wheels, so no compiler is needed.
RUN /venv/bin/pip install --only-binary=:all: zxing-cpp pillow && /venv/bin/pip install .

FROM python:3.12-slim-bookworm
LABEL org.opencontainers.image.source="https://github.com/guru-bharadwaj20/satark" \
      org.opencontainers.image.title="satark-qr-decoder" \
      org.opencontainers.image.licenses="MIT"
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PATH=/venv/bin:$PATH PORT=8001
RUN groupadd --system --gid 10001 satark \
 && useradd --system --uid 10001 --gid satark --no-create-home --shell /usr/sbin/nologin satark
COPY --from=build /venv /venv
USER 10001:10001
WORKDIR /app
EXPOSE 8001
CMD ["sh", "-c", "exec uvicorn app.main:app --host 0.0.0.0 --port ${PORT} --no-access-log --proxy-headers"]
