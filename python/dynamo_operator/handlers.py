"""
DeploymentHandler — processes NATS deployment events and drives K8s CRD lifecycle.
"""

from __future__ import annotations

import asyncio
import json
import logging
from typing import Any

from nats.aio.client import Client as NATS
from nats.js import JetStreamContext

from .config import Settings
from .k8s_client import K8sClient

logger = logging.getLogger(__name__)


class DeploymentHandler:
    """Consumes NATS deployment events and manages DynamoWorker CRDs."""

    def __init__(
        self,
        js: JetStreamContext,
        k8s: K8sClient,
        settings: Settings,
    ) -> None:
        self._js = js
        self._k8s = k8s
        self._settings = settings

    # ── Main consumer loop ──────────────────────────────────────────────────

    async def consume(self, subscription: Any) -> None:
        """
        Continuously pull messages from the NATS subscription and dispatch
        them to the appropriate handler based on subject.
        """
        try:
            async for msg in subscription.messages:
                subject = msg.subject
                try:
                    payload = json.loads(msg.data.decode())
                    logger.info("Received event", extra={"subject": subject, "payload": payload})

                    if subject == "model.deploy.requested":
                        await self._handle_deploy_requested(payload)
                    elif subject == "model.deploy.completed":
                        await self._handle_deploy_completed(payload)
                    elif subject == "model.deploy.failed":
                        await self._handle_deploy_failed(payload)
                    else:
                        logger.warning("Unknown subject: %s", subject)

                    await msg.ack()
                except Exception:
                    logger.exception("Error processing message on %s", subject)
                    await msg.nak(delay=5)
        except asyncio.CancelledError:
            logger.info("Consumer loop cancelled")
            raise

    # ── Event handlers ──────────────────────────────────────────────────────

    async def _handle_deploy_requested(self, payload: dict) -> None:
        """Create a DynamoWorker CRD in Kubernetes for the requested deployment."""
        deployment_id: str = payload["deployment_id"]
        model_id: str = payload["model_id"]
        org_id: str = payload["org_id"]

        spec = {
            "modelId": model_id,
            "orgId": org_id,
            "deploymentId": deployment_id,
            "slaTier": payload.get("sla_tier", "standard"),
            "replicas": {
                "min": payload.get("replicas_min", 1),
                "max": payload.get("replicas_max", 1),
            },
            "gpu": {
                "type": payload.get("gpu_type", ""),
                "countPerReplica": payload.get("gpu_count_per_replica", 1),
            },
            "model": {
                "storageUri": payload.get("storage_uri", ""),
                "framework": payload.get("framework", ""),
                "format": payload.get("format", ""),
            },
            "inference": {
                "maxBatchSize": payload.get("max_batch_size", 0),
                "maxSequenceLength": payload.get("max_sequence_length", 0),
            },
        }

        logger.info(
            "Creating DynamoWorker CRD",
            extra={"deployment_id": deployment_id, "model_id": model_id},
        )
        await self._k8s.create_dynamo_worker(
            name=f"deploy-{deployment_id}",
            spec=spec,
        )

    async def _handle_deploy_completed(self, payload: dict) -> None:
        """Handle successful deployment — update status via NATS publish."""
        deployment_id: str = payload["deployment_id"]
        endpoint_url: str = payload.get("endpoint_url", "")

        logger.info(
            "Deployment completed",
            extra={"deployment_id": deployment_id, "endpoint_url": endpoint_url},
        )

        # Publish status update for the Go gateway to consume
        await self._js.publish(
            "deployment.status.updated",
            json.dumps(
                {
                    "deployment_id": deployment_id,
                    "status": "running",
                    "endpoint_url": endpoint_url,
                }
            ).encode(),
        )

    async def _handle_deploy_failed(self, payload: dict) -> None:
        """Handle a failed deployment — clean up the CRD and publish failure."""
        deployment_id: str = payload["deployment_id"]
        error_message: str = payload.get("error", "unknown error")

        logger.error(
            "Deployment failed",
            extra={"deployment_id": deployment_id, "error": error_message},
        )

        # Attempt to clean up the CRD
        try:
            await self._k8s.delete_dynamo_worker(name=f"deploy-{deployment_id}")
        except Exception:
            logger.exception("Failed to clean up CRD for %s", deployment_id)

        await self._js.publish(
            "deployment.status.updated",
            json.dumps(
                {
                    "deployment_id": deployment_id,
                    "status": "failed",
                    "error_message": error_message,
                }
            ).encode(),
        )

    # ── K8s watcher callback ────────────────────────────────────────────────

    async def on_worker_event(self, event_type: str, worker: dict) -> None:
        """
        Callback invoked by K8sClient.watch_dynamo_workers when a
        DynamoWorker CRD status changes.
        """
        metadata = worker.get("metadata", {})
        status = worker.get("status", {})
        name = metadata.get("name", "unknown")
        deployment_id = worker.get("spec", {}).get("deploymentId", "")

        logger.info(
            "K8s worker event",
            extra={"type": event_type, "name": name, "status": status},
        )

        phase = status.get("phase", "")

        if event_type in ("ADDED", "MODIFIED"):
            if phase == "Running":
                endpoint = status.get("endpointUrl", "")
                await self._js.publish(
                    "model.deploy.completed",
                    json.dumps(
                        {"deployment_id": deployment_id, "endpoint_url": endpoint}
                    ).encode(),
                )
            elif phase == "Failed":
                error_msg = status.get("message", "CRD reported failure")
                await self._js.publish(
                    "model.deploy.failed",
                    json.dumps(
                        {"deployment_id": deployment_id, "error": error_msg}
                    ).encode(),
                )
