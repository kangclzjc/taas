"""
K8sClient — async Kubernetes client for managing DynamoWorker CRDs.
"""

from __future__ import annotations

import asyncio
import logging
from typing import Any, Callable, Coroutine, Optional

from kubernetes_asyncio import client, config, watch

logger = logging.getLogger(__name__)

# DynamoWorker CRD coordinates
CRD_GROUP = "dynamo.nvidia.com"
CRD_VERSION = "v1alpha1"
CRD_PLURAL = "dynamoworkers"


class K8sClient:
    """Manages NVIDIA Dynamo DynamoWorker custom resources in Kubernetes."""

    def __init__(self, namespace: str = "taas-dynamo") -> None:
        self._namespace = namespace
        self._api: Optional[client.CustomObjectsApi] = None

    async def _ensure_client(self) -> client.CustomObjectsApi:
        if self._api is None:
            try:
                config.load_incluster_config()
            except config.ConfigException:
                await config.load_kube_config()
            self._api = client.CustomObjectsApi()
        return self._api

    # ── CRUD ────────────────────────────────────────────────────────────────

    async def create_dynamo_worker(self, name: str, spec: dict) -> dict:
        """Create a DynamoWorker custom resource."""
        api = await self._ensure_client()

        body = {
            "apiVersion": f"{CRD_GROUP}/{CRD_VERSION}",
            "kind": "DynamoWorker",
            "metadata": {
                "name": name,
                "namespace": self._namespace,
                "labels": {
                    "app.kubernetes.io/managed-by": "taas-operator",
                    "taas.io/deployment-id": spec.get("deploymentId", ""),
                },
            },
            "spec": spec,
        }

        result = await api.create_namespaced_custom_object(
            group=CRD_GROUP,
            version=CRD_VERSION,
            namespace=self._namespace,
            plural=CRD_PLURAL,
            body=body,
        )
        logger.info("Created DynamoWorker %s", name)
        return result

    async def delete_dynamo_worker(self, name: str) -> None:
        """Delete a DynamoWorker custom resource."""
        api = await self._ensure_client()

        await api.delete_namespaced_custom_object(
            group=CRD_GROUP,
            version=CRD_VERSION,
            namespace=self._namespace,
            plural=CRD_PLURAL,
            name=name,
        )
        logger.info("Deleted DynamoWorker %s", name)

    async def get_worker_status(self, name: str) -> dict:
        """Retrieve the current status of a DynamoWorker."""
        api = await self._ensure_client()

        obj = await api.get_namespaced_custom_object(
            group=CRD_GROUP,
            version=CRD_VERSION,
            namespace=self._namespace,
            plural=CRD_PLURAL,
            name=name,
        )
        return obj.get("status", {})

    # ── Watch ───────────────────────────────────────────────────────────────

    async def watch_dynamo_workers(
        self,
        callback: Callable[[str, dict], Coroutine[Any, Any, None]],
    ) -> None:
        """
        Long-running watch on DynamoWorker CRDs.  Invokes *callback(event_type, object)*
        for each ADDED / MODIFIED / DELETED event.  Automatically retries on
        disconnections.
        """
        api = await self._ensure_client()
        w = watch.Watch()

        while True:
            try:
                async for event in w.stream(
                    api.list_namespaced_custom_object,
                    group=CRD_GROUP,
                    version=CRD_VERSION,
                    namespace=self._namespace,
                    plural=CRD_PLURAL,
                ):
                    event_type: str = event["type"]
                    obj: dict = event["object"]
                    try:
                        await callback(event_type, obj)
                    except Exception:
                        logger.exception(
                            "Error in watch callback for %s",
                            obj.get("metadata", {}).get("name", "?"),
                        )
            except asyncio.CancelledError:
                logger.info("DynamoWorker watch cancelled")
                raise
            except Exception:
                logger.exception("Watch stream error, reconnecting in 5s")
                await asyncio.sleep(5)
