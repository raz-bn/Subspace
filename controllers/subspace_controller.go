/*


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

package controllers

import (
	"context"
	"k8s.io/apimachinery/pkg/types"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	util "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "dana.794/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// SubspaceReconciler reconciles a Subspace object
type SubspaceReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dana.794.dana.794,resources=subspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dana.794.dana.794,resources=subspaces/status,verbs=get;update;patch

func (r *SubspaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("subspace", req.NamespacedName)

	var ss v1.Subspace
	if err := r.Get(ctx, req.NamespacedName, &ss); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Subspace has been deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var sns corev1.Namespace
	nsn := types.NamespacedName{Name: ss.Name}
	if err := r.Get(ctx, nsn, &sns); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Info("Couldnt find Subnamespace")
			return ctrl.Result{}, err
		}
		// if subnamespace not found create an empty one
		sns = corev1.Namespace{}
	}

	if ss.Status.State == v1.Ok {
		if len(ss.Finalizers) == 0 {
			log.Info("Finalizer not present, add one for cleanup")
			if _, err := ctrl.CreateOrUpdate(ctx, r, &ss, func() error {
				util.AddFinalizer(&ss, v1.FinalizerHasSubnamespace)
				return nil
			}); err != nil {
				return ctrl.Result{}, err
			}
		}
		if ss.DeletionTimestamp != nil {
			if sns.Name != "" {
				if sns.DeletionTimestamp == nil {
					log.Info("Subspace has been deleted cleanup subnamespace")
					if err := r.Delete(ctx, &sns); err != nil {
						return ctrl.Result{}, err
					}
					return ctrl.Result{Requeue: true}, nil
				}
				return ctrl.Result{Requeue: true}, nil
			} else {
				log.Info("Cleanup complete remove finalizer")
				if _, err := ctrl.CreateOrUpdate(ctx, r, &ss, func() error {
					util.RemoveFinalizer(&ss, v1.FinalizerHasSubnamespace)
					return nil
				}); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}

	if ss.Status.State == v1.Missing {
		if sns.Name != "" {
			// if the subnamespace is already created change the state to Ok
			log.Info("Subnamespace created update Subspace state to Ok")
			if _, err := ctrl.CreateOrUpdate(ctx, r, &ss, func() error {
				ss.Status.State = v1.Ok
				return nil
			}); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			// create the subnamespace and reconcile again
			log.Info("Subnamespace is missing create one")
			sns.Name = ss.Name // cant set name of obj in mutefn
			if _, err := ctrl.CreateOrUpdate(ctx, r, &sns, func() error { return nil }); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}

	if sns.Name == "" {
		if ss.DeletionTimestamp == nil {
			//TODO:should check if someone did not delete the ns
			log.Info("First reconcile update Subspace state to Missing")
			if _, err := ctrl.CreateOrUpdate(ctx, r, &ss, func() error {
				ss.Status.State = v1.Missing
				return nil
			}); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *SubspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.Subspace{}).
		Complete(r)
}
