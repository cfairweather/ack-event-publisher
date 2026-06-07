# ack-event-publisher — End-to-End Tests

Verifies that `ack-event-publisher` correctly publishes Kubernetes Events when
ACK resource status conditions change. Uses an EKS Auto Mode cluster and the
ACK S3 controller as the test subject.

## Prerequisites

| Tool | Minimum version |
|---|---|
| `aws` CLI | v2 |
| `helm` | 3.8 (OCI support) |
| `kubectl` | matching cluster version |
| `envsubst` | any (part of `gettext`) |

AWS credentials must have permissions to create:
- CloudFormation stacks
- EKS clusters, add-ons, pod identity associations
- VPCs, subnets, internet gateways, NAT gateways
- IAM roles and policies
- S3 buckets (for the test ACK resource)

## Deployment layers

The test infrastructure is split into three discrete layers to work around
the bootstrap dependency between AWS infrastructure and Kubernetes tooling.

```
Layer 1 — CloudFormation (no Kubernetes prereqs)
│
│  cfn/cluster.yaml deploys:
│    • VPC with public + private subnets across 2 AZs
│    • EKS Auto Mode cluster (manages its own nodes)
│    • eks-pod-identity-agent add-on
│    • IAM role for the ACK S3 controller (Pod Identity trust)
│    • EKS Pod Identity Association linking the role to
│      ack-system/ack-s3-controller ServiceAccount
│
▼
Layer 2 — Helm (requires cluster from Layer 1)
│
│  scripts/setup.sh installs:
│    • ACK S3 controller  (public.ecr.aws/aws-controllers-k8s/s3-chart)
│    • ack-event-publisher (ghcr.io/cfairweather/charts/ack-event-publisher)
│
▼
Layer 3 — kubectl (requires controllers from Layer 2)
│
│  scripts/verify.sh:
│    • Applies a test Bucket resource
│    • Polls kubectl get events until ResourceSynced + ResourceReady appear
│    • Prints the full event timeline and exits 0 on pass, 1 on fail
```

## Running the tests

Two methods are available: **native** (tools installed locally) or **Docker** (no local tooling required beyond Docker and AWS credentials).

### Docker method (recommended)

Requires only Docker and AWS credentials in `~/.aws`. All tools (aws CLI, helm, kubectl) run inside the container.

```bash
cd test/e2e

# Build the test image once
make docker-build-test

# Deploy AWS infrastructure
make docker-test-infra AWS_REGION=us-east-1
make docker-test-infra-wait AWS_REGION=us-east-1

# Install Helm charts
make docker-test-setup AWS_REGION=us-east-1

# Run the test
make docker-test-run AWS_REGION=us-east-1

# Or run everything in one shot
make docker-test-all AWS_REGION=us-east-1
```

Cleanup:

```bash
make docker-test-clean AWS_REGION=us-east-1           # remove k8s resources only
make docker-test-infra-destroy AWS_REGION=us-east-1   # delete cluster + VPC
```

### Native method

### Step 1 — Deploy AWS infrastructure

```bash
cd test/e2e

# Deploy the CloudFormation stack and wait for completion (~15 min for EKS)
make test-infra AWS_REGION=us-east-1
make test-infra-wait AWS_REGION=us-east-1
```

The stack name defaults to `ack-event-publisher-test`. Override with
`STACK_NAME=my-stack`.

### Step 2 — Install Helm charts

```bash
make test-setup AWS_REGION=us-east-1
```

This configures `kubectl`, creates the `ack-system` namespace, and installs
both the ACK S3 controller and `ack-event-publisher` with Helm. The ACK
controller picks up its IAM role automatically via EKS Pod Identity — no
annotation or IRSA configuration needed.

### Step 3 — Run the test

```bash
make test-run AWS_REGION=us-east-1
```

Applies a test `Bucket` resource and waits up to 5 minutes for events. On
success you will see output similar to:

```
Events published by ack-event-publisher:
LAST SEEN   TYPE      REASON           OBJECT                         MESSAGE
4s          Normal    ResourceSynced   Bucket/ack-event-publisher-test   ACK.ResourceSynced condition is True
4s          Normal    ResourceReady    Bucket/ack-event-publisher-test   Ready condition is True

✓ PASS: ResourceSynced and ResourceReady events observed.
```

You can also inspect events directly on the resource:

```bash
kubectl describe bucket ack-event-publisher-test
```

### Cleanup

```bash
# Remove test k8s resources and Helm releases (cluster stays up)
make test-clean AWS_REGION=us-east-1

# Delete the CloudFormation stack (cluster + VPC + IAM)
make test-infra-destroy AWS_REGION=us-east-1
```

### Full run (CI)

```bash
make test-all AWS_REGION=us-east-1
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `STACK_NAME` | `ack-event-publisher-test` | CloudFormation stack name and EKS cluster name |
| `AWS_REGION` | `us-east-1` | AWS region |
| `ACK_S3_CHART_VERSION` | `1.6.0` | ACK S3 Helm chart version |
| `PUBLISHER_CHART_VERSION` | _(latest)_ | ack-event-publisher chart version |
| `EVENT_TIMEOUT` | `300` | Seconds to wait for events before failing |

## Cost note

The test cluster uses EKS Auto Mode which provisions EC2 instances on demand.
Remember to run `make test-infra-destroy` when done to avoid ongoing charges.
Approximate cost for a short test run: EKS cluster ($0.10/hr) + 1–2 small
EC2 instances + NAT Gateway.
