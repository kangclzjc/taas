#!/usr/bin/env bash
# Install NVIDIA AI Dynamo Kubernetes Platform (official operator + bundled NATS; etcd off by default).
#
# Prerequisites:
#   - kubectl + helm 3.x, cluster admin
#   - GPU node with NVIDIA device plugin / GPU Operator, nvidia.com/gpu allocatable
#   - Optional: NGC API key if pulls from nvcr.io fail (imagePullSecret — see NGC docs)
#
# This does NOT modify TaaS. After install, deploy a graph (DGD YAML below), discover the Frontend
# Service, then point TaaS at it:
#   helm upgrade --install taas-local ./deploy/helm/taas ... \
#     -f deploy/helm/taas/values-local.yaml \
#     -f deploy/helm/taas/values-nvidia-dynamo.yaml \
#     --set-string dynamoOperator.mockInferenceBaseUrl=http://<frontend-svc>.dynamo-system.svc.cluster.local:8000 \
#     --set-string gateway.dynamoFrontendUrl=http://<frontend-svc>.dynamo-system.svc.cluster.local:8000
#
# Docs: https://docs.nvidia.com/dynamo/kubernetes-deployment/deployment-guide
#
set -euo pipefail

NAMESPACE="${NAMESPACE:-dynamo-system}"
RELEASE_NAME="${DYNAMO_RELEASE:-dynamo-platform}"
# Match chart version from: helm search repo nvidia-dynamo/dynamo-platform --versions
CHART_VERSION="${DYNAMO_CHART_VERSION:-1.0.1}"
HELM_REPO_NAME="${HELM_REPO_NAME:-nvidia-dynamo}"
HELM_REPO_URL="${HELM_REPO_URL:-https://helm.ngc.nvidia.com/nvidia/ai-dynamo}"

echo "==> Helm repo: ${HELM_REPO_NAME}"
if ! helm repo list 2>/dev/null | awk '{print $1}' | grep -qx "${HELM_REPO_NAME}"; then
  helm repo add "${HELM_REPO_NAME}" "${HELM_REPO_URL}"
fi
helm repo update "${HELM_REPO_NAME}"

echo "==> Installing/upgrading ${RELEASE_NAME} (chart ${CHART_VERSION}) in namespace ${NAMESPACE}"
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# Bitnami brownout: if you enable bundled etcd later, use bitnamilegacy — see NVIDIA install guide.
helm upgrade --install "${RELEASE_NAME}" "${HELM_REPO_NAME}/dynamo-platform" \
  --namespace "${NAMESPACE}" \
  --version "${CHART_VERSION}" \
  --wait \
  --timeout 20m

echo ""
echo "==> Verify"
kubectl get pods -n "${NAMESPACE}"
kubectl get crd 2>/dev/null | grep -E 'dynamographdeployment|nvidia.com' || true

echo ""
echo "Next steps:"
echo "  1) HuggingFace token (recommended):"
echo "       kubectl create secret generic hf-token-secret --from-literal=HF_TOKEN=\"\$HF_TOKEN\" -n ${NAMESPACE}"
echo "  2) Apply a vLLM graph (no profiling) — copy/edit deploy/extras/nvidia-dynamo-dgd-vllm-agg-dynamo-ns.yaml"
echo "       (set namespace + image tags to match ${NAMESPACE} / your NGC version), then:"
echo "       kubectl apply -f deploy/extras/nvidia-dynamo-dgd-vllm-agg-dynamo-ns.yaml"
echo "     Or legacy DGDR (profiling): deploy/extras/nvidia-dynamo-dgdr-qwen3-vllm.yaml"
echo "  3) Watch: kubectl get dynamographdeployment -n ${NAMESPACE} -w"
echo "  4) Frontend Service: kubectl get svc -n ${NAMESPACE} | grep -i frontend"
echo "  5) Wire TaaS gateway + taas-dynamo-operator mockInferenceBaseUrl to that http://<svc>.${NAMESPACE}.svc.cluster.local:8000"
echo ""
echo "If GPU pods stay Pending, add tolerations for nvidia.com/gpu:NoSchedule (see YAML comments)."
