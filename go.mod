module github.com/aws-controllers-k8s/ack-event-publisher

go 1.25

require (
	github.com/go-logr/logr v1.4.3
	go.uber.org/zap v1.27.0
	k8s.io/api v0.35.0
	k8s.io/apiextensions-apiserver v0.35.0
	k8s.io/apimachinery v0.35.0
	k8s.io/client-go v0.35.0
	k8s.io/klog/v2 v2.130.1
	sigs.k8s.io/controller-runtime v0.23.0
)
