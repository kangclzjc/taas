#!/usr/bin/env bash
# Point TaaS gateway + dynamo-operator at an existing NVIDIA Dynamo Frontend Service (OpenAI-compatible base URL).
#
# Usage:
#   NAMESPACE=taas-local RELEASE=taas-local DYNAMO_NS=dynamo ./scripts/bind-taas-dynamo-frontend.sh
#
# Picks the first Service in DYNAMO_NS whose name contains "frontend" (case-insensitive). Override with FRONTEND_SVC.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

NAMESPACE="${NAMESPACE:-taas-local}"
RELEASE="${RELEASE:-taas-local}"
DYNAMO_NS="${DYNAMO_NS:-dynamo}"
FRONTEND_SVC="${FRONTEND_SVC:-}"
FRONTEND_PORT="${FRONTEND_PORT:-8000}"

pick_svc() {
  if [[ -n "${FRONTEND_SVC}" ]]; then
    echo "${FRONTEND_SVC}"
    return
  fi
  kubectl get svc -n "${DYNAMO_NS}" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null |
    grep -i frontend |
    head -1
}

SVC="$(pick_svc)"
if [[ -z "${SVC}" ]]; then
  echo "No *frontend* Service in namespace ${DYNAMO_NS}." >&2
  echo "Deploy a model first (DynamoGraphDeployment or DGDR), then:" >&2
  echo "  kubectl get svc -n ${DYNAMO_NS} | grep -i frontend" >&2
  echo "Or: FRONTEND_SVC=my-frontend ${0}" >&2
  exit 1
fi

BASE="http://${SVC}.${DYNAMO_NS}.svc.cluster.local:${FRONTEND_PORT}"
echo "==> Binding TaaS to Dynamo Frontend: ${BASE}"

helm upgrade "${RELEASE}" deploy/helm/taas \
  --namespace "${NAMESPACE}" \
  --reuse-values \
  --set-string gateway.dynamoFrontendUrl="${BASE}" \
  --set-string dynamoOperator.mockInferenceBaseUrl="${BASE}"

echo "==> Restart gateway + dynamo-operator to pick up env"
for comp in gateway dynamo-operator; do
  dep=$(kubectl get deploy -n "${NAMESPACE}" -l "app.kubernetes.io/component=${comp},app.kubernetes.io/instance=${RELEASE}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)
  if [[ -n "${dep}" ]]; then
    kubectl rollout restart "deployment/${dep}" -n "${NAMESPACE}"
    kubectl rollout status "deployment/${dep}" -n "${NAMESPACE}" --timeout=120s || true
  fi
done

echo "Done. Gateway proxies /v1/* to ${BASE}"
