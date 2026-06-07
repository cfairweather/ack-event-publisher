#!/usr/bin/env bash
# Layer 2: Install Helm charts onto the EKS cluster.
# Run after `make test-infra` completes and the cluster is ACTIVE.
#
# Prerequisites:
#   aws CLI configured with sufficient permissions
#   helm >= 3.8 (OCI support)
#   kubectl
#
# Usage:
#   STACK_NAME=ack-event-publisher-test AWS_REGION=us-east-1 ./setup.sh

set -euo pipefail

STACK_NAME="${STACK_NAME:-ack-event-publisher-test}"
AWS_REGION="${AWS_REGION:-us-east-1}"
ACK_S3_CHART_VERSION="${ACK_S3_CHART_VERSION:-1.6.0}"
PUBLISHER_CHART_VERSION="${PUBLISHER_CHART_VERSION:-}"  # empty = latest

echo "==> Fetching cluster name from CloudFormation stack ${STACK_NAME}"
CLUSTER_NAME=$(aws cloudformation describe-stacks \
  --stack-name "${STACK_NAME}" \
  --region "${AWS_REGION}" \
  --query "Stacks[0].Outputs[?OutputKey=='ClusterName'].OutputValue" \
  --output text)

echo "==> Updating kubeconfig for cluster ${CLUSTER_NAME}"
aws eks update-kubeconfig \
  --name "${CLUSTER_NAME}" \
  --region "${AWS_REGION}"

echo "==> Waiting for cluster nodes to be Ready"
kubectl wait nodes \
  --all \
  --for=condition=Ready \
  --timeout=300s 2>/dev/null || true

echo "==> Creating ack-system namespace"
kubectl create namespace ack-system --dry-run=client -o yaml | kubectl apply -f -

echo "==> Installing ACK S3 controller (v${ACK_S3_CHART_VERSION})"
helm install ack-s3-controller \
  oci://public.ecr.aws/aws-controllers-k8s/s3-chart \
  --version "${ACK_S3_CHART_VERSION}" \
  --namespace ack-system \
  --set aws.region="${AWS_REGION}" \
  --wait

echo "==> Installing ack-event-publisher"
PUBLISHER_ARGS=(
  oci://ghcr.io/cfairweather/charts/ack-event-publisher
  --namespace ack-system
  --set log.level=debug
  --wait
)
if [[ -n "${PUBLISHER_CHART_VERSION}" ]]; then
  PUBLISHER_ARGS+=(--version "${PUBLISHER_CHART_VERSION}")
fi
helm install ack-event-publisher "${PUBLISHER_ARGS[@]}"

echo ""
echo "✓ Setup complete. Run 'make test-run' to deploy test resources."
