"""Model Registry — Pydantic models."""

from __future__ import annotations

from enum import Enum
from typing import Optional

from pydantic import BaseModel, Field


class ModelValidationStatus(str, Enum):
    PENDING = "pending"
    VALIDATING = "validating"
    VALID = "valid"
    INVALID = "invalid"
    ERROR = "error"


class ModelUploadResponse(BaseModel):
    model_id: str
    storage_uri: str
    status: ModelValidationStatus = ModelValidationStatus.PENDING
    message: Optional[str] = None


class ValidationResult(BaseModel):
    model_id: str
    status: ModelValidationStatus
    framework: str
    format: str
    file_size_bytes: int = 0
    parameter_count: Optional[int] = None
    errors: list[str] = Field(default_factory=list)
    warnings: list[str] = Field(default_factory=list)


class ModelMetadata(BaseModel):
    """Lightweight metadata returned from storage inspection."""
    storage_uri: str
    file_size_bytes: int
    content_type: str = ""
    etag: str = ""
