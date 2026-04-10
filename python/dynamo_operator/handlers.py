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
        """Create a DynamoGraphDeploymentRequest (DGDR) CRD in Kubernetes."""
        deployment_id: str = payload["deployment_id"]
        model_id: str = payload["model_id"]
        org_id: str = payload["org_id"]

        # Build DGDR spec from TaaS deployment parameters
        spec: dict = {
            # Core identifiers
            "modelId": model_id,
            "orgId": org_id,
            "deploymentId": deployment_id,

            # Model reference (HuggingFace ID or storage URI)
            "model": payload.get("model_name", ""),

            # Inference backend: vllm, sglang, trtllm
            "backend": payload.get("backend", "vllm"),
        }

        # Container image (optional override)
        if payload.get("backend_image"):
            spec["image"] = payload["backend_image"]

        # Hardware configuration
        hardware: dict = {}
        if payload.get("gpu_type"):
            hardware["gpuSku"] = payload["gpu_type"]
        if payload.get("num_gpus_per_node"):
            hardware["numGpusPerNode"] = payload["num_gpus_per_node"]
        if payload.get("vram_mb"):
            hardware["vramMb"] = payload["vram_mb"]
        if hardware:
            spec["hardware"] = hardware

        # Workload profile (for AIConfigurator SLA optimization)
        workload: dict = {}
        if payload.get("input_sequence_length"):
            workload["isl"] = payload["input_sequence_length"]
        if payload.get("output_sequence_length"):
            workload["osl"] = payload["output_sequence_length"]
        if workload:
            spec["workload"] = workload

        # SLA targets
        sla: dict = {}
        if payload.get("target_ttft_ms"):
            sla["ttft"] = payload["target_ttft_ms"]
        if payload.get("target_itl_ms"):
            sla["itl"] = payload["target_itl_ms"]
        if payload.get("target_tpot_ms"):
            sla["tpot"] = payload["target_tpot_ms"]
        if sla:
            spec["sla"] = sla

        # AIConfigurator search strategy
        if payload.get("search_strategy"):
            spec["searchStrategy"] = payload["search_strategy"]

        # Auto-apply: automatically create DGD after profiling (default: true)
        spec["autoApply"] = payload.get("auto_apply", True)

        # Scaling / replicas (for manual DGD, used when disagg is explicit)
        replicas: dict = {
            "min": payload.get("replicas_min", 1),
            "max": payload.get("replicas_max", payload.get("replicas_min", 1)),
        }

        # Disaggregated serving (prefill/decode separation)
        disagg_enabled = payload.get("disagg_enabled", False)
        if disagg_enabled:
            spec["disaggregated"] = {
                "enabled": True,
                "prefillReplicas": payload.get("prefill_replicas", 1),
                "decodeReplicas": payload.get("decode_replicas", 1),
            }

        # Parallelism
        parallelism: dict = {}
        if payload.get("tensor_parallel_size", 1) > 1:
            parallelism["tensorParallelSize"] = payload["tensor_parallel_size"]
        if payload.get("pipeline_parallel_size", 1) > 1:
            parallelism["pipelineParallelSize"] = payload["pipeline_parallel_size"]
        if parallelism:
            spec["parallelism"] = parallelism

        # Advanced inference settings
        inference: dict = {}
        if payload.get("max_batch_size"):
            inference["maxBatchSize"] = payload["max_batch_size"]
        if payload.get("max_sequence_length"):
            inference["maxSequenceLength"] = payload["max_sequence_length"]
        if payload.get("dtype"):
            inference["dtype"] = payload["dtype"]
        if inference:
            spec["inference"] = inference

        # Legacy fields for backward compat
        spec["slaTier"] = payload.get("sla_tier", "standard")
        spec["replicas"] = replicas

        # GPU per worker
        if payload.get("gpu_count_per_replica"):
            spec.setdefault("gpu", {})["countPerReplica"] = payload["gpu_count_per_replica"]

        # Model storage info (for non-HuggingFace models)
        if payload.get("storage_uri") or payload.get("framework") or payload.get("format"):
            spec["modelStorage"] = {
                "storageUri": payload.get("storage_uri", ""),
                "framework": payload.get("framework", ""),
                "format": payload.get("format", ""),
            }

        # Extra backend-specific args
        if payload.get("extra_args"):
            spec["extraArgs"] = payload["extra_args"]

        logger.info(
            "Creating DGDR CRD",
            extra={
                "deployment_id": deployment_id,
                "model": spec.get("model"),
                "backend": spec.get("backend"),
                "disagg": disagg_enabled,
                "gpu_sku": hardware.get("gpuSku", "auto"),
            },
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
