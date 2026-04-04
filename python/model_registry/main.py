"""
TaaS Model Registry

Handles model upload, validation, format conversion, and storage management.
"""

import logging
from contextlib import asynccontextmanager

import nats
from fastapi import FastAPI, File, Form, UploadFile, HTTPException, Depends
from fastapi.responses import JSONResponse
from prometheus_client import make_asgi_app

from .config import Settings
from .models import ModelUploadResponse, ModelValidationStatus
from .validator import ModelValidator
from .storage import ModelStorage

logger = logging.getLogger(__name__)

settings = Settings()


@asynccontextmanager
async def lifespan(app: FastAPI):
    nc = await nats.connect(settings.nats_url)
    app.state.nc = nc
    app.state.storage = ModelStorage(settings)
    app.state.validator = ModelValidator(settings)
    logger.info("Model Registry started")
    yield
    await nc.drain()
    logger.info("Model Registry stopped")


app = FastAPI(
    title="TaaS Model Registry",
    description="Model upload, validation, and storage management",
    version="0.1.0",
    lifespan=lifespan,
)

metrics_app = make_asgi_app()
app.mount("/metrics", metrics_app)


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.get("/health/ready")
async def ready():
    return {"status": "ready"}


@app.post("/internal/models/{model_id}/upload", response_model=ModelUploadResponse)
async def upload_model(
    model_id: str,
    file: UploadFile = File(...),
    framework: str = Form(...),
    format: str = Form(...),
):
    """
    Upload a model file. Called internally after the API creates the model record.
    Streams the file to object storage and queues background validation.
    """
    storage: ModelStorage = app.state.storage
    nc: nats.NATS = app.state.nc

    storage_uri = await storage.upload(model_id=model_id, file=file)

    # Publish event to trigger async validation
    js = nc.jetstream()
    await js.publish(
        f"model.validate.{model_id}",
        f'{{"model_id": "{model_id}", "storage_uri": "{storage_uri}", "framework": "{framework}", "format": "{format}"}}'.encode(),
    )

    return ModelUploadResponse(
        model_id=model_id,
        storage_uri=storage_uri,
        status=ModelValidationStatus.PENDING,
    )
