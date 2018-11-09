/*
Copyright 2018 Datadog Inc..

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

package birdcage

import (
	"context"
	"reflect"
	"sync"

	datadoghqv1alpha1 "github.com/bpineau/birdcage/pkg/apis/datadoghq/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	watchedSourceList   = make(map[string]types.NamespacedName)
	watchedSourceListMu sync.RWMutex

	p = predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			nspname := &types.NamespacedName{
				Namespace: e.MetaNew.GetNamespace(),
				Name:      e.MetaNew.GetName(),
			}
			watchedSourceListMu.RLock()
			defer watchedSourceListMu.RUnlock()
			if _, ok := watchedSourceList[nspname.String()]; !ok {
				return false
			}
			return e.ObjectOld != e.ObjectNew
		},
		CreateFunc: func(e event.CreateEvent) bool {
			nspname := &types.NamespacedName{
				Namespace: e.Meta.GetNamespace(),
				Name:      e.Meta.GetName(),
			}
			watchedSourceListMu.RLock()
			defer watchedSourceListMu.RUnlock()
			if _, ok := watchedSourceList[nspname.String()]; !ok {
				return false
			}
			return true
		},
	}

	mapFn = handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			nspname := &types.NamespacedName{
				Namespace: a.Meta.GetNamespace(),
				Name:      a.Meta.GetName(),
			}
			watchedSourceListMu.RLock()
			defer watchedSourceListMu.RUnlock()
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name:      watchedSourceList[nspname.String()].Name,
					Namespace: watchedSourceList[nspname.String()].Namespace,
				}},
			}
		})
)

// Add creates a new Birdcage Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
// USER ACTION REQUIRED: update cmd/manager/main.go to call this datadoghq.Add(mgr) to install this Controller
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileBirdcage{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetRecorder("birdcages"),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("birdcage-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Birdcage
	err = c.Watch(&source.Kind{Type: &datadoghqv1alpha1.Birdcage{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch source deployments
	err = c.Watch(
		&source.Kind{Type: &appsv1.Deployment{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: mapFn,
		},
		p)
	if err != nil {
		return err
	}

	// Watch target deployments
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.Birdcage{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileBirdcage{}

// ReconcileBirdcage reconciles a Birdcage object
type ReconcileBirdcage struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a Birdcage object and makes changes based on the state read
// and what is in the Birdcage.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.datadoghq.com,resources=birdcages,verbs=get;list;watch;create;update;patch;delete
func (r *ReconcileBirdcage) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the Birdcage instance
	instance := &datadoghqv1alpha1.Birdcage{}

	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	watchedSourceName := &types.NamespacedName{
		Namespace: instance.Spec.SourceObject.Namespace,
		Name:      instance.Spec.SourceObject.Name,
	}
	watchedSourceListMu.Lock()
	watchedSourceList[watchedSourceName.String()] = request.NamespacedName
	watchedSourceListMu.Unlock()

	// The birdcage object is being deleted, we're done here
	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		watchedSourceListMu.Lock()
		delete(watchedSourceList, watchedSourceName.String())
		watchedSourceListMu.Unlock()
		return reconcile.Result{}, nil
	}

	// Retrieve our watched source deployment (if it exists, else we're done for now)
	watchedSource := &appsv1.Deployment{}
	if err = r.Get(context.TODO(), *watchedSourceName, watchedSource); err != nil {
		// No deployment yet (or anymore), nothing to do
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// This will be our target/canary deployment model
	target, err := newCanaryDeployment(instance, watchedSource, r.scheme)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Create target/canary if needed
	found := &appsv1.Deployment{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.create(instance, target)
	} else if err != nil {
		return reconcile.Result{}, err
	}

	if found.GetNamespace() == "" || found.GetName() == "" {
		// object still being created
		return reconcile.Result{}, nil
	}

	// Update target/canary if needed
	if !reflect.DeepEqual(target.Spec, found.Spec) {
		found.Spec = target.Spec
		return r.update(instance, found)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileBirdcage) create(instance *datadoghqv1alpha1.Birdcage, target *appsv1.Deployment) (reconcile.Result, error) {
	log := logf.Log.WithName("reconcile")
	err := r.Create(context.TODO(), target)
	if err != nil {
		log.Error(err, "creating deployment", "namespace", target.Namespace, "name", target.Name)
		return reconcile.Result{}, err
	}
	log.Info("Created canary deployment", "namespace", target.Namespace, "name", target.Name)
	r.recorder.Eventf(instance, "Normal", "Created", "created deployment %s/%s", target.Namespace, target.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileBirdcage) update(instance *datadoghqv1alpha1.Birdcage, target *appsv1.Deployment) (reconcile.Result, error) {
	log := logf.Log.WithName("reconcile")
	err := r.Update(context.TODO(), target)
	if err != nil {
		log.Error(err, "updating deployment", "namespace", target.Namespace, "name", target.Name)
		return reconcile.Result{}, err
	}
	log.Info("Updated canary deployment", "namespace", target.Namespace, "name", target.Name)
	r.recorder.Eventf(instance, "Normal", "Updated", "updated deployment %s/%s", target.Namespace, target.Name)
	return reconcile.Result{}, nil
}

func newCanaryDeployment(birdcage *datadoghqv1alpha1.Birdcage, source *appsv1.Deployment, scheme *runtime.Scheme) (*appsv1.Deployment, error) {
	targetObject := &birdcage.Spec.TargetObject
	sourceSpec := &source.Spec
	var nbReplicas int32 = 1

	res := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetObject.Name,
			Namespace: targetObject.Namespace,
			Labels:    targetObject.Labels,
		},
		Spec: appsv1.DeploymentSpec{
			// TODO: make this configurable with the targetObject
			Replicas: &nbReplicas,
			Template: *sourceSpec.Template.DeepCopy(),
		},
	}
	// Overwrite labels in template as well
	res.Spec.Template.Labels = targetObject.Labels
	res.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: targetObject.Labels,
	}

	// Target deployment is owned by our birdcage object
	if err := controllerutil.SetControllerReference(birdcage, res, scheme); err != nil {
		return res, err
	}

	// TODO patch target with kustomize or jsonnet here
	return res, nil
}
