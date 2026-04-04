from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    nats_url: str = "nats://localhost:4222"
    nats_stream: str = "TAAS_EVENTS"
    prometheus_url: str = "http://localhost:9090"
    postgres_dsn: str = "postgresql://taas:taas@localhost:5432/taas"
    check_interval_seconds: int = 30
    log_level: str = "INFO"

    # SLA thresholds per tier
    standard_p99_latency_ms: float = 5000.0
    professional_p99_latency_ms: float = 2000.0
    enterprise_p99_latency_ms: float = 1000.0
    error_rate_threshold: float = 0.01  # 1%

    class Config:
        env_prefix = "TAAS_"
        env_file = ".env"
