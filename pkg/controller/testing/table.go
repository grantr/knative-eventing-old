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

package testing

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TestCase holds a single row of our table test.
type TestCase struct {
	// Name is a descriptive name for this test suitable as a first argument to t.Run()
	Name string

	// InitialState is the list of objects that already exists when reconciliation
	// starts.
	InitialState []runtime.Object

	// ReconcileRequest is the argument to pass to the Reconcile function.
	ReconcileRequest reconcile.Request

	// WantErr is true when we expect the Reconcile function to return an error.
	WantErr bool

	// WantErrMsg contains the pattern to match the returned error message.
	// Implies WantErr = true.
	WantErrMsg string

	// WantResult is the reconcile result we expect to be returned from the
	// Reconcile function.
	WantResult reconcile.Result

	// WantPresent holds the non-exclusive set of objects we expect to exist
	// after reconciliation completes.
	WantPresent []runtime.Object

	// WantAbsent holds the list of objects expected to not exist
	// after reconciliation completes.
	WantAbsent []runtime.Object
}

// Runner returns a testing func that can be passed to t.Run.
func (tc *TestCase) Runner(t *testing.T, r reconcile.Reconciler, c client.Client) func(t *testing.T) {
	return func(t *testing.T) {
		result, recErr := tc.Reconcile(r)

		if err := tc.VerifyErr(recErr); err != nil {
			t.Error(err)
		}

		if err := tc.VerifyResult(result); err != nil {
			t.Error(err)
		}

		if err := tc.VerifyWantPresent(c); err != nil {
			t.Error(err)
		}

		if err := tc.VerifyWantAbsent(c); err != nil {
			t.Error(err)
		}
	}
}

// GetClient returns the fake client to use for this test case.
func (tc *TestCase) GetClient() client.Client {
	return fake.NewFakeClient(tc.InitialState...)
}

// Reconcile calls the given reconciler's Reconcile() function with the test
// case's reconcile request.
func (tc *TestCase) Reconcile(r reconcile.Reconciler) (reconcile.Result, error) {
	return r.Reconcile(tc.ReconcileRequest)
}

// VerifyErr verifies that the given error returned from Reconcile is the error
// expected by the test case.
func (tc *TestCase) VerifyErr(err error) error {
	if tc.WantErr && err == nil {
		return fmt.Errorf("want error, got nil")
	}

	if !tc.WantErr && err != nil {
		return fmt.Errorf("want no error, got %v", err)
	}

	//TODO if tc.WantErrMsg
	return nil
}

// VerifyResult verifies that the given result returned from Reconcile is the
// result expected by the test case.
func (tc *TestCase) VerifyResult(result reconcile.Result) error {
	if diff := cmp.Diff(tc.WantResult, result); diff != "" {
		return fmt.Errorf("Unexpected reconcile Result (-want +got) %v", diff)
	}
	return nil
}

// VerifyWantPresent verifies that the client contains all the objects expected
// to be present after reconciliation.
func (tc *TestCase) VerifyWantPresent(c client.Client) error {
	//TODO(grantr) return all errors
	for _, wp := range tc.WantPresent {
		o := wp.DeepCopyObject()
		acc, err := meta.Accessor(wp)
		if err != nil {
			return fmt.Errorf("Error getting accessor for %v", wp)
		}
		err = c.Get(context.TODO(), client.ObjectKey{Namespace: acc.GetNamespace(), Name: acc.GetName()}, o)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("Want present, got absent %v", acc)
			}
			return fmt.Errorf("Error getting %v: %v", acc, err)
		}

		if diff := cmp.Diff(wp, o); diff != "" {
			return fmt.Errorf("Unexpected present object (-want +got) %v", diff)
		}
	}
	return nil
}

// VerifyWantAbsent verifies that the client does not contain any of the objects
// expected to be absent after reconciliation.
func (tc *TestCase) VerifyWantAbsent(c client.Client) error {
	//TODO(grantr) return all errors
	for _, wa := range tc.WantAbsent {
		o := wa.DeepCopyObject()
		acc, err := meta.Accessor(wa)
		if err != nil {
			return fmt.Errorf("Error getting accessor for %v", wa)
		}
		err = c.Get(context.TODO(), client.ObjectKey{Namespace: acc.GetNamespace(), Name: acc.GetName()}, o)
		if err == nil {
			return fmt.Errorf("Want absent, got present %v", acc)
		}
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("Error getting %v: %v", acc, err)
		}
	}
	return nil
}
