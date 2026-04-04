"""Model Registry — S3 object storage client."""

from __future__ import annotations

import logging
from typing import BinaryIO, Union

import aiobotocore.session
from aiobotocore.session import AioSession
from fastapi import UploadFile

from .config import Settings
from .models import ModelMetadata

logger = logging.getLogger(__name__)


class ModelStorage:
    """Upload, download, and inspect model artifacts in S3-compatible storage."""

    def __init__(self, settings: Settings) -> None:
        self._settings = settings
        self._session: AioSession = aiobotocore.session.get_session()

    def _client_kwargs(self) -> dict:
        kwargs: dict = {
            "region_name": self._settings.s3_region,
        }
        if self._settings.s3_endpoint_url:
            kwargs["endpoint_url"] = self._settings.s3_endpoint_url
        if self._settings.s3_access_key:
            kwargs["aws_access_key_id"] = self._settings.s3_access_key
            kwargs["aws_secret_access_key"] = self._settings.s3_secret_key
        return kwargs

    async def upload(
        self,
        model_id: str,
        file: Union[UploadFile, BinaryIO],
        prefix: str = "models",
    ) -> str:
        """
        Stream-upload a model file to S3 and return the storage URI.

        Returns:
            The ``s3://<bucket>/<key>`` URI of the uploaded object.
        """
        # Determine filename
        if isinstance(file, UploadFile):
            filename = file.filename or "model.bin"
        else:
            filename = "model.bin"

        key = f"{prefix}/{model_id}/{filename}"

        async with self._session.create_client("s3", **self._client_kwargs()) as s3:
            if isinstance(file, UploadFile):
                body = await file.read()
            else:
                body = file.read()

            await s3.put_object(
                Bucket=self._settings.s3_bucket,
                Key=key,
                Body=body,
            )

        uri = f"s3://{self._settings.s3_bucket}/{key}"
        logger.info("Uploaded model %s → %s", model_id, uri)
        return uri

    async def get_metadata(self, model_id: str, key: str) -> ModelMetadata:
        """Retrieve object metadata (HEAD) without downloading the body."""
        async with self._session.create_client("s3", **self._client_kwargs()) as s3:
            resp = await s3.head_object(
                Bucket=self._settings.s3_bucket,
                Key=key,
            )
            return ModelMetadata(
                storage_uri=f"s3://{self._settings.s3_bucket}/{key}",
                file_size_bytes=resp.get("ContentLength", 0),
                content_type=resp.get("ContentType", ""),
                etag=resp.get("ETag", ""),
            )

    async def delete(self, key: str) -> None:
        """Delete an object from S3."""
        async with self._session.create_client("s3", **self._client_kwargs()) as s3:
            await s3.delete_object(
                Bucket=self._settings.s3_bucket,
                Key=key,
            )
            logger.info("Deleted object %s", key)
