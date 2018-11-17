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
	"fmt"
	"reflect"
	"strings"

	"github.com/yuin/gopher-lua"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"

	datadoghqv1alpha1 "github.com/bpineau/birdcage/pkg/apis/datadoghq/v1alpha1"
	"github.com/bpineau/birdcage/pkg/watchlist"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	luajson "layeh.com/gopher-json"
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

const finalizerKey = "finalizer.birdcage.datadoghq.com"

var (
	watchedSourceList = watchlist.New()

	p = predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			nspname := &types.NamespacedName{
				Namespace: e.MetaNew.GetNamespace(),
				Name:      e.MetaNew.GetName(),
			}
			ok := watchedSourceList.Get(*nspname)
			if len(ok) == 0 {
				return false
			}
			return e.ObjectOld != e.ObjectNew
		},
		CreateFunc: func(e event.CreateEvent) bool {
			nspname := &types.NamespacedName{
				Namespace: e.Meta.GetNamespace(),
				Name:      e.Meta.GetName(),
			}
			ok := watchedSourceList.Get(*nspname)
			if len(ok) == 0 {
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

			var result []reconcile.Request
			srcs := watchedSourceList.Get(*nspname)
			for _, src := range srcs {
				rr := reconcile.Request{
					NamespacedName: src,
				}
				result = append(result, rr)
			}
			return result
		})
)

// Add creates a new Birdcage Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	scheme := mgr.GetScheme()
	return &ReconcileBirdcage{
		Client:     mgr.GetClient(),
		scheme:     scheme,
		recorder:   mgr.GetRecorder("birdcages"),
		serializer: json.NewSerializer(json.DefaultMetaFactory, scheme, scheme, false),
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

	// Watch generated, child target deployments
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
	scheme     *runtime.Scheme
	recorder   record.EventRecorder
	serializer runtime.Serializer
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
			// Cleanup is handled by finalizer and children ownership refs
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The birdcage object is being deleted, we're done here
		return r.finalize(instance)
	}

	// Inject our finalizer if needed
	if !containsString(instance.ObjectMeta.Finalizers, finalizerKey) {
		instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, finalizerKey)
		if err := r.Update(context.Background(), instance); err != nil {
			return reconcile.Result{Requeue: true}, nil
		}
	}

	watchedSourceName := &types.NamespacedName{
		Namespace: instance.Spec.SourceObject.Namespace,
		Name:      instance.Spec.SourceObject.Name,
	}
	watchedSourceList.Add(*watchedSourceName, request.NamespacedName)

	targetName := types.NamespacedName{
		Namespace: instance.Spec.TargetObject.Namespace,
		Name:      instance.Spec.TargetObject.Name,
	}

	// Retrieve our watched source deployment
	watchedSource := &appsv1.Deployment{}
	if err = r.Get(context.TODO(), *watchedSourceName, watchedSource); err != nil {
		// No source deployment anymore
		if errors.IsNotFound(err) {
			// Delete our target deployment if any
			return r.delete(instance, targetName)
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Generate our target deployment spec
	target, err := r.patchDeployment(instance, watchedSource)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Create target/canary if needed
	found := &appsv1.Deployment{}
	err = r.Get(context.TODO(), targetName, found)
	if err != nil && errors.IsNotFound(err) {
		return r.create(instance, target)
	} else if err != nil {
		return reconcile.Result{}, err
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
		log.Error(err, "updating canary deployment", "namespace", target.Namespace, "name", target.Name)
		return reconcile.Result{}, err
	}
	log.Info("Updated canary deployment", "namespace", target.Namespace, "name", target.Name)
	r.recorder.Eventf(instance, "Normal", "Updated", "updated deployment %s/%s", target.Namespace, target.Name)
	return reconcile.Result{}, nil
}

func (r *ReconcileBirdcage) delete(instance *datadoghqv1alpha1.Birdcage, targetName types.NamespacedName) (reconcile.Result, error) {
	log := logf.Log.WithName("reconcile")

	found := &appsv1.Deployment{}
	err := r.Get(context.TODO(), targetName, found)
	if err != nil && errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, err
	}

	err = r.Delete(context.TODO(), found)
	log.Info("Deleting canary", "namespace", targetName.Namespace, "name", targetName.Name)
	r.recorder.Eventf(instance, "Normal", "Deleted", "deleted deployment %s", targetName.String())
	return reconcile.Result{}, err
}

func (r *ReconcileBirdcage) finalize(instance *datadoghqv1alpha1.Birdcage) (reconcile.Result, error) {
	log := logf.Log.WithName("reconcile")

	// "You should implement the pre-delete logic in such a way
	// that it is safe to invoke it multiple times for the same object.
	if !containsString(instance.ObjectMeta.Finalizers, finalizerKey) {
		return reconcile.Result{}, nil
	}

	log.Info("Deleting birdcage", "namespace", instance.GetNamespace(), "name", instance.GetName())
	watchedSourceList.Remove(types.NamespacedName{Namespace: instance.GetNamespace(), Name: instance.GetName()})

	// remove our finalizer from the list and update it.
	instance.ObjectMeta.Finalizers = removeString(instance.ObjectMeta.Finalizers, finalizerKey)
	if err := r.Update(context.Background(), instance); err != nil {
		return reconcile.Result{Requeue: true}, nil
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileBirdcage) patchDeployment(birdcage *datadoghqv1alpha1.Birdcage, source *appsv1.Deployment) (*appsv1.Deployment, error) {
	targetObject := &birdcage.Spec.TargetObject
	obj := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      targetObject.Name,
			Namespace: targetObject.Namespace,
			Labels:    source.GetLabels(),
		},
		Spec: source.Spec,
	}

	buffer := strings.Builder{}
	if err := r.serializer.Encode(obj, &buffer); err != nil {
		return nil, err
	}
	encoded, err := runLuaPatch(birdcage.Spec.TargetObject.LuaCode, buffer.String())
	if err != nil {
		return nil, err
	}

	gvk := obj.GroupVersionKind()
	_, _, err = r.serializer.Decode([]byte(encoded), &gvk, obj)
	if err != nil {
		return nil, err
	}

	// Target deployment is owned by our birdcage object
	if err := controllerutil.SetControllerReference(birdcage, obj, r.scheme); err != nil {
		return nil, err
	}

	// XXX FIXME - this is ugly (but fixes all spurious updates and conflicts on update we have)
	obj.Spec.Template.Spec.SecurityContext = source.Spec.Template.Spec.SecurityContext.DeepCopy()

	return obj, nil
}

const luaStub = `
local json = require("json")

function __stub(rawJSON)
	retObj = patch(json.decode(rawJSON))
	return json.encode(retObj)
end
`

func runLuaPatch(code string, rawInput string) (string, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	// only enable the minimum requiered base modules (no syscalls etc)
	for _, pair := range []struct {
		n string
		f lua.LGFunction
	}{
		{lua.LoadLibName, lua.OpenPackage}, // Must be first
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
	} {
		if err := L.CallByParam(lua.P{
			Fn:      L.NewFunction(pair.f),
			NRet:    0,
			Protect: true,
		}, lua.LString(pair.n)); err != nil {
			panic(err)
		}
	}
	L.PreloadModule("json", luajson.Loader)

	err := L.DoString(luaStub + code)
	if err != nil {
		return "", fmt.Errorf("failed to load LUA script: %s", err)
	}

	if err := L.CallByParam(lua.P{
		Fn:      L.GetGlobal("__stub"),
		NRet:    1,
		Protect: true,
	}, lua.LString(rawInput)); err != nil {
		return "", err
	}
	ret := L.Get(-1) // returned value
	L.Pop(1)         // remove received value

	return ret.String(), nil
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
