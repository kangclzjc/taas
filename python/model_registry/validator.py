"""Model Registry — model artifact validator."""

from __future__ import annotations

import logging
from pathlib import PurePosixPath
from typing import Optional

from .config import Settings
from .models import ModelValidationStatus, ValidationResult

logger = logging.getLogger(__name__)


class ModelValidator:
    """Validates uploaded model artifacts for framework, format, and size."""

    def __init__(self, settings: Settings) -> None:
        self._settings = settings

    async def validate(
        self,
        model_id: str,
        storage_uri: str,
        framework: str,
        format: str,
        file_size_bytes: int,
    ) -> ValidationResult:
        """
        Run all validation checks and return a consolidated result.
        """
        errors: list[str] = []
        warnings: list[str] = []

        # ── Framework check ─────────────────────────────────────────────
        if framework not in self._settings.allowed_frameworks:
            errors.append(
                f"Unsupported framework '{framework}'. "
                f"Allowed: {', '.join(self._settings.allowed_frameworks)}"
            )

        # ── Format check ────────────────────────────────────────────────
        if format and format not in self._settings.allowed_formats:
            errors.append(
                f"Unsupported format '{format}'. "
                f"Allowed: {', '.join(self._settings.allowed_formats)}"
            )

        # ── File extension sanity ───────────────────────────────────────
        if storage_uri:
            ext = PurePosixPath(storage_uri).suffix.lstrip(".")
            if ext and format and ext != format:
                warnings.append(
                    f"File extension '.{ext}' does not match declared format '{format}'"
                )

        # ── Size check ──────────────────────────────────────────────────
        if file_size_bytes <= 0:
            errors.append("File size must be greater than 0")
        elif file_size_bytes > self._settings.max_upload_bytes:
            errors.append(
                f"File size {file_size_bytes} exceeds maximum "
                f"{self._settings.max_upload_bytes} bytes"
            )

        # ── Result ──────────────────────────────────────────────────────
        status = (
            ModelValidationStatus.VALID
            if not errors
            else ModelValidationStatus.INVALID
        )

        result = ValidationResult(
            model_id=model_id,
            status=status,
            framework=framework,
            format=format,
            file_size_bytes=file_size_bytes,
            errors=errors,
            warnings=warnings,
        )

        logger.info(
            "Validation result for %s: %s (%d errors, %d warnings)",
            model_id,
            status.value,
            len(errors),
            len(warnings),
        )
        return result
