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
	"testing"
	"time"

	datadoghqv1alpha1 "github.com/bpineau/birdcage/pkg/apis/datadoghq/v1alpha1"
	"github.com/onsi/gomega"
	"golang.org/x/net/context"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	c client.Client

	expectedRequest = reconcile.Request{NamespacedName: types.NamespacedName{Name: "foo", Namespace: "default"}}

	sourceDepKey = types.NamespacedName{Name: "bar", Namespace: "default"}
	targetDepKey = types.NamespacedName{Name: "bar-canary", Namespace: "default"}

	sourceDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sourceDepKey.Name,
			Namespace: sourceDepKey.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"deployment": instance.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"deployment": instance.Name}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx",
						},
					},
				},
			},
		},
	}

	instance = &datadoghqv1alpha1.Birdcage{
		ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "default"},
		Spec: datadoghqv1alpha1.BirdcageSpec{
			SourceObject: datadoghqv1alpha1.BirdcageSourceObject{
				Name:      "bar",
				Namespace: "default",
			},
			TargetObject: datadoghqv1alpha1.BirdcageTargetObject{
				Name:      "bar-canary",
				Namespace: "default",
				Labels:    map[string]string{"a": "b"},
			},
		},
	}
)

const timeout = time.Second * 5

func TestReconcile(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
	// channel when it is finished.
	mgr, err := manager.New(cfg, manager.Options{})
	g.Expect(err).NotTo(gomega.HaveOccurred())
	c = mgr.GetClient()

	recFn, requests := SetupTestReconcile(newReconciler(mgr))
	g.Expect(add(mgr, recFn)).NotTo(gomega.HaveOccurred())

	stopMgr, mgrStopped := StartTestManager(mgr, g)

	defer func() {
		close(stopMgr)
		mgrStopped.Wait()
	}()

	// Create the source (watched) deployment
	err = c.Create(context.TODO(), sourceDeployment)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Eventually(func() error { return c.Get(context.TODO(), sourceDepKey, sourceDeployment) }, timeout).
		Should(gomega.Succeed())

	// Create the Birdcage object and expect the Reconcile and Deployment to be created
	err = c.Create(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	defer c.Delete(context.TODO(), instance)
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// Birdcage should have derived and created a new target/canary deployment
	dep := &appsv1.Deployment{}
	g.Eventually(func() error { return c.Get(context.TODO(), targetDepKey, dep) }, timeout).
		Should(gomega.Succeed())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// Deleted target/canary should be re-created automaticaly
	newdep := &appsv1.Deployment{}
	err = c.Delete(context.TODO(), dep)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Eventually(func() error { return c.Get(context.TODO(), targetDepKey, newdep) }, timeout).
		Should(gomega.Succeed())

	// Deleted birdcage custom resource should propagate
	err = c.Delete(context.TODO(), instance)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Eventually(requests, timeout).Should(gomega.Receive(gomega.Equal(expectedRequest)))

	// Manually delete Deployment since GC isn't enabled in the test control plane
	g.Expect(c.Delete(context.TODO(), sourceDeployment)).To(gomega.Succeed())
}
