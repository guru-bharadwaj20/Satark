"""Runtime configuration from environment variables (prefix OPSBOT_)."""

from __future__ import annotations

from functools import lru_cache

from pydantic import Field, SecretStr
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="OPSBOT_", extra="ignore")

    # Shared secret Alertmanager sends as "Authorization: Bearer <token>".
    webhook_token: SecretStr = SecretStr("")
    queue_size: int = 100
    # Only alerts from these namespaces are diagnosed (empty = all).
    namespaces: list[str] = Field(
        default_factory=lambda: ["satark", "satark-data", "satark-sandbox", "monitoring", "argocd"]
    )

    # Kubernetes API (in-cluster defaults).
    kube_api: str = "https://kubernetes.default.svc"
    kube_token_path: str = "/var/run/secrets/kubernetes.io/serviceaccount/token"
    kube_ca_path: str = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

    loki_url: str = "http://loki-gateway.monitoring.svc.cluster.local"
    prometheus_url: str = (
        "http://kube-prometheus-stack-prometheus.monitoring.svc.cluster.local:9090"
    )

    # LLM
    llm_provider: str = "ollama"  # ollama | anthropic | openai-compatible
    llm_url: str = "http://ollama.satark.svc.cluster.local:11434"
    llm_model: str = "qwen2.5:7b-instruct"
    llm_api_key: SecretStr = SecretStr("")
    llm_timeout_s: float = 45.0

    # RAG
    database_url: SecretStr = SecretStr("")
    embed_model: str = "nomic-embed-text"
    embed_dim: int = 768
    runbooks_dir: str = "/app/docs/runbooks"

    # Telegram
    telegram_bot_token: SecretStr = SecretStr("")
    telegram_ops_chat_id: str = ""
    telegram_allowed_user_ids: list[int] = Field(default_factory=list)

    # Limits
    cooldown_s: int = 1800
    daily_token_budget: int = 200_000


@lru_cache
def get_settings() -> Settings:
    return Settings()
