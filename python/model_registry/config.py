"""Model Registry — configuration."""

from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    nats_url: str = "nats://localhost:4222"
    postgres_dsn: str = "postgresql://taas:taas@localhost:5432/taas"
    log_level: str = "INFO"

    # S3 / object storage
    s3_endpoint_url: str = ""
    s3_bucket: str = "taas-models"
    s3_region: str = "us-east-1"
    s3_access_key: str = ""
    s3_secret_key: str = ""

    # Validation
    max_upload_bytes: int = 50 * 1024 * 1024 * 1024  # 50 GiB
    allowed_frameworks: list[str] = ["pytorch", "tensorflow", "onnx", "tensorrt"]
    allowed_formats: list[str] = ["safetensors", "bin", "onnx", "plan", "gguf"]

    class Config:
        env_prefix = "TAAS_REGISTRY_"
        env_file = ".env"
