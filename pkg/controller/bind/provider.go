/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bind

import (
	feedsv1alpha1 "github.com/knative/eventing/pkg/apis/feeds/v1alpha1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerAgentName = "bind-controller"

type reconciler struct {
	client   client.Client
	recorder record.EventRecorder
	// kubeclient is passed to ContainerEventSource to avoid leaking the
	// controller-runtime client (for now).
	kubeclient kubernetes.Interface
}

// Verify the struct implements reconcile.Reconciler
var _ reconcile.Reconciler = &reconciler{}

// ProvideController returns a bind controller.
func ProvideController(mrg manager.Manager) (controller.Controller, error) {
	// This client is passed to ContainerEventSource to avoid leaking the
	// controller-runtime client. This may be revisited if we decide to use
	// controller-runtime everywhere.
	kubeclient, err := kubernetes.NewForConfig(mrg.GetConfig())
	if err != nil {
		return nil, err
	}

	// Setup a new controller to Reconcile Routes
	c, err := controller.New(controllerAgentName, mrg, controller.Options{
		Reconciler: &reconciler{
			client:     mrg.GetClient(),
			recorder:   mrg.GetRecorder(controllerAgentName),
			kubeclient: kubeclient,
		},
	})
	if err != nil {
		return nil, err
	}

	// Watch ReplicaSets and enqueue ReplicaSet object key
	if err := c.Watch(&source.Kind{Type: &feedsv1alpha1.Bind{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return nil, err
	}

	return c, nil
}
