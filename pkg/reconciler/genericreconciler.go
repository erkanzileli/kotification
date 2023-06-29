package reconciler

import (
	"context"
	"github.com/antonmedv/expr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type genericReconciler struct {
	gvk          schema.GroupVersionKind
	expression   string
	managerCache cache.Cache
}

func NewGenericReconciler(gvk schema.GroupVersionKind, expression string, managerCache cache.Cache) reconcile.Reconciler {
	return &genericReconciler{
		gvk:          gvk,
		expression:   expression,
		managerCache: managerCache,
	}
}

func (genericReconciler *genericReconciler) Reconcile(ctx context.Context, key reconcile.Request) (reconcile.Result, error) {
	log.Printf("reconciling %s", key.String())

	object := &unstructured.Unstructured{}
	object.SetGroupVersionKind(genericReconciler.gvk)
	err := genericReconciler.managerCache.Get(ctx, key.NamespacedName, object)
	if err != nil {
		log.Printf("failed to get object %s: %+v", key.String(), err)
		return reconcile.Result{}, nil
	}

	log.Printf("got unstructured object: %s/%s", object.GetNamespace(), object.GetName())

	resultRaw, err := expr.Eval(genericReconciler.expression, object.Object)
	if err != nil {
		log.Printf("failed to evaluate expression %s for %s: %+v", genericReconciler.expression, key.String(), err)
		return reconcile.Result{}, nil
	}

	result := GetBool(resultRaw)
	log.Printf("expression is %v for %s", result, key.String())

	return reconcile.Result{}, nil
}

func GetBool(val any) bool {
	b, ok := val.(bool)
	if !ok {
		return false
	}
	return b
}
