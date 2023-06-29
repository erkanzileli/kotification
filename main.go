package main

import (
	"github.com/erkanzileli/kotification/pkg/genericcontroller"
	"github.com/erkanzileli/kotification/pkg/reconciler"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	gvk = schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1alpha3",
		Kind:    "VirtualService",
	}
	expression = `any(spec.gateways, "mesh")`
)

func main() {
	ctx := signals.SetupSignalHandler()

	kubeClientConf := config.GetConfigOrDie()
	managerConf := manager.Options{
		LeaderElection:          true,
		LeaderElectionID:        "kotification",
		LeaderElectionNamespace: "default",
	}
	mgr, err := manager.New(kubeClientConf, managerConf)
	if err != nil {
		log.Fatalf("failed to create a new manager for creating controllers: %+v", err)
	}

	gvkController, err := genericcontroller.NewUnmanaged(mgr, gvk, genericcontroller.Options{
		Reconciler: reconciler.NewGenericReconciler(gvk, expression, mgr.GetCache()),
	})

	if err = mgr.Add(gvkController); err != nil {
		log.Fatalf("failed to register controller to manager: %+v", err)
	}

	log.Print("controller added to manager")

	// Entrypoint
	if err = mgr.Start(ctx); err != nil {
		log.Fatalf("failed to start all registered controllers: %+v", err)
	}
}
