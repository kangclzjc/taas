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
        """Create a Dynamo CRD in Kubernetes based on deploy_mode.

        deploy_mode == "dgdr":
            Creates a DynamoGraphDeploymentRequest — SLA-driven, auto-profiling.
            Dynamo runs AIConfigurator to find optimal config, then auto-deploys.

        deploy_mode == "dgd":
            Creates a DynamoGraphDeployment — Direct deploy with explicit config.
            No profiling, immediate deployment. User must specify all parameters.
        """
        deployment_id: str = payload["deployment_id"]
        model_id: str = payload["model_id"]
        org_id: str = payload["org_id"]
        deploy_mode: str = payload.get("deploy_mode", "dgdr")

        if deploy_mode == "dgd":
            await self._create_dgd(payload)
        else:
            await self._create_dgdr(payload)

    # ── DGDR: SLA-driven auto-profiling deployment ──────────────────────────

    async def _create_dgdr(self, payload: dict) -> None:
        """Create a DynamoGraphDeploymentRequest CRD."""
        deployment_id: str = payload["deployment_id"]

        spec: dict = {
            "modelId": payload.get("model_id", ""),
            "orgId": payload.get("org_id", ""),
            "deploymentId": deployment_id,
            "model": payload.get("model_name", ""),
            "backend": payload.get("backend", "vllm"),
        }

        if payload.get("backend_image"):
            spec["image"] = payload["backend_image"]

        # Hardware
        hardware: dict = {}
        if payload.get("gpu_type"):
            hardware["gpuSku"] = payload["gpu_type"]
        if payload.get("num_gpus_per_node"):
            hardware["numGpusPerNode"] = payload["num_gpus_per_node"]
        if payload.get("vram_mb"):
            hardware["vramMb"] = payload["vram_mb"]
        if hardware:
            spec["hardware"] = hardware

        # Workload profile
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

        if payload.get("search_strategy"):
            spec["searchStrategy"] = payload["search_strategy"]

        spec["autoApply"] = payload.get("auto_apply", True)

        logger.info(
            "Creating DGDR (auto-profiling mode)",
            extra={
                "deployment_id": deployment_id,
                "model": spec.get("model"),
                "backend": spec.get("backend"),
                "gpu_sku": hardware.get("gpuSku", "auto-detect"),
            },
        )
        await self._k8s.create_dynamo_worker(
            name=f"dgdr-{deployment_id}",
            spec=spec,
            kind="DynamoGraphDeploymentRequest",
        )

    # ── DGD: Direct deploy with explicit config ─────────────────────────────

    async def _create_dgd(self, payload: dict) -> None:
        """Create a DynamoGraphDeployment CRD (no profiling, immediate deploy)."""
        deployment_id: str = payload["deployment_id"]
        model_name: str = payload.get("model_name", "")
        backend: str = payload.get("backend", "vllm")
        image: str = payload.get("backend_image", "")
        dynamo_ns: str = payload.get("dynamo_namespace", f"taas-{deployment_id[:8]}")

        tp = payload.get("tensor_parallel_size", 1)
        pp = payload.get("pipeline_parallel_size", 1)
        gpu_per_replica = payload.get("gpu_count_per_replica", tp * pp)
        disagg = payload.get("disagg_enabled", False)
        router_mode = payload.get("router_mode", "kv" if disagg else "random")

        # Build services spec
        services: dict = {}

        # Frontend service
        frontend_replicas = payload.get("frontend_replicas", 1)
        frontend_envs = {"DYN_ROUTER_MODE": router_mode}
        services["Frontend"] = {
            "dynamoNamespace": dynamo_ns,
            "componentType": "frontend",
            "replicas": frontend_replicas,
            "extraPodSpec": {
                "mainContainer": {
                    "image": image,
                },
            },
            "envs": frontend_envs,
        }

        # Worker command
        worker_cmd = payload.get("worker_command", "")
        if not worker_cmd:
            # Build default command based on backend
            cmd_parts = [f"python3 -m dynamo.{backend}", f"--model {model_name}"]
            if tp > 1:
                cmd_parts.append(f"--tp {tp}")
            if pp > 1:
                cmd_parts.append(f"--pp {pp}")
            if payload.get("dtype"):
                cmd_parts.append(f"--dtype {payload['dtype']}")
            if payload.get("max_sequence_length"):
                cmd_parts.append(f"--max-model-len {payload['max_sequence_length']}")
            # Extra args
            for k, v in payload.get("extra_args", {}).items():
                cmd_parts.append(f"--{k} {v}")
            worker_cmd = " ".join(cmd_parts)

        # Worker env vars
        worker_envs = payload.get("env_vars", {})

        if disagg:
            # Disaggregated: separate prefill and decode workers
            prefill_replicas = payload.get("prefill_replicas", 1)
            decode_replicas = payload.get("decode_replicas", 1)

            prefill_cmd = worker_cmd + " --disaggregation-mode prefill"
            decode_cmd = worker_cmd + " --disaggregation-mode decode"

            services["PrefillWorker"] = {
                "dynamoNamespace": dynamo_ns,
                "componentType": "worker",
                "replicas": prefill_replicas,
                "resources": {"limits": {"gpu": str(gpu_per_replica)}},
                "extraPodSpec": {
                    "mainContainer": {
                        "image": image,
                        "command": ["/bin/sh", "-c"],
                        "args": [prefill_cmd],
                    },
                },
                "envs": worker_envs,
            }
            services["DecodeWorker"] = {
                "dynamoNamespace": dynamo_ns,
                "componentType": "worker",
                "replicas": decode_replicas,
                "resources": {"limits": {"gpu": str(gpu_per_replica)}},
                "extraPodSpec": {
                    "mainContainer": {
                        "image": image,
                        "command": ["/bin/sh", "-c"],
                        "args": [decode_cmd],
                    },
                },
                "envs": worker_envs,
            }
        else:
            # Aggregated: single worker type
            worker_replicas = payload.get("replicas_min", 1)
            services["Worker"] = {
                "dynamoNamespace": dynamo_ns,
                "componentType": "worker",
                "replicas": worker_replicas,
                "resources": {"limits": {"gpu": str(gpu_per_replica)}},
                "extraPodSpec": {
                    "mainContainer": {
                        "image": image,
                        "command": ["/bin/sh", "-c"],
                        "args": [worker_cmd],
                    },
                },
                "envs": worker_envs,
            }

        spec = {
            "deploymentId": deployment_id,
            "orgId": payload.get("org_id", ""),
            "services": services,
        }

        logger.info(
            "Creating DGD (direct deploy mode)",
            extra={
                "deployment_id": deployment_id,
                "model": model_name,
                "backend": backend,
                "disagg": disagg,
                "services": list(services.keys()),
            },
        )
        await self._k8s.create_dynamo_worker(
            name=f"dgd-{deployment_id}",
            spec=spec,
            kind="DynamoGraphDeployment",
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
