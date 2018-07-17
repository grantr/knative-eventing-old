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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/knative/eventing/pkg/apis/feeds/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeClusterEventSources implements ClusterEventSourceInterface
type FakeClusterEventSources struct {
	Fake *FakeFeedsV1alpha1
}

var clustereventsourcesResource = schema.GroupVersionResource{Group: "feeds.knative.dev", Version: "v1alpha1", Resource: "clustereventsources"}

var clustereventsourcesKind = schema.GroupVersionKind{Group: "feeds.knative.dev", Version: "v1alpha1", Kind: "ClusterEventSource"}

// Get takes name of the clusterEventSource, and returns the corresponding clusterEventSource object, and an error if there is any.
func (c *FakeClusterEventSources) Get(name string, options v1.GetOptions) (result *v1alpha1.ClusterEventSource, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(clustereventsourcesResource, name), &v1alpha1.ClusterEventSource{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterEventSource), err
}

// List takes label and field selectors, and returns the list of ClusterEventSources that match those selectors.
func (c *FakeClusterEventSources) List(opts v1.ListOptions) (result *v1alpha1.ClusterEventSourceList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(clustereventsourcesResource, clustereventsourcesKind, opts), &v1alpha1.ClusterEventSourceList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.ClusterEventSourceList{}
	for _, item := range obj.(*v1alpha1.ClusterEventSourceList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested clusterEventSources.
func (c *FakeClusterEventSources) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(clustereventsourcesResource, opts))
}

// Create takes the representation of a clusterEventSource and creates it.  Returns the server's representation of the clusterEventSource, and an error, if there is any.
func (c *FakeClusterEventSources) Create(clusterEventSource *v1alpha1.ClusterEventSource) (result *v1alpha1.ClusterEventSource, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(clustereventsourcesResource, clusterEventSource), &v1alpha1.ClusterEventSource{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterEventSource), err
}

// Update takes the representation of a clusterEventSource and updates it. Returns the server's representation of the clusterEventSource, and an error, if there is any.
func (c *FakeClusterEventSources) Update(clusterEventSource *v1alpha1.ClusterEventSource) (result *v1alpha1.ClusterEventSource, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(clustereventsourcesResource, clusterEventSource), &v1alpha1.ClusterEventSource{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterEventSource), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeClusterEventSources) UpdateStatus(clusterEventSource *v1alpha1.ClusterEventSource) (*v1alpha1.ClusterEventSource, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(clustereventsourcesResource, "status", clusterEventSource), &v1alpha1.ClusterEventSource{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterEventSource), err
}

// Delete takes name of the clusterEventSource and deletes it. Returns an error if one occurs.
func (c *FakeClusterEventSources) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteAction(clustereventsourcesResource, name), &v1alpha1.ClusterEventSource{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeClusterEventSources) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(clustereventsourcesResource, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.ClusterEventSourceList{})
	return err
}

// Patch applies the patch and returns the patched clusterEventSource.
func (c *FakeClusterEventSources) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.ClusterEventSource, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(clustereventsourcesResource, name, data, subresources...), &v1alpha1.ClusterEventSource{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.ClusterEventSource), err
}
