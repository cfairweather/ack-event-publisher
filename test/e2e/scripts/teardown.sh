#!/usr/bin/env bash
# Remove test Kubernetes resources and Helm releases.
# The CloudFormation stack is deleted separately via `make test-infra-destroy`
# to avoid accidentally deleting the cluster during a test iteration.
#
# Usage:
#   STACK_NAME=ack-event-publisher-test AWS_REGION=us-east-1 ./teardown.sh

set -euo pipefail

STACK_NAME="${STACK_NAME:-ack-event-publisher-test}"
AWS_REGION="${AWS_REGION:-us-east-1}"
NAMESPACE="${NAMESPACE:-default}"

echo "==> Removing test S3 Bucket resource"
kubectl delete -n "${NAMESPACE}" bucket ack-event-publisher-test --ignore-not-found

echo "==> Uninstalling ack-event-publisher"
helm uninstall ack-event-publisher --namespace ack-system --ignore-not-found 2>/dev/null || true

echo "==> Uninstalling ack-s3-controller"
helm uninstall ack-s3-controller --namespace ack-system --ignore-not-found 2>/dev/null || true

echo ""
echo "✓ Kubernetes resources removed."
echo "  To destroy the cluster and VPC run: make test-infra-destroy"
