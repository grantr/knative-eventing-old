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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	feedsv1alpha1 "github.com/knative/eventing/pkg/apis/feeds/v1alpha1"
	controllertesting "github.com/knative/eventing/pkg/controller/testing"
	"github.com/knative/eventing/pkg/sources"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakekubeclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
)

/*
TODO
- initial: new bind with or without status, no job
  reconciled: new bind with status unknown, job created, bind has job ref

- initial: bind with job not completed
  reconciled: same

- initial: bind with job completed
  reconciled: bind success and context, finalizer, job ref

- initial: bind with job failure
  reconciled: bind failure, job ref, job exists, finalizer

- initial: bind with job deadline exceeded
  reconciled: bind failure, job ref, job exists, finalizer

- initial: bind with deletionTimestamp
  reconciled: bind same, job created, job ref, job exists, finalizer

- initial: bind with deletionTimestamp, job completed
  reconciled: bind has no finalizers, job deleted

*/

var (
	trueVal  = true
	falseVal = false
)

func init() {
	// Add types to scheme
	feedsv1alpha1.AddToScheme(scheme.Scheme)
	servingv1alpha1.AddToScheme(scheme.Scheme)
}

func getEventSource() *feedsv1alpha1.EventSource {
	return &feedsv1alpha1.EventSource{
		ObjectMeta: om("test", "test-es"),
		Spec: feedsv1alpha1.EventSourceSpec{
			Source:     "github",
			Image:      "example.com/test-es-binder",
			Parameters: nil,
		},
	}
}

func getEventType() *feedsv1alpha1.EventType {
	return &feedsv1alpha1.EventType{
		ObjectMeta: om("test", "test-et"),
		Spec: feedsv1alpha1.EventTypeSpec{
			EventSource: "test-es",
		},
	}
}

func getBind() *feedsv1alpha1.Bind {
	return &feedsv1alpha1.Bind{
		ObjectMeta: om("test", "test-bind"),
		Spec: feedsv1alpha1.BindSpec{
			Action: feedsv1alpha1.BindAction{
				RouteName: "test-route",
			},
			Trigger: feedsv1alpha1.EventTrigger{
				EventType:      "test-et",
				Resource:       "",
				Service:        "",
				Parameters:     nil,
				ParametersFrom: nil,
			},
		},
	}
}

func getRoute() *servingv1alpha1.Route {
	return &servingv1alpha1.Route{
		ObjectMeta: om("test", "test-route"),
		Spec:       servingv1alpha1.RouteSpec{},
		Status: servingv1alpha1.RouteStatus{
			Domain: "example.com",
		},
	}
}

func getBindJob() *batchv1.Job {
	return &batchv1.Job{
		TypeMeta: jobType(),
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      "test-bind-bind",
			Labels:    map[string]string{"app": "bindpod"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         feedsv1alpha1.SchemeGroupVersion.String(),
				Kind:               "Bind",
				Name:               "test-bind",
				Controller:         &trueVal,
				BlockOwnerDeletion: &trueVal,
			}},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"sidecar.istio.io/inject": "false"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "binder",
						Image: "example.com/test-es-binder",
						Env: []corev1.EnvVar{{
							Name:  "BIND_OPERATION",
							Value: "BIND",
						}, {
							Name:  "BIND_TARGET",
							Value: "example.com",
						}, {
							Name: "BIND_TRIGGER",
							Value: base64.StdEncoding.EncodeToString(bytesOrDie(json.Marshal(
								sources.EventTrigger{
									EventType:  "test-et",
									Parameters: map[string]interface{}{},
								},
							))),
						}, {
							Name:  "BIND_CONTEXT",
							Value: "",
						}, {
							Name:  "EVENT_SOURCE_PARAMETERS",
							Value: "",
						}, {
							Name: "BIND_NAMESPACE",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "metadata.namespace",
								},
							},
						}, {
							Name: "BIND_SERVICE_ACCOUNT",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "spec.serviceAccountName",
								},
							},
						}},
						ImagePullPolicy: corev1.PullAlways,
					}},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
		Status: batchv1.JobStatus{},
	}
}

var testCases = []controllertesting.TestCase{
	{
		Name: "new bind gets status and job",
		InitialState: []runtime.Object{
			getEventSource(),
			getEventType(),
			getRoute(),
			getBind(),
		},
		ReconcileKey: "test/test-bind",
		WantPresent: []runtime.Object{
			&feedsv1alpha1.Bind{
				TypeMeta: bindType(),
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test",
					Name:      "test-bind",
					SelfLink:  "/apis/eventing/v1alpha1/namespaces/test/object/test-bind",
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion:         feedsv1alpha1.SchemeGroupVersion.String(),
						Kind:               "EventType",
						Name:               "test-et",
						Controller:         &falseVal,
						BlockOwnerDeletion: &trueVal,
					}},
					Finalizers: []string{finalizerName},
				},
				Spec: feedsv1alpha1.BindSpec{
					Action: feedsv1alpha1.BindAction{
						RouteName: "test-route",
					},
					Trigger: feedsv1alpha1.EventTrigger{
						EventType:      "test-et",
						Resource:       "",
						Service:        "",
						Parameters:     nil,
						ParametersFrom: nil,
					},
				},
				Status: feedsv1alpha1.BindStatus{
					Conditions: []feedsv1alpha1.BindCondition{{
						Type:   feedsv1alpha1.BindFailed,
						Status: corev1.ConditionUnknown,
					}, {
						Type:   feedsv1alpha1.BindInvalid,
						Status: corev1.ConditionUnknown,
					}, {
						Type:    feedsv1alpha1.BindComplete,
						Status:  corev1.ConditionUnknown,
						Reason:  "BindJob",
						Message: "Bind job in progress",
					}},
				},
			},
			getBindJob(),
		},
	},
}

func TestAllCases(t *testing.T) {
	kubeclient := fakekubeclient.NewSimpleClientset()
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

func om(namespace, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Namespace: namespace,
		Name:      name,
		SelfLink:  fmt.Sprintf("/apis/eventing/v1alpha1/namespaces/%s/object/%s", namespace, name),
	}
}

func bytesOrDie(v []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return v
}

func bindType() metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: feedsv1alpha1.SchemeGroupVersion.String(),
		Kind:       "Bind",
	}
}

func jobType() metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: batchv1.SchemeGroupVersion.String(),
		Kind:       "Job",
	}
}
