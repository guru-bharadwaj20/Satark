# syntax=docker/dockerfile:1.7
# Build context: repository root (the image bundles rules/*.yaml).
#   docker build -f apps/text-analyzer/Dockerfile .

FROM python:3.12-slim-bookworm AS build
ENV PIP_NO_CACHE_DIR=1 PIP_DISABLE_PIP_VERSION_CHECK=1
RUN python -m venv /venv
WORKDIR /src
COPY apps/text-analyzer/pyproject.toml ./
COPY apps/text-analyzer/app ./app
RUN /venv/bin/pip install .

FROM python:3.12-slim-bookworm
LABEL org.opencontainers.image.source="https://github.com/guru-bharadwaj20/satark" \
      org.opencontainers.image.title="satark-text-analyzer" \
      org.opencontainers.image.licenses="MIT"
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PATH=/venv/bin:$PATH \
    RULES_DIR=/app/rules PORT=8000
RUN groupadd --system --gid 10001 satark \
 && useradd --system --uid 10001 --gid satark --no-create-home --shell /usr/sbin/nologin satark
COPY --from=build /venv /venv
COPY --chown=root:root rules /app/rules
USER 10001:10001
WORKDIR /app
EXPOSE 8000
CMD ["sh", "-c", "exec uvicorn app.main:app --host 0.0.0.0 --port ${PORT} --no-access-log --proxy-headers"]
