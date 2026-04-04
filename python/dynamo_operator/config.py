from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    nats_url: str = "nats://localhost:4222"
    nats_stream: str = "TAAS_EVENTS"
    dynamo_namespace: str = "taas-dynamo"
    dynamo_api_version: str = "dynamo.nvidia.com/v1alpha1"
    postgres_dsn: str = "postgresql://taas:taas@localhost:5432/taas"
    redis_url: str = "redis://localhost:6379"
    log_level: str = "INFO"
    otlp_endpoint: str = ""
    # S3 / object storage for model artifacts
    model_storage_bucket: str = "taas-models"
    model_storage_region: str = "us-east-1"

    class Config:
        env_prefix = "TAAS_"
        env_file = ".env"
