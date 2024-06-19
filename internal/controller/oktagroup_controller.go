/*
Copyright 2024.

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

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	accessmanagerv1 "github.com/franciscoprin/access-manager-operator/api/v1"
	"github.com/okta/okta-sdk-golang/v2/okta"
)

// OktaGroupReconciler reconciles a OktaGroup object
type OktaGroupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=access-manager.github.com,resources=oktagroups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=access-manager.github.com,resources=oktagroups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=access-manager.github.com,resources=oktagroups/finalizers,verbs=update

const (
	ConstOktaGroupFinalizer = "franciscoprin.access-manager-operator.finalizer"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the OktaGroup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.0/pkg/reconcile
func (r *OktaGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// Get Okta group object
	oktaGroupCRD := &accessmanagerv1.OktaGroup{}
	if err := r.Get(ctx, req.NamespacedName, oktaGroupCRD); err != nil {
		log.Log.Error(err, "unable to fetch OktaGroup")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Create the Okta client
	_, oktaClient, err := okta.NewClient(ctx, okta.WithCache(false))
	if err != nil {
		log.Log.Error(err, "unable to create Okta client")
		return ctrl.Result{}, err
	}

	// Set up the OktaGroup manager
	oktaManager, err := NewOktaGroupManager(ctx, oktaGroupCRD, oktaClient)
	if err != nil {
		log.Log.Error(err, "unable to create OktaGroup manager")
		return ctrl.Result{}, err
	}

	// examine DeletionTimestamp to determine if object is under deletion
	if oktaGroupCRD.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// to registering our finalizer.
		if !controllerutil.ContainsFinalizer(oktaGroupCRD, ConstOktaGroupFinalizer) {
			controllerutil.AddFinalizer(oktaGroupCRD, ConstOktaGroupFinalizer)
			if err := r.Update(ctx, oktaGroupCRD); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(oktaGroupCRD, ConstOktaGroupFinalizer) {
			// Delete the Okta group. If the deletion fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := oktaManager.DeleteOktaGroup(); err != nil {
				log.Log.Error(err, "unable to delete OktaGroupAPI")
				return ctrl.Result{}, err
			}

			// Remove ConstOktaGroupFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(oktaGroupCRD, ConstOktaGroupFinalizer)
			if err := r.Update(ctx, oktaGroupCRD); err != nil {
				log.Log.Error(err, "unable to delete after removing finalizer OktaGroupAPI")
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	// Upsert the Okta group
	oktaGroupAPI, err := oktaManager.UpsertOktaGroup()
	if err != nil {
		log.Log.Error(err, "unable to upsert OktaGroupAPI")
		return ctrl.Result{}, err
	}

	// Add users to the Okta group API
	if err = oktaManager.UpsertUsersToOktaGroup(oktaGroupAPI); err != nil {
		log.Log.Error(err, "unable to upsert users to OktaGroupAPI")
		return ctrl.Result{}, err
	}

	// Refresh the group by using the Id
	oktaGroupAPI, err = oktaManager.SearchOktaGroup(oktaGroupAPI.Id)
	if err != nil {
		log.Log.Error(err, "unable to get OktaGroupAPI")
		return ctrl.Result{}, err
	}

	// Update the OktaGroup status
	oktaGroupCRD.Status = accessmanagerv1.OktaGroupStatus{
		Id:      oktaGroupAPI.Id,
		Created: metav1.NewTime(oktaGroupAPI.Created.UTC()),

		// Convert the time.Time pointers to metav1.Time, with UTC timezone
		LastMembershipUpdated: metav1.NewTime(oktaGroupAPI.LastMembershipUpdated.UTC()),
		LastUpdated:           metav1.NewTime(oktaGroupAPI.LastUpdated.UTC()),
	}

	if err := r.Status().Update(ctx, oktaGroupCRD); err != nil {
		log.Log.Error(err, "unable to update OktaGroupCRD status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OktaGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&accessmanagerv1.OktaGroup{}).
		Complete(r)
}
