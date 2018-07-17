/*
Copyright 2018 The Knative Authors

Licensed under the Apache License, Veroute.on 2.0 (the "License");
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
	"testing"

	feedsv1alpha1 "github.com/knative/eventing/pkg/apis/feeds/v1alpha1"
	controllertesting "github.com/knative/eventing/pkg/controller/testing"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
)

/*
TODO

*/

func init() {
	// Add types to scheme
	feedsv1alpha1.AddToScheme(scheme.Scheme)
}

func getBind() *feedsv1alpha1.Bind {
	return &feedsv1alpha1.Bind{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "test",
		},
	}
}

var testCases = []controllertesting.TestCase{
	{
		Name:         "test",
		InitialState: []runtime.Object{getBind()},
		ReconcileKey: "test/test",
	},
}

func TestAllCases(t *testing.T) {
	kubeclient, err := kubernetes.NewForConfig(&rest.Config{})
	if err != nil {
		t.Fatal(err)
	}
	recorder := record.NewBroadcaster().NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	for _, tc := range testCases {
		r := &reconciler{
			client:     tc.GetClient(),
			recorder:   recorder,
			kubeclient: kubeclient,
		}
		t.Run(tc.Name, tc.Runner(t, r, r.client))
	}
}
