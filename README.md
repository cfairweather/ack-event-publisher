# ack-event-publisher

A companion Kubernetes controller that watches all [AWS Controllers for Kubernetes (ACK)](https://aws-controllers-k8s.github.io/community/) managed resources and publishes standard Kubernetes Events on status condition transitions.

DevOps teams familiar with `kubectl describe pod` expect to see an event timeline. ACK controllers communicate reconciliation state exclusively through `.status.conditions`, leaving `kubectl describe <ack-resource>` with an empty Events section. This controller fills that gap — no changes to existing ACK controllers required.

```
$ kubectl describe s3.s3.services.k8s.aws my-bucket

...
Events:
  Type     Reason           Age   From                  Message
  ----     ------           ----  ----                  -------
  Normal   ResourceSynced   2m    ack-event-publisher   ACK.ResourceSynced condition is True
  Normal   ResourceReady    2m    ack-event-publisher   Ready condition is True
  Warning  TerminalError    5m    ack-event-publisher   InvalidBucketName: bucket name must be...
```

## How it works

1. On startup, lists all CRDs whose API group ends in `.services.k8s.aws` (the ACK group naming convention).
2. Registers a dynamic informer for every served version of each ACK CRD.
3. On every add or update, diffs `.status.conditions[]` against a per-resource in-memory cache and publishes one Kubernetes Event per genuine condition transition.
4. Re-discovers CRDs on a configurable interval to pick up newly installed ACK service controllers without a restart.

## Condition → Event mapping

| ACK Condition | Event Type | Reason (True / False) |
|---|---|---|
| `ACK.ResourceSynced` | Normal / Warning | `ResourceSynced` / `SyncFailed` |
| `Ready` | Normal / Warning | `ResourceReady` / `ResourceNotReady` |
| `ACK.Terminal` | Warning / Normal | `TerminalError` / `TerminalCleared` |
| `ACK.Recoverable` | Warning / Normal | `RecoverableError` / `RecoverableCleared` |
| `ACK.Advisory` | Normal | `Advisory` |
| `ACK.LateInitialized` | Normal | `LateInitialized` |
| `ACK.ReferencesResolved` | Normal / Warning | `ReferencesResolved` / `ReferenceUnresolved` |
| `ACK.Adopted` | Normal / Warning | `ResourceAdopted` / `AdoptionFailed` |

## Artifacts

| Artifact | Location |
|---|---|
| Container image | `ghcr.io/cfairweather/ack-event-publisher:latest` |
| Helm chart (OCI) | `ghcr.io/cfairweather/charts/ack-event-publisher` |

Images and charts are published automatically on every push to `main`. Each build produces:
- A `sha-<short-commit>` image tag for pinned deployments
- A `latest` image tag tracking the most recent build
- A Helm chart version `0.1.0-<short-commit>`

## Installation

### Helm from OCI registry (recommended)

```bash
# Install latest build
helm install ack-event-publisher \
  oci://ghcr.io/cfairweather/charts/ack-event-publisher \
  --namespace ack-system \
  --create-namespace
```

Pin to a specific build:

```bash
helm install ack-event-publisher \
  oci://ghcr.io/cfairweather/charts/ack-event-publisher \
  --version 0.1.0-<short-sha> \
  --namespace ack-system \
  --create-namespace
```

Watch a single namespace only:

```bash
helm install ack-event-publisher \
  oci://ghcr.io/cfairweather/charts/ack-event-publisher \
  --namespace ack-system \
  --set watchNamespace=my-app
```

Enable debug logging:

```bash
helm install ack-event-publisher \
  oci://ghcr.io/cfairweather/charts/ack-event-publisher \
  --namespace ack-system \
  --set log.level=debug
```

### From source

```bash
helm install ack-event-publisher ./helm \
  --namespace ack-system \
  --create-namespace
```

### Verify

```bash
# Controller pod is running
kubectl get pods -n ack-system -l app.kubernetes.io/name=ack-event-publisher

# View all events from this controller across all namespaces
kubectl get events -A --field-selector reportingComponent=ack-event-publisher
```

## Configuration

| Flag | Default | Description |
|---|---|---|
| `--log-level` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `--enable-development-logging` | `false` | Zap development encoder (colorised console, debug threshold) |
| `--watch-namespace` | `""` | Namespace to watch; empty = all namespaces |
| `--resync-period` | `10m` | Interval for CRD re-discovery |
| `--metrics-bind-address` | `:8080` | Prometheus metrics endpoint |
| `--health-probe-bind-address` | `:8081` | `/healthz` and `/readyz` endpoints |
| `--leader-elect` | `false` | Enable leader election for HA |
| `--leader-election-namespace` | `""` | Namespace for leader election lease |
| `--kubeconfig` | `""` | Kubeconfig path (out-of-cluster dev only) |

All flags can also be set via environment variables prefixed with `ACK_EVENT_PUBLISHER_`
(e.g. `ACK_EVENT_PUBLISHER_LOG_LEVEL=debug`).

## Testing

End-to-end tests verify event publishing against a real EKS cluster using the
ACK S3 controller as the test subject. See **[test/e2e/README.md](test/e2e/README.md)**
for full instructions.

Quick start:

```bash
cd test/e2e
make test-infra AWS_REGION=us-east-1   # deploy EKS Auto Mode cluster via CloudFormation
make test-infra-wait AWS_REGION=us-east-1
make test-setup AWS_REGION=us-east-1   # install ACK S3 + ack-event-publisher via Helm
make test-run AWS_REGION=us-east-1     # apply test Bucket resource and verify events
```

## Building

```bash
# Fetch dependencies
go mod tidy

# Build binary
make build

# Run tests
make test

# Build container image
make docker-build IMG=<your-registry>/ack-event-publisher:latest
```

## Design notes

This controller is intentionally designed to match ACK runtime conventions to support upstream adoption:

- Uses `sigs.k8s.io/controller-runtime` directly (no Kubebuilder operator-sdk dependency), matching the ACK runtime.
- Logging via `go.uber.org/zap` through `github.com/go-logr/logr`, set up identically to `ackcfg.Config.SetupLogger()`.
- Module path `github.com/aws-controllers-k8s/ack-event-publisher` signals intent for an upstream home.
- Helm chart structure mirrors ACK service controller charts generated by `aws-controllers-k8s/code-generator`.
- ACK does not currently use `record.EventRecorder` — this controller introduces that capability as a standalone proof-of-concept ahead of a proposed Stage 2 integration into the ACK runtime library.

See [`.ai/REQUIREMENTS.md`](.ai/REQUIREMENTS.md) for the full design specification.

## License

Apache License 2.0. See [LICENSE](LICENSE).

---

## AI Disclosure

This project was developed with the assistance of [Claude Code](https://claude.ai/code) (Anthropic). The design, architecture, and ACK convention research were produced through an AI-assisted workflow:

- ACK codebase analysis (framework, logging patterns, CRD conventions, Helm structure) was performed by AI research agents.
- All code, Helm templates, and documentation were generated by AI and reviewed by the project author before commit.
- The requirements document at [`.ai/REQUIREMENTS.md`](.ai/REQUIREMENTS.md) was produced by the AI based on that research and approved by the author prior to implementation.

Human review and judgment guided all architectural decisions. AI-generated content does not imply endorsement by Anthropic or the AWS Controllers for Kubernetes project.
