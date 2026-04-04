"""
SLA Evaluator — queries Prometheus for deployment metrics,
compares against SLA thresholds, and publishes violations to NATS.
"""

import asyncio
import json
import logging
from datetime import datetime, timezone
from typing import Any

import httpx
import nats

from .config import Settings

logger = logging.getLogger(__name__)


class SLAEvaluator:
    """Periodically evaluates SLA compliance for all active deployments."""

    def __init__(self, settings: Settings, nc: nats.NATS):
        self.settings = settings
        self.nc = nc
        self.http = httpx.AsyncClient(base_url=settings.prometheus_url, timeout=10.0)

    async def run_loop(self):
        """Main evaluation loop — runs every check_interval_seconds."""
        js = self.nc.jetstream()
        interval = self.settings.check_interval_seconds

        while True:
            try:
                await self._evaluate_all(js)
            except asyncio.CancelledError:
                break
            except Exception:
                logger.exception("SLA evaluation cycle failed")

            await asyncio.sleep(interval)

    async def _evaluate_all(self, js):
        """Evaluate SLA compliance for every active deployment."""
        deployments = await self._get_active_deployments()
        if not deployments:
            logger.debug("No active deployments to evaluate")
            return

        for deployment in deployments:
            deployment_id = deployment["deployment_id"]
            org_id = deployment["org_id"]
            sla_tier = deployment.get("sla_tier", "standard")

            try:
                violations = await self._check_deployment(deployment_id, sla_tier)
                for violation in violations:
                    event = {
                        "deployment_id": deployment_id,
                        "org_id": org_id,
                        "sla_tier": sla_tier,
                        "violation_type": violation["type"],
                        "threshold": violation["threshold"],
                        "actual_value": violation["actual"],
                        "timestamp": datetime.now(timezone.utc).isoformat(),
                    }
                    await js.publish(
                        f"sla.violation.{deployment_id}",
                        json.dumps(event).encode(),
                    )
                    logger.warning(
                        "SLA violation: %s for deployment %s (threshold=%.2f, actual=%.2f)",
                        violation["type"],
                        deployment_id,
                        violation["threshold"],
                        violation["actual"],
                    )
            except Exception:
                logger.exception("Failed to evaluate deployment %s", deployment_id)

    async def _check_deployment(self, deployment_id: str, sla_tier: str) -> list[dict]:
        """Check a single deployment against its SLA tier thresholds."""
        violations = []

        # Get P99 latency
        latency_threshold = self._get_latency_threshold(sla_tier)
        p99_latency = await self._query_p99_latency(deployment_id)

        if p99_latency is not None and p99_latency > latency_threshold:
            violations.append({
                "type": "latency_p99",
                "threshold": latency_threshold,
                "actual": p99_latency,
            })

        # Get error rate
        error_rate = await self._query_error_rate(deployment_id)
        if error_rate is not None and error_rate > self.settings.error_rate_threshold:
            violations.append({
                "type": "error_rate",
                "threshold": self.settings.error_rate_threshold,
                "actual": error_rate,
            })

        return violations

    def _get_latency_threshold(self, sla_tier: str) -> float:
        """Return the P99 latency threshold in ms for a given SLA tier."""
        thresholds = {
            "standard": self.settings.standard_p99_latency_ms,
            "professional": self.settings.professional_p99_latency_ms,
            "enterprise": self.settings.enterprise_p99_latency_ms,
        }
        return thresholds.get(sla_tier, self.settings.standard_p99_latency_ms)

    async def _query_p99_latency(self, deployment_id: str) -> float | None:
        """Query Prometheus for P99 inference latency over the last 5 minutes."""
        query = (
            f'histogram_quantile(0.99, rate(taas_inference_latency_seconds_bucket'
            f'{{deployment_id="{deployment_id}"}}[5m]))'
        )
        result = await self._prometheus_query(query)
        if result is not None:
            return result * 1000  # Convert to ms
        return None

    async def _query_error_rate(self, deployment_id: str) -> float | None:
        """Query Prometheus for error rate over the last 5 minutes."""
        total_query = (
            f'sum(rate(taas_inference_requests_total'
            f'{{deployment_id="{deployment_id}"}}[5m]))'
        )
        error_query = (
            f'sum(rate(taas_inference_requests_total'
            f'{{deployment_id="{deployment_id}", status="error"}}[5m]))'
        )

        total = await self._prometheus_query(total_query)
        errors = await self._prometheus_query(error_query)

        if total is not None and total > 0 and errors is not None:
            return errors / total
        return None

    async def _prometheus_query(self, query: str) -> float | None:
        """Execute a PromQL instant query and return the scalar result."""
        try:
            resp = await self.http.get("/api/v1/query", params={"query": query})
            resp.raise_for_status()
            data = resp.json()

            if data.get("status") != "success":
                return None

            results = data.get("data", {}).get("result", [])
            if not results:
                return None

            # Return the first result's value
            value_pair = results[0].get("value", [])
            if len(value_pair) >= 2:
                return float(value_pair[1])
        except (httpx.HTTPError, ValueError, IndexError, KeyError):
            logger.debug("Prometheus query failed: %s", query)
        return None

    async def _get_active_deployments(self) -> list[dict[str, Any]]:
        """
        Query Prometheus for deployments that have received traffic recently.
        In production, this would query the database instead.
        """
        query = 'group by (deployment_id, org_id, sla_tier) (taas_inference_active_requests)'
        try:
            resp = await self.http.get("/api/v1/query", params={"query": query})
            resp.raise_for_status()
            data = resp.json()

            if data.get("status") != "success":
                return []

            results = data.get("data", {}).get("result", [])
            return [
                {
                    "deployment_id": r["metric"].get("deployment_id", ""),
                    "org_id": r["metric"].get("org_id", ""),
                    "sla_tier": r["metric"].get("sla_tier", "standard"),
                }
                for r in results
                if r.get("metric", {}).get("deployment_id")
            ]
        except (httpx.HTTPError, ValueError):
            logger.debug("Failed to query active deployments")
            return []
