#!/usr/bin/env bash
# Layer 3: Deploy a test S3 Bucket via ACK and verify that ack-event-publisher
# emits the expected Kubernetes Events.
#
# Usage:
#   AWS_REGION=us-east-1 ./verify.sh

set -euo pipefail

AWS_REGION="${AWS_REGION:-us-east-1}"
NAMESPACE="${NAMESPACE:-default}"
EVENT_TIMEOUT="${EVENT_TIMEOUT:-300}"   # seconds to wait for events
POLL_INTERVAL=5

# Derive a unique bucket name: account-id + region + short random suffix
ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
RANDOM_SUFFIX=$(tr -dc 'a-z0-9' < /dev/urandom | head -c 6 || true)
BUCKET_NAME="ack-test-${ACCOUNT_ID}-${AWS_REGION:0:3}-${RANDOM_SUFFIX}"
export BUCKET_NAME

echo "==> Deploying test S3 Bucket: ${BUCKET_NAME}"
# Substitute BUCKET_NAME into the manifest before applying
envsubst < "$(dirname "$0")/../k8s/s3-bucket.yaml" | kubectl apply -n "${NAMESPACE}" -f -

echo "==> Waiting up to ${EVENT_TIMEOUT}s for ack-event-publisher events..."
DEADLINE=$((SECONDS + EVENT_TIMEOUT))
FOUND_SYNCED=false
FOUND_READY=false

while [[ $SECONDS -lt $DEADLINE ]]; do
  EVENTS=$(kubectl get events \
    --namespace "${NAMESPACE}" \
    --field-selector "involvedObject.name=ack-event-publisher-test,reportingComponent=ack-event-publisher" \
    --no-headers 2>/dev/null || true)

  if echo "${EVENTS}" | grep -q "ResourceSynced"; then
    FOUND_SYNCED=true
  fi
  if echo "${EVENTS}" | grep -q "ResourceReady"; then
    FOUND_READY=true
  fi

  if $FOUND_SYNCED && $FOUND_READY; then
    break
  fi

  echo "  [$(date +%T)] waiting... (synced=${FOUND_SYNCED} ready=${FOUND_READY})"
  sleep "${POLL_INTERVAL}"
done

echo ""
echo "==> Events published by ack-event-publisher:"
kubectl get events \
  --namespace "${NAMESPACE}" \
  --field-selector "reportingComponent=ack-event-publisher" \
  --sort-by=".lastTimestamp"

echo ""
if $FOUND_SYNCED && $FOUND_READY; then
  echo "✓ PASS: ResourceSynced and ResourceReady events observed."
  exit 0
else
  echo "✗ FAIL: Expected events not observed within ${EVENT_TIMEOUT}s."
  echo ""
  echo "  ACK S3 controller logs:"
  kubectl logs -n ack-system -l app.kubernetes.io/name=ack-s3-controller --tail=30 || true
  echo ""
  echo "  ack-event-publisher logs:"
  kubectl logs -n ack-system -l app.kubernetes.io/name=ack-event-publisher --tail=30 || true
  exit 1
fi
