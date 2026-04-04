"""
TaaS Dynamo Operator

Watches NATS JetStream for model deployment/teardown events and manages
DynamoWorker Kubernetes CRDs accordingly.
"""

import asyncio
import logging
import os
import signal
from contextlib import asynccontextmanager

import nats
from fastapi import FastAPI
from prometheus_client import Counter, Gauge, make_asgi_app

from .config import Settings
from .handlers import DeploymentHandler
from .k8s_client import K8sClient

logger = logging.getLogger(__name__)

settings = Settings()

# ─── Metrics ──────────────────────────────────────────────────────────────────
deployments_total = Counter(
    "taas_operator_deployments_total",
    "Total deployment operations",
    ["operation", "status"],
)
active_deployments = Gauge(
    "taas_operator_active_deployments",
    "Current number of active deployments",
)


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Start NATS consumer and K8s watcher on startup."""
    nc = await nats.connect(settings.nats_url)
    js = nc.jetstream()

    k8s = K8sClient(namespace=settings.dynamo_namespace)
    handler = DeploymentHandler(js=js, k8s=k8s, settings=settings)

    # Subscribe to deployment events
    sub = await js.subscribe(
        "model.deploy.*",
        durable="dynamo-operator",
        stream="TAAS_EVENTS",
    )

    # Start consumers
    consumer_task = asyncio.create_task(handler.consume(sub))

    # Start K8s CRD watcher
    watcher_task = asyncio.create_task(k8s.watch_dynamo_workers(handler.on_worker_event))

    logger.info("Dynamo Operator started", extra={"namespace": settings.dynamo_namespace})

    yield

    consumer_task.cancel()
    watcher_task.cancel()
    await nc.drain()
    logger.info("Dynamo Operator stopped")


app = FastAPI(
    title="TaaS Dynamo Operator",
    version="0.1.0",
    lifespan=lifespan,
)

# Prometheus metrics endpoint
metrics_app = make_asgi_app()
app.mount("/metrics", metrics_app)


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.get("/health/ready")
async def ready():
    # TODO: check NATS connection, K8s API connectivity
    return {"status": "ready"}
