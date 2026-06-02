// Package informer manages dynamic informers for ACK-managed resources.
// It registers informers for each discovered GroupVersionResource and supports
// periodic re-discovery to pick up newly installed ACK service controllers
// without requiring a controller restart.
package informer

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/aws-controllers-k8s/ack-event-publisher/pkg/discovery"
)

// EventHandler is the callback interface satisfied by pkg/handler.Handler.
type EventHandler interface {
	OnAdd(obj interface{}, isInitialList bool)
	OnUpdate(oldObj, newObj interface{})
	OnDelete(obj interface{})
}

// Manager owns a set of dynamic informers, one per GVR, and manages their
// lifecycle. Informers are added incrementally as new ACK CRDs are discovered.
type Manager struct {
	log            logr.Logger
	dynClient      dynamic.Interface
	watchNamespace string
	resyncPeriod   time.Duration
	handler        EventHandler

	mu      sync.Mutex
	watched map[schema.GroupVersionResource]struct{}
	factory dynamicinformer.DynamicSharedInformerFactory
}

// NewManager returns an uninitialised Manager. Call NewRunnable to integrate
// it with a controller-runtime Manager.
func NewManager(
	log logr.Logger,
	dynClient dynamic.Interface,
	watchNamespace string,
	resyncPeriod time.Duration,
	handler EventHandler,
) *Manager {
	return &Manager{
		log:            log,
		dynClient:      dynClient,
		watchNamespace: watchNamespace,
		resyncPeriod:   resyncPeriod,
		handler:        handler,
		watched:        make(map[schema.GroupVersionResource]struct{}),
	}
}

// runnable implements sigs.k8s.io/controller-runtime/pkg/manager.Runnable.
type runnable struct {
	mgr        *Manager
	discoverer *discovery.Discoverer
}

// NewRunnable returns a ctrl.Runnable that runs the informer manager and
// periodic re-discovery loop. Add it to a controller-runtime Manager via
// mgr.Add(infMgr.NewRunnable(disc, resyncPeriod)).
func (m *Manager) NewRunnable(disc *discovery.Discoverer, resyncPeriod time.Duration) *runnable {
	return &runnable{mgr: m, discoverer: disc}
}

// Start implements controller-runtime's Runnable interface. It performs
// initial CRD discovery, starts informers, and then re-discovers on the
// configured interval until the context is cancelled.
func (r *runnable) Start(ctx context.Context) error {
	log := r.mgr.log
	log.Info("informer manager starting")

	r.mgr.mu.Lock()
	r.mgr.factory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		r.mgr.dynClient,
		r.mgr.resyncPeriod,
		r.mgr.watchNamespace,
		nil,
	)
	r.mgr.mu.Unlock()

	if err := r.discover(ctx); err != nil {
		return err
	}

	r.mgr.mu.Lock()
	r.mgr.factory.Start(ctx.Done())
	r.mgr.mu.Unlock()

	ticker := time.NewTicker(r.mgr.resyncPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("informer manager stopping")
			return nil
		case <-ticker.C:
			log.V(1).Info("running periodic CRD re-discovery")
			if err := r.discover(ctx); err != nil {
				log.Error(err, "CRD re-discovery failed, will retry next cycle")
			}
		}
	}
}

// discover lists ACK CRDs and registers an informer for any GVR not already watched.
func (r *runnable) discover(ctx context.Context) error {
	gvrs, err := r.discoverer.Discover(ctx)
	if err != nil {
		return err
	}

	r.mgr.mu.Lock()
	defer r.mgr.mu.Unlock()

	for _, gvr := range gvrs {
		if _, ok := r.mgr.watched[gvr]; ok {
			r.mgr.log.V(1).Info("informer already registered, skipping",
				"gvr", gvr.String(),
			)
			continue
		}

		inf := r.mgr.factory.ForResource(gvr).Informer()
		if _, err := inf.AddEventHandler(cache.ResourceEventHandlerDetailedFuncs{
			AddFunc:    r.mgr.handler.OnAdd,
			UpdateFunc: r.mgr.handler.OnUpdate,
			DeleteFunc: r.mgr.handler.OnDelete,
		}); err != nil {
			r.mgr.log.Error(err, "failed to add event handler", "gvr", gvr.String())
			continue
		}

		r.mgr.watched[gvr] = struct{}{}
		r.mgr.log.Info("registered informer for ACK resource",
			"group", gvr.Group,
			"version", gvr.Version,
			"resource", gvr.Resource,
		)

		// If the factory is already started (re-discovery cycle), start the
		// new informer immediately.
		if r.mgr.factory != nil {
			r.mgr.factory.Start(ctx.Done())
		}
	}

	return nil
}
