// Package discovery locates all ACK-managed CRDs in the cluster by filtering
// on the API group suffix ".services.k8s.aws". ACK encodes service identity
// in the group (e.g. s3.services.k8s.aws, dynamodb.services.k8s.aws) rather
// than CRD labels, so group-suffix matching is the canonical discovery method.
package discovery

import (
	"context"
	"strings"

	"github.com/go-logr/logr"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const ackGroupSuffix = ".services.k8s.aws"

// Discoverer lists ACK CRDs from the API server.
type Discoverer struct {
	log    logr.Logger
	client apiextensionsclient.Interface
}

// New returns a Discoverer backed by the given apiextensions clientset.
func New(log logr.Logger, client apiextensionsclient.Interface) *Discoverer {
	return &Discoverer{log: log, client: client}
}

// Discover returns a deduplicated slice of GroupVersionResources for every
// version of every ACK-managed CRD currently installed in the cluster.
func (d *Discoverer) Discover(ctx context.Context) ([]schema.GroupVersionResource, error) {
	list, err := d.client.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var gvrs []schema.GroupVersionResource
	for _, crd := range list.Items {
		if !strings.HasSuffix(crd.Spec.Group, ackGroupSuffix) {
			continue
		}
		for _, v := range crd.Spec.Versions {
			if !v.Served {
				continue
			}
			gvr := schema.GroupVersionResource{
				Group:    crd.Spec.Group,
				Version:  v.Name,
				Resource: crd.Spec.Names.Plural,
			}
			gvrs = append(gvrs, gvr)
			d.log.Info("discovered ACK CRD",
				"group", gvr.Group,
				"version", gvr.Version,
				"resource", gvr.Resource,
			)
		}
	}

	d.log.Info("ACK CRD discovery complete", "count", len(gvrs))
	return gvrs, nil
}
