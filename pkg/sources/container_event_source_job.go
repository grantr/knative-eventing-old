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

package sources

import (
	"encoding/base64"
	"encoding/json"

	v1alpha1 "github.com/knative/eventing/pkg/apis/feeds/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BindOperation specifies whether we're binding or unbinding
type BindOperation string

const (
	// Each binding pod gets these.
	watcherContainerCPU = "400m"

	// Bind specifies a binding should be created
	Bind BindOperation = "BIND"
	// Unbind specifies a binding should be deleted
	Unbind BindOperation = "UNBIND"

	// BindOperationKey is the Env variable that gets set to requested BindOperation
	BindOperationKey string = "BIND_OPERATION"

	// BindTriggerKey is the Env variable that gets set to serialized trigger configuration
	BindTriggerKey string = "BIND_TRIGGER"

	// BindTargetKey is the Env variable that gets set to target of the bind operation
	BindTargetKey string = "BIND_TARGET"

	// BindContextKey is the Env variable that gets set to serialized BindContext if unbinding
	BindContextKey string = "BIND_CONTEXT"

	// EventSourceParametersKey is the Env variable that gets set to serialized EventSourceSpec
	EventSourceParametersKey string = "EVENT_SOURCE_PARAMETERS"

	// BindNamespaceKey is the Env variable that gets set to namespace of the container doing
	// the Bind (aka, namespace of the binding). Uses downward api
	BindNamespaceKey string = "BIND_NAMESPACE"

	// BindServiceAccount is the Env variable that gets set to serviceaccount of the
	// container doing the Bind. Uses downward api
	BindServiceAccountKey string = "BIND_SERVICE_ACCOUNT"
)

// MakeJob creates a Job to complete a bind or unbind operation.
func MakeJob(bind *v1alpha1.Bind, spec *v1alpha1.EventSourceSpec, op BindOperation, trigger EventTrigger, route string, bindContext BindContext) (*batchv1.Job, error) {
	labels := map[string]string{
		"app": "bindpod",
	}

	podTemplate, err := makePodTemplate(bind, spec, op, trigger, route, bindContext)
	if err != nil {
		return nil, err
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "binder",
			Namespace:       bind.Namespace,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(bind, v1alpha1.SchemeGroupVersion.WithKind("Bind"))},
		},
		Spec: batchv1.JobSpec{
			Template: *podTemplate,
		},
	}, nil
}

// makePodTemplate creates a pod template for a bind or unbind Job.
func makePodTemplate(bind *v1alpha1.Bind, spec *v1alpha1.EventSourceSpec, op BindOperation, trigger EventTrigger, route string, bindContext BindContext) (*corev1.PodTemplateSpec, error) {
	marshalledBindContext, err := json.Marshal(bindContext)
	if err != nil {
		return nil, err
	}
	encodedBindContext := base64.StdEncoding.EncodeToString(marshalledBindContext)

	marshalledTrigger, err := json.Marshal(trigger)
	if err != nil {
		return nil, err
	}
	encodedTrigger := base64.StdEncoding.EncodeToString(marshalledTrigger)

	encodedParameters := ""
	if spec.Parameters != nil {
		marshalledParameters, err := json.Marshal(spec.Parameters)
		if err != nil {
			return nil, err
		}
		encodedParameters = base64.StdEncoding.EncodeToString(marshalledParameters)
	}

	return &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"sidecar.istio.io/inject": "false",
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: bind.Spec.ServiceAccountName,
			RestartPolicy:      corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				corev1.Container{
					Name:            "binder",
					Image:           spec.Image,
					ImagePullPolicy: "Always",
					Env: []corev1.EnvVar{
						{
							Name:  BindOperationKey,
							Value: string(op),
						},
						{
							Name:  BindTargetKey,
							Value: route,
						},
						{
							Name:  BindTriggerKey,
							Value: encodedTrigger,
						},
						{
							Name:  BindContextKey,
							Value: encodedBindContext,
						},
						{
							Name:  EventSourceParametersKey,
							Value: encodedParameters,
						},
						{
							Name: BindNamespaceKey,
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "metadata.namespace",
								},
							},
						},
						{
							Name: BindServiceAccountKey,
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "spec.serviceAccountName",
								},
							},
						},
					},
				},
			},
		},
	}, nil
}
