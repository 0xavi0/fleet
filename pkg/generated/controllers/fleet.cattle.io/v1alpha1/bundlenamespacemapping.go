/*
Copyright (c) 2020 - 2023 SUSE LLC

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

// Code generated by main. DO NOT EDIT.

package v1alpha1

import (
	"context"
	"time"

	v1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/rancher/lasso/pkg/client"
	"github.com/rancher/lasso/pkg/controller"
	"github.com/rancher/wrangler/pkg/generic"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

type BundleNamespaceMappingHandler func(string, *v1alpha1.BundleNamespaceMapping) (*v1alpha1.BundleNamespaceMapping, error)

type BundleNamespaceMappingController interface {
	generic.ControllerMeta
	BundleNamespaceMappingClient

	OnChange(ctx context.Context, name string, sync BundleNamespaceMappingHandler)
	OnRemove(ctx context.Context, name string, sync BundleNamespaceMappingHandler)
	Enqueue(namespace, name string)
	EnqueueAfter(namespace, name string, duration time.Duration)

	Cache() BundleNamespaceMappingCache
}

type BundleNamespaceMappingClient interface {
	Create(*v1alpha1.BundleNamespaceMapping) (*v1alpha1.BundleNamespaceMapping, error)
	Update(*v1alpha1.BundleNamespaceMapping) (*v1alpha1.BundleNamespaceMapping, error)

	Delete(namespace, name string, options *metav1.DeleteOptions) error
	Get(namespace, name string, options metav1.GetOptions) (*v1alpha1.BundleNamespaceMapping, error)
	List(namespace string, opts metav1.ListOptions) (*v1alpha1.BundleNamespaceMappingList, error)
	Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error)
	Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.BundleNamespaceMapping, err error)
}

type BundleNamespaceMappingCache interface {
	Get(namespace, name string) (*v1alpha1.BundleNamespaceMapping, error)
	List(namespace string, selector labels.Selector) ([]*v1alpha1.BundleNamespaceMapping, error)

	AddIndexer(indexName string, indexer BundleNamespaceMappingIndexer)
	GetByIndex(indexName, key string) ([]*v1alpha1.BundleNamespaceMapping, error)
}

type BundleNamespaceMappingIndexer func(obj *v1alpha1.BundleNamespaceMapping) ([]string, error)

type bundleNamespaceMappingController struct {
	controller    controller.SharedController
	client        *client.Client
	gvk           schema.GroupVersionKind
	groupResource schema.GroupResource
}

func NewBundleNamespaceMappingController(gvk schema.GroupVersionKind, resource string, namespaced bool, controller controller.SharedControllerFactory) BundleNamespaceMappingController {
	c := controller.ForResourceKind(gvk.GroupVersion().WithResource(resource), gvk.Kind, namespaced)
	return &bundleNamespaceMappingController{
		controller: c,
		client:     c.Client(),
		gvk:        gvk,
		groupResource: schema.GroupResource{
			Group:    gvk.Group,
			Resource: resource,
		},
	}
}

func FromBundleNamespaceMappingHandlerToHandler(sync BundleNamespaceMappingHandler) generic.Handler {
	return func(key string, obj runtime.Object) (ret runtime.Object, err error) {
		var v *v1alpha1.BundleNamespaceMapping
		if obj == nil {
			v, err = sync(key, nil)
		} else {
			v, err = sync(key, obj.(*v1alpha1.BundleNamespaceMapping))
		}
		if v == nil {
			return nil, err
		}
		return v, err
	}
}

func (c *bundleNamespaceMappingController) Updater() generic.Updater {
	return func(obj runtime.Object) (runtime.Object, error) {
		newObj, err := c.Update(obj.(*v1alpha1.BundleNamespaceMapping))
		if newObj == nil {
			return nil, err
		}
		return newObj, err
	}
}

func UpdateBundleNamespaceMappingDeepCopyOnChange(client BundleNamespaceMappingClient, obj *v1alpha1.BundleNamespaceMapping, handler func(obj *v1alpha1.BundleNamespaceMapping) (*v1alpha1.BundleNamespaceMapping, error)) (*v1alpha1.BundleNamespaceMapping, error) {
	if obj == nil {
		return obj, nil
	}

	copyObj := obj.DeepCopy()
	newObj, err := handler(copyObj)
	if newObj != nil {
		copyObj = newObj
	}
	if obj.ResourceVersion == copyObj.ResourceVersion && !equality.Semantic.DeepEqual(obj, copyObj) {
		return client.Update(copyObj)
	}

	return copyObj, err
}

func (c *bundleNamespaceMappingController) AddGenericHandler(ctx context.Context, name string, handler generic.Handler) {
	c.controller.RegisterHandler(ctx, name, controller.SharedControllerHandlerFunc(handler))
}

func (c *bundleNamespaceMappingController) AddGenericRemoveHandler(ctx context.Context, name string, handler generic.Handler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), handler))
}

func (c *bundleNamespaceMappingController) OnChange(ctx context.Context, name string, sync BundleNamespaceMappingHandler) {
	c.AddGenericHandler(ctx, name, FromBundleNamespaceMappingHandlerToHandler(sync))
}

func (c *bundleNamespaceMappingController) OnRemove(ctx context.Context, name string, sync BundleNamespaceMappingHandler) {
	c.AddGenericHandler(ctx, name, generic.NewRemoveHandler(name, c.Updater(), FromBundleNamespaceMappingHandlerToHandler(sync)))
}

func (c *bundleNamespaceMappingController) Enqueue(namespace, name string) {
	c.controller.Enqueue(namespace, name)
}

func (c *bundleNamespaceMappingController) EnqueueAfter(namespace, name string, duration time.Duration) {
	c.controller.EnqueueAfter(namespace, name, duration)
}

func (c *bundleNamespaceMappingController) Informer() cache.SharedIndexInformer {
	return c.controller.Informer()
}

func (c *bundleNamespaceMappingController) GroupVersionKind() schema.GroupVersionKind {
	return c.gvk
}

func (c *bundleNamespaceMappingController) Cache() BundleNamespaceMappingCache {
	return &bundleNamespaceMappingCache{
		indexer:  c.Informer().GetIndexer(),
		resource: c.groupResource,
	}
}

func (c *bundleNamespaceMappingController) Create(obj *v1alpha1.BundleNamespaceMapping) (*v1alpha1.BundleNamespaceMapping, error) {
	result := &v1alpha1.BundleNamespaceMapping{}
	return result, c.client.Create(context.TODO(), obj.Namespace, obj, result, metav1.CreateOptions{})
}

func (c *bundleNamespaceMappingController) Update(obj *v1alpha1.BundleNamespaceMapping) (*v1alpha1.BundleNamespaceMapping, error) {
	result := &v1alpha1.BundleNamespaceMapping{}
	return result, c.client.Update(context.TODO(), obj.Namespace, obj, result, metav1.UpdateOptions{})
}

func (c *bundleNamespaceMappingController) Delete(namespace, name string, options *metav1.DeleteOptions) error {
	if options == nil {
		options = &metav1.DeleteOptions{}
	}
	return c.client.Delete(context.TODO(), namespace, name, *options)
}

func (c *bundleNamespaceMappingController) Get(namespace, name string, options metav1.GetOptions) (*v1alpha1.BundleNamespaceMapping, error) {
	result := &v1alpha1.BundleNamespaceMapping{}
	return result, c.client.Get(context.TODO(), namespace, name, result, options)
}

func (c *bundleNamespaceMappingController) List(namespace string, opts metav1.ListOptions) (*v1alpha1.BundleNamespaceMappingList, error) {
	result := &v1alpha1.BundleNamespaceMappingList{}
	return result, c.client.List(context.TODO(), namespace, result, opts)
}

func (c *bundleNamespaceMappingController) Watch(namespace string, opts metav1.ListOptions) (watch.Interface, error) {
	return c.client.Watch(context.TODO(), namespace, opts)
}

func (c *bundleNamespaceMappingController) Patch(namespace, name string, pt types.PatchType, data []byte, subresources ...string) (*v1alpha1.BundleNamespaceMapping, error) {
	result := &v1alpha1.BundleNamespaceMapping{}
	return result, c.client.Patch(context.TODO(), namespace, name, pt, data, result, metav1.PatchOptions{}, subresources...)
}

type bundleNamespaceMappingCache struct {
	indexer  cache.Indexer
	resource schema.GroupResource
}

func (c *bundleNamespaceMappingCache) Get(namespace, name string) (*v1alpha1.BundleNamespaceMapping, error) {
	obj, exists, err := c.indexer.GetByKey(namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(c.resource, name)
	}
	return obj.(*v1alpha1.BundleNamespaceMapping), nil
}

func (c *bundleNamespaceMappingCache) List(namespace string, selector labels.Selector) (ret []*v1alpha1.BundleNamespaceMapping, err error) {

	err = cache.ListAllByNamespace(c.indexer, namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.BundleNamespaceMapping))
	})

	return ret, err
}

func (c *bundleNamespaceMappingCache) AddIndexer(indexName string, indexer BundleNamespaceMappingIndexer) {
	utilruntime.Must(c.indexer.AddIndexers(map[string]cache.IndexFunc{
		indexName: func(obj interface{}) (strings []string, e error) {
			return indexer(obj.(*v1alpha1.BundleNamespaceMapping))
		},
	}))
}

func (c *bundleNamespaceMappingCache) GetByIndex(indexName, key string) (result []*v1alpha1.BundleNamespaceMapping, err error) {
	objs, err := c.indexer.ByIndex(indexName, key)
	if err != nil {
		return nil, err
	}
	result = make([]*v1alpha1.BundleNamespaceMapping, 0, len(objs))
	for _, obj := range objs {
		result = append(result, obj.(*v1alpha1.BundleNamespaceMapping))
	}
	return result, nil
}
