"""
TaaS SLA Monitor

Continuously evaluates SLA compliance per tenant/deployment.
Reads metrics from Prometheus and publishes alerts via NATS.
"""

import asyncio
import logging
from contextlib import asynccontextmanager
from datetime import datetime, timezone

import nats
from fastapi import FastAPI
from prometheus_client import make_asgi_app

from .config import Settings
from .evaluator import SLAEvaluator

logger = logging.getLogger(__name__)

settings = Settings()


@asynccontextmanager
async def lifespan(app: FastAPI):
    nc = await nats.connect(settings.nats_url)
    evaluator = SLAEvaluator(settings=settings, nc=nc)

    eval_task = asyncio.create_task(evaluator.run_loop())
    logger.info("SLA Monitor started")

    yield

    eval_task.cancel()
    await nc.drain()
    logger.info("SLA Monitor stopped")


app = FastAPI(title="TaaS SLA Monitor", version="0.1.0", lifespan=lifespan)
metrics_app = make_asgi_app()
app.mount("/metrics", metrics_app)


@app.get("/health")
async def health():
    return {"status": "ok", "time": datetime.now(timezone.utc).isoformat()}


@app.get("/health/ready")
async def ready():
    return {"status": "ready"}
