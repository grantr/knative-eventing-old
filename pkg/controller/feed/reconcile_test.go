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

package feed

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	feedsv1alpha1 "github.com/knative/eventing/pkg/apis/feeds/v1alpha1"
	"github.com/knative/eventing/pkg/controller/feed/resources"
	controllertesting "github.com/knative/eventing/pkg/controller/testing"
	"github.com/knative/eventing/pkg/sources"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
)

/*
TODO
- initial: bind with job deadline exceeded
  reconciled: bind failure, job exists, finalizer
*/

var (
	trueVal  = true
	falseVal = false
	// deletionTime is used when objects are marked as deleted. Rfc3339Copy()
	// truncates to seconds to match the loss of precision during serialization.
	deletionTime = metav1.Now().Rfc3339Copy()
)

func init() {
	// Add types to scheme
	feedsv1alpha1.AddToScheme(scheme.Scheme)
	servingv1alpha1.AddToScheme(scheme.Scheme)
}

var testCases = []controllertesting.TestCase{
	{
		Name: "new bind: adds status, finalizer, creates job",
		InitialState: []runtime.Object{
			getEventSource(),
			getEventType(),
			getRoute(),
			getNewBind(),
		},
		ReconcileKey: "test/test-bind",
		WantPresent: []runtime.Object{
			getBindInProgressBind(),
			getNewBindJob(),
		},
	},
	{
		Name: "in progress bind with existing job: both unchanged",
		InitialState: []runtime.Object{
			getEventSource(),
			getEventType(),
			getRoute(),
			getBindInProgressBind(),
			getNewBindJob(),
		},
		ReconcileKey: "test/test-bind",
		WantPresent: []runtime.Object{
			getBindInProgressBind(),
			getNewBindJob(),
		},
	},
	{
		Name: "in progress bind with completed job: updated status, context, job exists",
		InitialState: []runtime.Object{
			getEventSource(),
			getEventType(),
			getRoute(),
			getBindInProgressBind(),
			getCompletedBindJob(),
			getCompletedBindJobPod(),
		},
		ReconcileKey: "test/test-bind",
		WantPresent: []runtime.Object{
			getBoundBind(),
			getCompletedBindJob(),
		},
	},
	{
		Name: "in progress bind with failed job: updated status, job exists",
		InitialState: []runtime.Object{
			getEventSource(),
			getEventType(),
			getRoute(),
			getBindInProgressBind(),
			getFailedBindJob(),
			getCompletedBindJobPod(),
		},
		ReconcileKey: "test/test-bind",
		WantPresent: []runtime.Object{
			getBindFailedBind(),
			getFailedBindJob(),
		},
	},
	{
		Name: "Deleted bind with finalizer, previously completed, bind job exists: bind job deleted",
		InitialState: []runtime.Object{
			getEventSource(),
			getEventType(),
			getRoute(),
			getDeletedBoundBind(),
			getCompletedBindJob(),
		},
		ReconcileKey: "test/test-bind",
		WantPresent: []runtime.Object{
			getDeletedBoundBind(),
		},
		WantAbsent: []runtime.Object{
			getCompletedBindJob(),
		},
	},
	{
		Name: "Deleted bind with finalizer, previously completed, bind job missing: unbind job created, status updated",
		InitialState: []runtime.Object{
			getEventSource(),
			getEventType(),
			getRoute(),
			getDeletedBoundBind(),
		},
		ReconcileKey: "test/test-bind",
		WantPresent: []runtime.Object{
			getDeletedStopInProgressBind(),
			getNewStopJob(),
		},
	},
	{
		Name: "Deleted in-progress bind with finalizer, unbind job exists: unchanged",
		InitialState: []runtime.Object{
			getEventSource(),
			getEventType(),
			getRoute(),
			getDeletedStopInProgressBind(),
			getInProgressStopJob(),
		},
		ReconcileKey: "test/test-bind",
		WantPresent: []runtime.Object{
			getDeletedStopInProgressBind(),
			getInProgressStopJob(),
		},
	},
	{
		Name: "Deleted bind with completed unbind job: no finalizers, update status",
		InitialState: []runtime.Object{
			getEventSource(),
			getEventType(),
			getRoute(),
			getDeletedStopInProgressBind(),
			getCompletedStopJob(),
		},
		ReconcileKey: "test/test-bind",
		WantPresent: []runtime.Object{
			getDeletedUnboundBind(),
		},
	},
}

func TestAllCases(t *testing.T) {
	recorder := record.NewBroadcaster().NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	for _, tc := range testCases {
		r := &reconciler{
			client:   tc.GetClient(),
			recorder: recorder,
		}
		t.Run(tc.Name, tc.Runner(t, r, r.client))
	}
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
			EventSource: getEventSource().Name,
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

func getFeedContext() *sources.FeedContext {
	return &sources.FeedContext{
		Context: map[string]interface{}{
			"foo": "bar",
		},
	}
}

func getNewFeed() *feedsv1alpha1.Feed {
	return &feedsv1alpha1.Feed{
		TypeMeta:   bindType(),
		ObjectMeta: om("test", "test-bind"),
		Spec: feedsv1alpha1.FeedSpec{
			Action: feedsv1alpha1.FeedAction{
				RouteName: getRoute().Name,
			},
			Trigger: feedsv1alpha1.EventTrigger{
				EventType:      getEventType().Name,
				Resource:       "",
				Service:        "",
				Parameters:     nil,
				ParametersFrom: nil,
			},
		},
	}
}

func getStartInProgressFeed() *feedsv1alpha1.Feed {
	bind := getNewFeed()
	bind.SetOwnerReference(&metav1.OwnerReference{
		APIVersion:         feedsv1alpha1.SchemeGroupVersion.String(),
		Kind:               "EventType",
		Name:               getEventType().Name,
		Controller:         &falseVal,
		BlockOwnerDeletion: &trueVal,
	})
	bind.AddFinalizer(finalizerName)

	bind.Status.InitializeConditions()
	bind.Status.SetCondition(&feedsv1alpha1.BindCondition{
		Type:    feedsv1alpha1.FeedStarted,
		Status:  corev1.ConditionUnknown,
		Reason:  "StartJob",
		Message: "Start job in progress",
	})

	return bind
}

func getBoundFeed() *feedsv1alpha1.Feed {
	bind := getBindInProgressFeed()
	marshalledContext, err := json.Marshal(getFeedContext().Context)
	if err != nil {
		panic(err)
	}
	bind.Status.FeedContext = &runtime.RawExtension{
		Raw: marshalledContext,
	}
	bind.Status.SetCondition(&feedsv1alpha1.FeedCondition{
		Type:    feedsv1alpha1.FeedStarted,
		Status:  corev1.ConditionTrue,
		Reason:  "BindJobComplete",
		Message: "Bind job succeeded",
	})
	return bind
}

func getBindFailedFeed() *feedsv1alpha1.Feed {
	bind := getBindInProgressFeed()
	bind.Status.SetCondition(&feedsv1alpha1.FeedCondition{
		Type:    feedsv1alpha1.FeedFailed,
		Status:  corev1.ConditionTrue,
		Reason:  "BindJobFailed",
		Message: "TODO replace with job failure message",
	})
	return bind
}

func getDeletedBoundFeed() *feedsv1alpha1.Feed {
	bind := getBoundFeed()
	bind.SetDeletionTimestamp(&deletionTime)
	return bind
}

func getDeletedStopInProgressFeed() *feedsv1alpha1.Feed {
	bind := getDeletedBoundFeed()

	bind.Status.SetCondition(&feedsv1alpha1.FeedCondition{
		Type:    feedsv1alpha1.FeedStarted,
		Status:  corev1.ConditionUnknown,
		Reason:  "StopJob",
		Message: "Stop job in progress",
	})
	return bind
}

func getDeletedUnboundBind() *feedsv1alpha1.Bind {
	bind := getDeletedStopInProgressBind()
	bind.RemoveFinalizer(finalizerName)
	bind.Status.SetCondition(&feedsv1alpha1.BindCondition{
		Type:    feedsv1alpha1.FeedStarted,
		Status:  corev1.ConditionTrue,
		Reason:  "StopJobComplete",
		Message: "Stop job succeeded",
	})
	return bind
}

func getNewBindJob() *batchv1.Job {
	jobName := resources.BindJobName(getNewBind())
	return &batchv1.Job{
		TypeMeta: jobType(),
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      jobName,
			Labels:    map[string]string{"app": "bindpod"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         feedsv1alpha1.SchemeGroupVersion.String(),
				Kind:               "Bind",
				Name:               getNewBind().Name,
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
							Name:  string(resources.EnvVarOperation),
							Value: string(resources.OperationBind),
						}, {
							Name:  string(resources.EnvVarTarget),
							Value: "example.com",
						}, {
							Name: string(resources.EnvVarTrigger),
							Value: base64.StdEncoding.EncodeToString(bytesOrDie(json.Marshal(
								sources.EventTrigger{
									EventType:  "test-et",
									Parameters: map[string]interface{}{},
								},
							))),
						}, {
							Name:  string(resources.EnvVarContext),
							Value: "",
						}, {
							Name:  string(resources.EnvVarEventSourceParameters),
							Value: "",
						}, {
							Name: string(resources.EnvVarNamespace),
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "metadata.namespace",
								},
							},
						}, {
							Name: string(resources.EnvVarServiceAccount),
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

func getInProgressBindJob() *batchv1.Job {
	job := getNewBindJob()
	// This is normally set by a webhook. Set it here
	// to simulate. TODO use a reactor when that's
	// supported.
	job.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"job": job.Name,
		},
	}
	return job
}

func getCompletedBindJob() *batchv1.Job {
	job := getInProgressBindJob()
	job.Status = batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{
			Type:   batchv1.JobComplete,
			Status: corev1.ConditionTrue,
		}},
	}
	return job
}

func getFailedBindJob() *batchv1.Job {
	job := getInProgressBindJob()
	job.Status = batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{
			Type:   batchv1.JobFailed,
			Status: corev1.ConditionTrue,
		}},
	}
	return job
}

func getCompletedBindJobPod() *corev1.Pod {
	job := getCompletedBindJob()
	outputContext, err := json.Marshal(getBindContext())
	if err != nil {
		panic(err)
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test",
			Name:      job.Name,
			Labels:    map[string]string{"job": job.Name},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{{
				State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						Message: base64.StdEncoding.EncodeToString(outputContext),
					},
				},
			}},
		},
	}
}

func getNewStopJob() *batchv1.Job {
	job := getNewBindJob()
	job.Name = resources.StopJobName(getDeletedBoundBind())

	job.Spec.Template.Spec.Containers[0].Env = []corev1.EnvVar{{
		Name:  string(resources.EnvVarOperation),
		Value: string(resources.OperationStop),
	}, {
		Name:  string(resources.EnvVarTarget),
		Value: "example.com",
	}, {
		Name: string(resources.EnvVarTrigger),
		Value: base64.StdEncoding.EncodeToString(bytesOrDie(json.Marshal(
			sources.EventTrigger{
				EventType:  "test-et",
				Parameters: map[string]interface{}{},
			},
		))),
	}, {
		Name:  string(resources.EnvVarContext),
		Value: base64.StdEncoding.EncodeToString(bytesOrDie(json.Marshal(getBindContext()))),
	}, {
		Name:  string(resources.EnvVarEventSourceParameters),
		Value: "",
	}, {
		Name: string(resources.EnvVarNamespace),
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.namespace",
			},
		},
	}, {
		Name: string(resources.EnvVarServiceAccount),
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "spec.serviceAccountName",
			},
		},
	}}
	fmt.Printf("new unbind job: %#v", job)
	return job
}

func getInProgressStopJob() *batchv1.Job {
	job := getNewStopJob()
	// This is normally set by a webhook. Set it here
	// to simulate. TODO use a reactor when that's
	// supported.
	job.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"job": job.Name,
		},
	}
	return job
}

func getCompletedStopJob() *batchv1.Job {
	job := getInProgressStopJob()
	job.Status = batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{
			Type:   batchv1.JobComplete,
			Status: corev1.ConditionTrue,
		}},
	}
	return job
}

func getFailedStopJob() *batchv1.Job {
	job := getInProgressStopJob()
	job.Status = batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{
			Type:   batchv1.JobFailed,
			Status: corev1.ConditionTrue,
		}},
	}
	return job
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

func podType() metav1.TypeMeta {
	return metav1.TypeMeta{
		APIVersion: corev1.SchemeGroupVersion.String(),
		Kind:       "Pod",
	}
}