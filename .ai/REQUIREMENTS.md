# ACK Event Publisher — Requirements

## Overview

`ack-event-publisher` is a companion Kubernetes controller that watches all
AWS Controllers for Kubernetes (ACK) managed resources and publishes standard
Kubernetes Events when their status conditions change. This gives DevOps
personnel a familiar `kubectl describe` / `kubectl get events` workflow for
reconciliation status, rather than having to parse potentially large YAML
`.status` fields.

---

## Background & Motivation

ACK controllers manage AWS resources from Kubernetes. They communicate state
entirely through:

1. `.status.conditions[]` — structured condition entries on each managed resource
2. Structured log output — visible only to cluster operators with log access
3. Prometheus metrics — require additional monitoring stack

**What is missing**: Kubernetes Events. `kubectl describe <ack-resource>` shows
no events. DevOps teams accustomed to seeing event timelines on native
Kubernetes objects (Pods, Deployments) have no equivalent for ACK resources.
This project fills that gap without modifying any existing ACK controller.

---

## Goals

1. Publish a Kubernetes Event for every meaningful ACK status condition
   transition on any ACK-managed resource, across all installed ACK service
   controllers.
2. Remain a completely standalone, zero-dependency companion — no changes to
   existing ACK controllers required for Stage 1.
3. Match ACK runtime conventions (framework, logging, Helm structure) to
   maximize the probability of upstream adoption into `aws-controllers-k8s`
   (Stage 2).
4. Provide a production-ready Helm chart for easy deployment.

---

## Non-Goals

- Replacing or duplicating Prometheus metrics.
- Forwarding events to external sinks (Slack, PagerDuty, etc.) — that is the
  role of tools like `kubernetes-event-exporter`.
- Managing AWS resources or calling any AWS APIs.
- Requiring changes to the ACK runtime library.

---

## Alignment with ACK Conventions

The following decisions are made deliberately to match the ACK codebase so this
project can be proposed as an upstream contribution.

| Dimension | Decision | Rationale |
|---|---|---|
| Controller framework | `sigs.k8s.io/controller-runtime` directly | ACK runtime uses controller-runtime; no Kubebuilder operator-sdk dependency |
| Logger backend | `go.uber.org/zap` via `github.com/go-logr/logr` | Matches `runtime/pkg/config.SetupLogger()` exactly |
| Logger setup | `zap.New(zap.UseFlagOptions(&opts))` → `ctrlrt.SetLogger()` + `klog.SetLogger()` | Identical to ACK runtime pattern |
| Structured log fields | `"kind"`, `"namespace"`, `"name"` on every resource-scoped log line | ACK `ackrtlog.NewResourceLogger` convention |
| Log level flag | `--log-level` (default `"info"`) | Matches `ackcfg.Config.LogLevel` |
| Debug mode flag | `--enable-development-logging` (bool) | Mirrors `ackcfg.Config.EnableDevelopmentLogging`; enables zap development encoder |
| Go version | `go 1.25` | Current ACK module baseline |
| Module name | `github.com/aws-controllers-k8s/ack-event-publisher` | Consistent with ACK naming; signals intent for upstream home |
| Condition types | `ackv1alpha1.ConditionType` constants from `github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1` | Re-use upstream type definitions |

---

## CRD Discovery

ACK does not label CRDs with a `services.k8s.aws/controller-name` label.
ACK service identity is encoded in the **API group**, which always follows the
pattern `<service>.services.k8s.aws` (e.g., `s3.services.k8s.aws`,
`dynamodb.services.k8s.aws`).

**Discovery strategy**: On startup, list all `CustomResourceDefinitions` from
the `apiextensions.k8s.io/v1` API and filter to those whose `spec.group` ends
in `.services.k8s.aws`. For each matching CRD, register a dynamic informer for
every stored version. Re-run discovery on a configurable resync interval (default
10 minutes) to pick up newly installed ACK service controllers without a restart.

This approach:
- Requires only `apiextensions.k8s.io` list/watch permission — no per-service
  CRD awareness baked into the binary.
- Automatically handles multi-version CRDs.
- Works with any combination of installed ACK services.

---

## Event Publishing Logic

### Condition → Event Mapping

For each ACK resource, the controller tracks the last-seen state of each
condition. When a condition transitions (type + status + reason combination
changes), it emits one Kubernetes Event.

| ACK Condition | `corev1.EventType` | Default Reason |
|---|---|---|
| `ACK.ResourceSynced` = True | `Normal` | `ResourceSynced` |
| `ACK.ResourceSynced` = False | `Warning` | `SyncFailed` |
| `Ready` = True | `Normal` | `ResourceReady` |
| `Ready` = False | `Warning` | `ResourceNotReady` |
| `ACK.Terminal` = True | `Warning` | `TerminalError` |
| `ACK.Terminal` = False (cleared) | `Normal` | `TerminalCleared` |
| `ACK.Recoverable` = True | `Warning` | `RecoverableError` |
| `ACK.Advisory` = True | `Normal` | `Advisory` |
| `ACK.LateInitialized` = True | `Normal` | `LateInitialized` |
| `ACK.ReferencesResolved` = False | `Warning` | `ReferenceUnresolved` |
| `ACK.ReferencesResolved` = True | `Normal` | `ReferencesResolved` |
| `ACK.Adopted` = True | `Normal` | `ResourceAdopted` |
| (any other condition) | `Normal` / `Warning` based on status | condition type string |

Event `Message` is populated from `condition.Message` when present, falling
back to a generated string such as `"ACK.Terminal condition is True"`.

Event `InvolvedObject` references the ACK resource directly (GVK + namespace/name + UID).

### Deduplication

- Events are emitted via `k8s.io/client-go/tools/record.EventRecorder`, which
  handles aggregation (count + last-seen timestamp) for identical events
  automatically.
- The controller maintains an in-memory state map (resource UID → condition
  snapshot) to suppress re-publishing conditions that have not changed across
  resync cycles. Only genuine transitions trigger new events.

### Event Source

`EventSource.Component` = `"ack-event-publisher"` on all events.

---

## Logging Specification

### Standard (info) level

- Controller startup: Go version, module version, watch namespace config.
- CRD discovery: number of ACK CRDs found, list of group/resource pairs.
- Informer registration: one log line per CRD version registered.
- Event published: `kind`, `namespace`, `name`, `conditionType`, `status`,
  `eventType`, `reason`.

### Debug level (`--log-level=debug` or `--enable-development-logging`)

- Every object reconcile entry and exit (Trace pattern matching ACK's
  `rlog.Trace("reconcile"); defer exit(err)`).
- Full condition snapshot logged on each reconcile.
- Cache state transitions: which objects are being watched, added, updated,
  deleted.
- Event suppression decisions: why a transition was not published (e.g.,
  "condition unchanged since last observation").

Log lines at debug level include all structured fields:
`"gvk"`, `"namespace"`, `"name"`, `"uid"`, `"conditionType"`,
`"previousStatus"`, `"currentStatus"`.

---

## Configuration Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--log-level` | string | `"info"` | Zap log level: debug, info, warn, error |
| `--enable-development-logging` | bool | `false` | Zap development encoder (colored console output, debug threshold) |
| `--watch-namespace` | string | `""` (all namespaces) | Restrict watching to a single namespace |
| `--resync-period` | duration | `10m` | How often to re-discover ACK CRDs and resync informers |
| `--metrics-bind-address` | string | `":8080"` | Address for Prometheus metrics endpoint |
| `--health-probe-bind-address` | string | `":8081"` | Address for `/healthz` and `/readyz` endpoints |
| `--leader-elect` | bool | `false` | Enable leader election for HA deployments |
| `--leader-election-namespace` | string | `""` | Namespace for leader election lease object |
| `--kubeconfig` | string | `""` | Path to kubeconfig (out-of-cluster dev use only) |

All flags are also settable via environment variables using the prefix
`ACK_EVENT_PUBLISHER_` (e.g., `ACK_EVENT_PUBLISHER_LOG_LEVEL=debug`).

---

## RBAC Requirements

```yaml
# Cluster-scoped: discover ACK CRDs
- apiGroups: [apiextensions.k8s.io]
  resources: [customresourcedefinitions]
  verbs: [get, list, watch]

# Dynamic watch of all ACK resources (*.services.k8s.aws)
# Added programmatically at runtime for each discovered GVR
- apiGroups: ["*.services.k8s.aws"]
  resources: ["*"]
  verbs: [get, list, watch]

# Emit events on ACK resources
- apiGroups: [""]
  resources: [events]
  verbs: [create, patch, update]

# Leader election
- apiGroups: ["coordination.k8s.io"]
  resources: [leases]
  verbs: [create, get, list, patch, update, watch]
```

Note: Because ACK API groups are not known at deploy time, the Helm chart
ClusterRole will use a wildcard rule `["*.services.k8s.aws"]` with a comment
explaining why.

---

## Helm Chart Structure

Follows the ACK service controller chart layout exactly:

```
helm/
├── Chart.yaml
├── values.yaml
├── values.schema.json
└── templates/
    ├── NOTES.txt
    ├── _helpers.tpl
    ├── cluster-role.yaml
    ├── cluster-role-binding.yaml
    ├── leader-election-role.yaml
    ├── leader-election-role-binding.yaml
    ├── deployment.yaml
    ├── service-account.yaml
    └── metrics-service.yaml
```

Key `values.yaml` top-level structure mirrors ACK service controllers:

```yaml
image:
  repository: ghcr.io/aws-controllers-k8s/ack-event-publisher
  tag: ""           # defaults to Chart.AppVersion
  pullPolicy: IfNotPresent

deployment:
  replicas: 1
  resources:
    requests: {memory: 64Mi, cpu: 50m}
    limits:   {memory: 128Mi, cpu: 100m}

serviceAccount:
  create: true
  name: ack-event-publisher
  annotations: {}   # for EKS IRSA if needed

log:
  enable_development_logging: false
  level: info

resyncPeriod: 10m
watchNamespace: ""

leaderElection:
  enabled: false
  namespace: ""

metrics:
  service:
    create: false
    port: 8080
```

Deployment security context: non-root UID/GID (65532), read-only root
filesystem, all Linux capabilities dropped — identical to ACK service
controllers.

---

## Repository Layout

```
ack-event-publisher/
├── .ai/
│   └── REQUIREMENTS.md          ← this file
├── cmd/
│   └── main.go                  ← entrypoint: flag parsing, manager setup
├── pkg/
│   ├── config/
│   │   └── config.go            ← Config struct, flag binding, SetupLogger()
│   ├── discovery/
│   │   └── discovery.go         ← CRD discovery by .services.k8s.aws group suffix
│   ├── informer/
│   │   └── manager.go           ← dynamic informer registration per GVR
│   ├── handler/
│   │   └── handler.go           ← OnAdd/OnUpdate/OnDelete, condition diffing, event emit
│   └── version/
│       └── version.go           ← build-time version variables
├── helm/
│   ├── Chart.yaml
│   ├── values.yaml
│   ├── values.schema.json
│   └── templates/
│       └── ...
├── LICENSE                       ← Apache 2.0
├── README.md                     ← includes AI disclosure section
├── Makefile
└── go.mod
```

---

## License

**Apache License 2.0** — the same license used by all ACK repositories. This is
a fully open license (OSI-approved, permissive) and is the correct choice for a
project targeting upstream adoption into `aws-controllers-k8s`.

---

## Stage 2: Upstream Integration Path

Once Stage 1 is stable, the following changes would be proposed to the ACK
`runtime` library:

1. Add an optional `EventRecorder` field to `resourceReconciler`.
2. Add a `pkg/event` package with condition-to-event mapping helpers using the
   same `ackv1alpha1.ConditionType` constants.
3. Have each service controller opt in via a `--enable-status-events` flag.
4. Deprecate the standalone `ack-event-publisher` in favor of the built-in
   implementation, with a migration guide.

The standalone companion approach for Stage 1 allows the event publishing logic
to be validated in production without touching the ACK runtime release cycle.
