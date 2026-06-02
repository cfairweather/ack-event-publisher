// Package handler processes add/update/delete events from dynamic informers,
// diffs ACK status conditions against a per-resource state cache, and emits
// Kubernetes Events for every genuine condition transition.
package handler

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// ACK condition type constants. These mirror ackv1alpha1.ConditionType values
// from github.com/aws-controllers-k8s/runtime/apis/core/v1alpha1 and must
// stay in sync with upstream definitions.
const (
	conditionReady              = "Ready"
	conditionAdopted            = "ACK.Adopted"
	conditionResourceSynced     = "ACK.ResourceSynced"
	conditionTerminal           = "ACK.Terminal"
	conditionRecoverable        = "ACK.Recoverable"
	conditionAdvisory           = "ACK.Advisory"
	conditionLateInitialized    = "ACK.LateInitialized"
	conditionReferencesResolved = "ACK.ReferencesResolved"
)

// conditionKey is the tuple used to detect a genuine condition transition.
// A transition is detected when type+status+reason changes between observations.
type conditionKey struct {
	status string
	reason string
}

// resourceState tracks the last-published condition snapshot for one resource.
type resourceState map[string]conditionKey // conditionType -> key

// condition is a parsed representation of one entry in .status.conditions[].
type condition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

// Handler processes object lifecycle events from dynamic informers.
type Handler struct {
	log        logr.Logger
	kubeClient kubernetes.Interface
	hostname   string

	mu    sync.Mutex
	cache map[types.UID]resourceState
}

// New returns a Handler that emits Kubernetes Events via kubeClient.
func New(log logr.Logger, kubeClient kubernetes.Interface) *Handler {
	hostname, _ := os.Hostname()
	return &Handler{
		log:        log,
		kubeClient: kubeClient,
		hostname:   hostname,
		cache:      make(map[types.UID]resourceState),
	}
}

// OnAdd is called when a resource is first observed. Events are published for
// every condition already present so operators see current state immediately.
func (h *Handler) OnAdd(obj interface{}, isInitialList bool) {
	u, ok := toUnstructured(obj)
	if !ok {
		return
	}
	log := h.resourceLog(u)
	log.V(1).Info("object added")
	h.process(context.Background(), u, log)
}

// OnUpdate is called when a resource is modified. Only genuine condition
// transitions (type+status+reason changes) produce new events.
func (h *Handler) OnUpdate(_, newObj interface{}) {
	u, ok := toUnstructured(newObj)
	if !ok {
		return
	}
	log := h.resourceLog(u)
	log.V(1).Info("object updated")
	h.process(context.Background(), u, log)
}

// OnDelete removes the resource from the state cache.
func (h *Handler) OnDelete(obj interface{}) {
	u, ok := toUnstructured(obj)
	if !ok {
		// Handle tombstone from the informer cache.
		if d, ok := obj.(cache.DeletedFinalStateUnknown); ok {
			if u, ok = toUnstructured(d.Obj); !ok {
				return
			}
		} else {
			return
		}
	}
	log := h.resourceLog(u)
	log.V(1).Info("object deleted")

	h.mu.Lock()
	delete(h.cache, u.GetUID())
	h.mu.Unlock()
}

// process diffs the current condition set against the cached state and emits
// events for every transition detected.
func (h *Handler) process(ctx context.Context, u *unstructured.Unstructured, log logr.Logger) {
	conditions := extractConditions(u)

	h.mu.Lock()
	prev := h.cache[u.GetUID()]
	if prev == nil {
		prev = make(resourceState)
	}
	next := make(resourceState, len(conditions))
	for _, c := range conditions {
		next[c.Type] = conditionKey{status: c.Status, reason: c.Reason}
	}
	h.cache[u.GetUID()] = next
	h.mu.Unlock()

	for _, c := range conditions {
		old, seen := prev[c.Type]
		newKey := conditionKey{status: c.Status, reason: c.Reason}

		if seen && old == newKey {
			log.V(1).Info("condition unchanged, suppressing event",
				"conditionType", c.Type,
				"status", c.Status,
			)
			continue
		}

		eventType, reason := classify(c)
		msg := c.Message
		if msg == "" {
			msg = fmt.Sprintf("%s condition is %s", c.Type, c.Status)
		}

		log.Info("publishing event",
			"conditionType", c.Type,
			"status", c.Status,
			"eventType", eventType,
			"reason", reason,
		)
		h.emit(ctx, u, eventType, reason, msg)
	}
}

// emit creates a Kubernetes Event directly via the API server. Direct creation
// is used instead of record.EventRecorder because dynamic (unstructured) objects
// are not registered in the controller-runtime scheme and the recorder's
// ObjectReference builder would fail.
func (h *Handler) emit(ctx context.Context, u *unstructured.Unstructured, eventType, reason, message string) {
	ns := u.GetNamespace()
	eventNs := ns
	if eventNs == "" {
		// Cluster-scoped resources: events are placed in "default" by convention.
		eventNs = "default"
	}

	t := metav1.Now()
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s.", u.GetName()),
			Namespace:    eventNs,
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: u.GetAPIVersion(),
			Kind:       u.GetKind(),
			Name:       u.GetName(),
			Namespace:  ns,
			UID:        u.GetUID(),
		},
		Reason:             reason,
		Message:            message,
		Type:               eventType,
		FirstTimestamp:     t,
		LastTimestamp:      t,
		Count:              1,
		ReportingComponent: "ack-event-publisher",
		ReportingInstance:  h.hostname,
		Source: corev1.EventSource{
			Component: "ack-event-publisher",
			Host:      h.hostname,
		},
	}

	if _, err := h.kubeClient.CoreV1().Events(eventNs).Create(ctx, event, metav1.CreateOptions{}); err != nil {
		h.log.Error(err, "failed to create event",
			"kind", u.GetKind(),
			"namespace", ns,
			"name", u.GetName(),
		)
	}
}

// classify maps an ACK condition to a Kubernetes event type and reason string.
func classify(c condition) (eventType, reason string) {
	type mapping struct {
		eventType string
		trueRsn   string
		falseRsn  string
	}

	table := map[string]mapping{
		conditionResourceSynced:     {corev1.EventTypeNormal, "ResourceSynced", "SyncFailed"},
		conditionReady:              {corev1.EventTypeNormal, "ResourceReady", "ResourceNotReady"},
		conditionTerminal:           {corev1.EventTypeWarning, "TerminalError", "TerminalCleared"},
		conditionRecoverable:        {corev1.EventTypeWarning, "RecoverableError", "RecoverableCleared"},
		conditionAdvisory:           {corev1.EventTypeNormal, "Advisory", "AdvisoryCleared"},
		conditionLateInitialized:    {corev1.EventTypeNormal, "LateInitialized", "LateInitializedCleared"},
		conditionReferencesResolved: {corev1.EventTypeNormal, "ReferencesResolved", "ReferenceUnresolved"},
		conditionAdopted:            {corev1.EventTypeNormal, "ResourceAdopted", "AdoptionFailed"},
	}

	if m, ok := table[c.Type]; ok {
		if c.Status == "True" {
			return m.eventType, m.trueRsn
		}
		// For terminal/recoverable, False is always a Warning.
		if c.Type == conditionTerminal || c.Type == conditionRecoverable {
			return corev1.EventTypeNormal, m.falseRsn
		}
		return corev1.EventTypeWarning, m.falseRsn
	}

	// Unknown condition type: Warning when False, Normal when True.
	if c.Status == "False" {
		return corev1.EventTypeWarning, sanitizeReason(c.Type)
	}
	return corev1.EventTypeNormal, sanitizeReason(c.Type)
}

// extractConditions reads .status.conditions[] from an unstructured object.
func extractConditions(u *unstructured.Unstructured) []condition {
	raw, found, err := unstructured.NestedSlice(u.Object, "status", "conditions")
	if err != nil || !found {
		return nil
	}

	out := make([]condition, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		c := condition{
			Type:    stringField(m, "type"),
			Status:  stringField(m, "status"),
			Reason:  stringField(m, "reason"),
			Message: stringField(m, "message"),
		}
		if c.Type == "" || c.Status == "" {
			continue
		}
		out = append(out, c)
	}
	return out
}

func stringField(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

// sanitizeReason strips characters invalid in a Kubernetes event reason field.
func sanitizeReason(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		b := s[i]
		if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		return "UnknownCondition"
	}
	return string(out)
}

func (h *Handler) resourceLog(u *unstructured.Unstructured) logr.Logger {
	return h.log.WithValues(
		"kind", u.GetKind(),
		"namespace", u.GetNamespace(),
		"name", u.GetName(),
	)
}

func toUnstructured(obj interface{}) (*unstructured.Unstructured, bool) {
	u, ok := obj.(*unstructured.Unstructured)
	return u, ok
}
