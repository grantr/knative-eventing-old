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
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/golang/glog"
	channelsv1alpha1 "github.com/knative/eventing/pkg/apis/channels/v1alpha1"
	feedsv1alpha1 "github.com/knative/eventing/pkg/apis/feeds/v1alpha1"
	"github.com/knative/eventing/pkg/sources"
	servingv1alpha1 "github.com/knative/serving/pkg/apis/serving/v1alpha1"
	yaml "gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimetypes "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// SuccessSynced is used as part of the Event 'reason' when a Bind is synced
	SuccessSynced = "Synced"

	// MessageResourceSynced is the message used for an Event fired when a Bind
	// is synced successfully
	MessageResourceSynced = "Bind synced successfully"
)

// Reconcile Routes
func (r *reconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	bind := &feedsv1alpha1.Bind{}
	err := r.client.Get(context.TODO(), request.NamespacedName, bind)

	if errors.IsNotFound(err) {
		log.Printf("Could not find Bind %v.\n", request)
		return reconcile.Result{}, nil
	}

	if err != nil {
		log.Printf("Could not fetch Bind %v for %+v\n", err, request)
		return reconcile.Result{}, err
	}

	// See if the binding has been deleted
	deletionTimestamp := bind.GetDeletionTimestamp()
	glog.Infof("DeletionTimestamp: %v", deletionTimestamp)

	functionDNS, err := r.resolveActionTarget(bind.Namespace, bind.Spec.Action)

	// Only return an error on not found if we're not deleting so that we can delete
	// the binding even if the route or channel has already been deleted.
	if err != nil && deletionTimestamp == nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("cannot resolve target for %v in namespace %q", bind.Spec.Action, bind.Namespace))
		}
		return reconcile.Result{}, err
	}

	es := &feedsv1alpha1.EventSource{}
	err = r.client.Get(context.TODO(), client.ObjectKey{Namespace: bind.Namespace, Name: bind.Spec.Trigger.Service}, es)
	if err != nil && deletionTimestamp == nil {
		if errors.IsNotFound(err) {
			if deletionTimestamp != nil {
				// If the Event Source can not be found, we will remove our finalizer
				// because without it, we can't unbind and hence this binding will never
				// be deleted.
				// https://github.com/knative/eventing/issues/94
				newFinalizers, err := RemoveFinalizer(bind, controllerAgentName)
				if err != nil {
					glog.Warningf("Failed to remove finalizer: %s", err)
					return reconcile.Result{}, err
				}
				bind.ObjectMeta.Finalizers = newFinalizers
				_, err = r.updateFinalizers(bind)
				if err != nil {
					glog.Warningf("Failed to update finalizers: %s", err)
					return reconcile.Result{}, err
				}
				return reconcile.Result{}, nil
			}
			runtime.HandleError(fmt.Errorf("EventSource %q in namespace %q does not exist", bind.Spec.Trigger.Service, bind.Namespace))
		}
		return reconcile.Result{}, err
	}

	et := &feedsv1alpha1.EventType{}
	err = r.client.Get(context.TODO(), client.ObjectKey{Namespace: bind.Namespace, Name: bind.Spec.Trigger.EventType}, et)
	if err != nil && deletionTimestamp == nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("EventType %q in namespace %q does not exist", bind.Spec.Trigger.Service, bind.Namespace))
		}
		return reconcile.Result{}, err
	}

	// If the EventSource has been deleted from underneath us, just remove our finalizer. We tried...
	if es == nil && deletionTimestamp != nil {
		glog.Warningf("Could not find a Bind container, removing finalizer")
		newFinalizers, err := RemoveFinalizer(bind, controllerAgentName)
		if err != nil {
			glog.Warningf("Failed to remove finalizer: %s", err)
			return reconcile.Result{}, err
		}
		bind.ObjectMeta.Finalizers = newFinalizers
		_, err = r.updateFinalizers(bind)
		if err != nil {
			glog.Warningf("Failed to update finalizers: %s", err)
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// If there are conditions or a context do nothing.
	if (bind.Status.Conditions != nil || bind.Status.BindContext != nil) && deletionTimestamp == nil {
		glog.Infof("Binding \"%s/%s\" already has status, skipping", bind.Namespace, bind.Name)
		return reconcile.Result{}, nil
	}

	// Set the OwnerReference to EventType to make sure that if it's deleted, the binding
	// will also get deleted and not left orphaned. However, this does not work yet. Regardless
	// we should set the owner reference to indicate a dependency.
	// https://github.com/knative/eventing/issues/94
	bind.ObjectMeta.OwnerReferences = append(bind.ObjectMeta.OwnerReferences, *newEventTypeNonControllerRef(et))
	err = r.client.Update(context.TODO(), bind)
	if err != nil {
		glog.Warningf("Failed to update OwnerReferences on bind '%s/%s' : %v", bind.Namespace, bind.Name, err)
		return reconcile.Result{}, err
	}

	trigger, err := r.resolveTrigger(bind.Namespace, bind.Spec.Trigger)
	if err != nil {
		glog.Warningf("Failed to process parameters: %s", err)
		return reconcile.Result{}, err
	}

	// check if the user specified a ServiceAccount to use and if so, use it.
	serviceAccountName := "default"
	if len(bind.Spec.ServiceAccountName) != 0 {
		serviceAccountName = bind.Spec.ServiceAccountName
	}
	binder := sources.NewContainerEventSource(bind, r.kubeclient, &es.Spec, bind.Namespace, serviceAccountName)
	if deletionTimestamp == nil {
		glog.Infof("Creating a subscription to %q : %q for resource %q", es.Name, et.Name, trigger.Resource)
		bindContext, err := binder.Bind(trigger, functionDNS)

		if err != nil {
			glog.Warningf("BIND failed: %s", err)
			msg := fmt.Sprintf("Bind failed with : %s", err)
			bind.Status.SetCondition(&feedsv1alpha1.BindCondition{
				Type:    feedsv1alpha1.BindFailed,
				Status:  corev1.ConditionTrue,
				Reason:  "BindFailed",
				Message: msg,
			})
		} else {
			glog.Infof("Got context back as: %+v", bindContext)
			marshalledBindContext, err := json.Marshal(&bindContext.Context)
			if err != nil {
				glog.Warningf("Couldn't marshal bind context: %+v : %s", bindContext, err)
			} else {
				glog.Infof("Marshaled context to: %+v", marshalledBindContext)
				bind.Status.BindContext = &runtimetypes.RawExtension{
					Raw: make([]byte, len(marshalledBindContext)),
				}
				bind.Status.BindContext.Raw = marshalledBindContext
			}

			// Set the finalizer since the bind succeeded, we need to clean up...
			// TODO: we should do this in the webhook instead...
			bind.Finalizers = append(bind.ObjectMeta.Finalizers, controllerAgentName)
			_, err = r.updateFinalizers(bind)
			if err != nil {
				glog.Warningf("Failed to update finalizers: %s", err)
				return reconcile.Result{}, err
			}

			bind.Status.SetCondition(&feedsv1alpha1.BindCondition{
				Type:    feedsv1alpha1.BindComplete,
				Status:  corev1.ConditionTrue,
				Reason:  "BindSuccess",
				Message: "Bind successful",
			})
		}
		_, err = r.updateStatus(bind)
		if err != nil {
			glog.Warningf("Failed to update status: %s", err)
			return reconcile.Result{}, err
		}
	} else {
		glog.Infof("Deleting a subscription to %q : %q with Trigger %+v", es.Name, et.Name, trigger)
		bindContext := sources.BindContext{
			Context: make(map[string]interface{}),
		}
		if bind.Status.BindContext != nil && bind.Status.BindContext.Raw != nil && len(bind.Status.BindContext.Raw) > 0 {
			if err := json.Unmarshal(bind.Status.BindContext.Raw, &bindContext.Context); err != nil {
				glog.Warningf("Couldn't unmarshal BindContext: %v", err)
				// TODO set the condition properly here
				return reconcile.Result{}, err
			}
		}
		err := binder.Unbind(trigger, bindContext)
		if err != nil {
			glog.Warningf("Couldn't unbind: %v", err)
			// TODO set the condition properly here
			return reconcile.Result{}, err
		}
		newFinalizers, err := RemoveFinalizer(bind, controllerAgentName)
		if err != nil {
			glog.Warningf("Failed to remove finalizer: %s", err)
			return reconcile.Result{}, err
		}
		bind.ObjectMeta.Finalizers = newFinalizers
		_, err = r.updateFinalizers(bind)
		if err != nil {
			glog.Warningf("Failed to update finalizers: %s", err)
			return reconcile.Result{}, err
		}
	}

	r.recorder.Event(bind, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return reconcile.Result{}, nil
}

func (r *reconciler) updateFinalizers(u *feedsv1alpha1.Bind) (*feedsv1alpha1.Bind, error) {
	bind := &feedsv1alpha1.Bind{}
	err := r.client.Get(context.TODO(), client.ObjectKey{Namespace: u.Namespace, Name: u.Name}, bind)
	if err != nil {
		return nil, err
	}
	//TODO(grantr) this should use sets
	bind.ObjectMeta.Finalizers = u.ObjectMeta.Finalizers
	return bind, r.client.Update(context.TODO(), bind)
}

func (r *reconciler) updateStatus(u *feedsv1alpha1.Bind) (*feedsv1alpha1.Bind, error) {
	bind := &feedsv1alpha1.Bind{}
	err := r.client.Get(context.TODO(), client.ObjectKey{Namespace: u.Namespace, Name: u.Name}, bind)
	if err != nil {
		return nil, err
	}
	bind.Status = u.Status
	// Until #38113 is merged, we must use Update instead of UpdateStatus to
	// update the Status block of the Bind resource. UpdateStatus will not
	// allow changes to the Spec of the resource, which is ideal for ensuring
	// nothing other than resource status has been updated.
	return bind, r.client.Update(context.TODO(), bind)
}

func (r *reconciler) resolveTrigger(namespace string, trigger feedsv1alpha1.EventTrigger) (sources.EventTrigger, error) {
	resolved := sources.EventTrigger{
		Resource:   trigger.Resource,
		EventType:  trigger.EventType,
		Parameters: make(map[string]interface{}),
	}
	if trigger.Parameters != nil && trigger.Parameters.Raw != nil && len(trigger.Parameters.Raw) > 0 {
		p := make(map[string]interface{})
		if err := yaml.Unmarshal(trigger.Parameters.Raw, &p); err != nil {
			return resolved, err
		}
		for k, v := range p {
			resolved.Parameters[k] = v
		}
	}
	if trigger.ParametersFrom != nil {
		glog.Infof("Fetching from source %+v", trigger.ParametersFrom)
		for _, p := range trigger.ParametersFrom {
			pfs, err := r.fetchParametersFromSource(namespace, &p)
			if err != nil {
				return resolved, err
			}
			for k, v := range pfs {
				resolved.Parameters[k] = v
			}
		}
	}
	return resolved, nil
}

func (r *reconciler) fetchParametersFromSource(namespace string, parametersFrom *feedsv1alpha1.ParametersFromSource) (map[string]interface{}, error) {
	var params map[string]interface{}
	if parametersFrom.SecretKeyRef != nil {
		glog.Infof("Fetching secret %+v", parametersFrom.SecretKeyRef)
		data, err := r.fetchSecretKeyValue(namespace, parametersFrom.SecretKeyRef)
		if err != nil {
			return nil, err
		}

		p, err := unmarshalJSON(data)
		if err != nil {
			return nil, err
		}
		params = p

	}
	return params, nil
}

func (r *reconciler) fetchSecretKeyValue(namespace string, secretKeyRef *feedsv1alpha1.SecretKeyReference) ([]byte, error) {
	secret := &corev1.Secret{}
	err := r.client.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: secretKeyRef.Name}, secret)
	if err != nil {
		return nil, err
	}
	return secret.Data[secretKeyRef.Key], nil
}

func unmarshalJSON(in []byte) (map[string]interface{}, error) {
	parameters := make(map[string]interface{})
	if err := json.Unmarshal(in, &parameters); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters as JSON object: %v", err)
	}
	return parameters, nil
}

// AddFinalizer adds value to the list of finalizers on obj
func AddFinalizer(obj runtimetypes.Object, value string) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	finalizers := sets.NewString(accessor.GetFinalizers()...)
	finalizers.Insert(value)
	accessor.SetFinalizers(finalizers.List())
	return nil
}

// RemoveFinalizer removes the given value from the list of finalizers in obj, then returns a new list
// of finalizers after value has been removed.
func RemoveFinalizer(obj runtimetypes.Object, value string) ([]string, error) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	finalizers := sets.NewString(accessor.GetFinalizers()...)
	finalizers.Delete(value)
	newFinalizers := finalizers.List()
	accessor.SetFinalizers(newFinalizers)
	return newFinalizers, nil
}

func newEventTypeNonControllerRef(et *feedsv1alpha1.EventType) *metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := false
	revRef := metav1.NewControllerRef(et, eventTypeControllerKind)
	revRef.BlockOwnerDeletion = &blockOwnerDeletion
	revRef.Controller = &isController
	return revRef
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Bind resource
// with the current status of the resource.
func (r *reconciler) resolveActionTarget(namespace string, action feedsv1alpha1.BindAction) (string, error) {
	if len(action.RouteName) > 0 {
		return r.resolveRouteDNS(namespace, action.RouteName)
	}
	if len(action.ChannelName) > 0 {
		return r.resolveChannelDNS(namespace, action.ChannelName)
	}
	// This should never happen, but because we don't have webhook validation yet, check
	// and complain.
	return "", fmt.Errorf("action is missing both RouteName and ChannelName")
}

func (r *reconciler) resolveRouteDNS(namespace string, routeName string) (string, error) {
	route := &servingv1alpha1.Route{}
	err := r.client.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: routeName}, route)
	if err != nil {
		return "", err
	}
	if len(route.Status.Domain) == 0 {
		return "", fmt.Errorf("route '%s/%s' is missing a domain", namespace, routeName)
	}
	return route.Status.Domain, nil
}

func (r *reconciler) resolveChannelDNS(namespace string, channelName string) (string, error) {
	channel := &channelsv1alpha1.Channel{}
	err := r.client.Get(context.TODO(), client.ObjectKey{Namespace: namespace, Name: channelName}, channel)
	if err != nil {
		return "", err
	}
	// TODO: The actual dns name should come from something in the status, or ?? But right
	// now it is hard coded to be <channelname>-channel
	// So we just check that the channel actually exists and tack on the -channel
	return fmt.Sprintf("%s-channel", channel.Name), nil
}
