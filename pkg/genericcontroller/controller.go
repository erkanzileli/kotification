package genericcontroller

import (
	"context"
	"errors"
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sync"
	"time"

	ctrlmetrics "github.com/erkanzileli/kotification/pkg/genericcontroller/metrics"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	metricLabelError        = "error"
	metricLabelRequeueAfter = "requeue_after"
	metricLabelRequeue      = "requeue"
	metricLabelSuccess      = "success"
)

type Controller interface {
	manager.Runnable
}

type genericController struct {
	name        string
	resourceGVK schema.GroupVersionKind
	manager     manager.Manager
	reconciler  reconcile.Reconciler

	mu                      sync.Mutex
	ctx                     context.Context
	queue                   workqueue.RateLimitingInterface
	started                 bool
	maxConcurrentReconciles int
}

func NewUnStarted(mgr manager.Manager, gvk schema.GroupVersionKind, reconciler reconcile.Reconciler) (Controller, error) {
	maxConcurrentReconciles := mgr.GetControllerOptions().MaxConcurrentReconciles
	if maxConcurrentReconciles == 0 {
		maxConcurrentReconciles = 1
	}

	return &genericController{
		manager:                 mgr,
		resourceGVK:             gvk,
		reconciler:              reconciler,
		name:                    gvk.String(),
		maxConcurrentReconciles: maxConcurrentReconciles,
	}, nil
}

func (c *genericController) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.started {
		return errors.New("controller was started more than once. This is likely to be caused by being added to a manager multiple times")
	}
	c.ctx = ctx

	c.initMetrics()

	if !APIExists(c.manager.GetRESTMapper(), c.resourceGVK) {
		c.manager.GetScheme().AddKnownTypeWithName(c.resourceGVK, &unstructured.Unstructured{})
	}

	informerForGVK, err := c.manager.GetCache().GetInformerForKind(ctx, c.resourceGVK)
	if err != nil {
		return fmt.Errorf("could not create informer for gvk %s: %w", c.resourceGVK.String(), err)
	}

	c.queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	resourceEventHandlerRegistration, err := informerForGVK.AddEventHandler(NewEventHandler(ctx, c.queue, &handler.EnqueueRequestForObject{}, nil).HandlerFuncs())
	if err != nil {
		return fmt.Errorf("could not register handler to informer of gvk %s: %w", c.resourceGVK.String(), err)
	}

	go func() {
		<-ctx.Done()
		c.queue.ShutDown()
		log.Printf("successfully shut down the queue for gvk %s", c.resourceGVK.String())

		if err := informerForGVK.RemoveEventHandler(resourceEventHandlerRegistration); err != nil {
			log.Printf("could not remove event handler for gvk %s", c.resourceGVK.String())
		} else {
			log.Printf("successfully removed event handler for gvk: %s", c.resourceGVK.String())
		}
	}()

	wg := &sync.WaitGroup{}
	err = func() error {
		defer c.mu.Unlock()
		defer utilruntime.HandleCrash()

		c.getLogger().Info("starting controller")

		wg.Add(c.maxConcurrentReconciles)
		for i := 0; i < c.maxConcurrentReconciles; i++ {
			go func() {
				defer wg.Done()
				for c.processNextWorkItem(ctx) {
				}
			}()
		}

		c.started = true
		return nil
	}()
	if err != nil {
		return fmt.Errorf("controller stopped: %w", err)
	}

	<-ctx.Done()
	c.getLogger().Info("stopping controller")
	wg.Wait()
	return nil
}

func (c *genericController) processNextWorkItem(ctx context.Context) bool {
	obj, shutdown := c.queue.Get()
	if shutdown {
		return false
	}

	defer c.queue.Done(obj)

	ctrlmetrics.ActiveWorkers.WithLabelValues(c.name).Add(1)
	defer ctrlmetrics.ActiveWorkers.WithLabelValues(c.name).Add(-1)

	c.reconcileHandler(ctx, obj)
	return true
}

func (c *genericController) reconcileHandler(ctx context.Context, obj interface{}) {
	reconcileStartTS := time.Now()
	defer func() {
		c.updateMetrics(time.Since(reconcileStartTS))
	}()

	req, ok := obj.(reconcile.Request)
	if !ok {
		c.queue.Forget(obj)
		c.getLogger().Errorf("invalid queue item: %+v", req)
		return
	}

	reconcileID := uuid.NewUUID()

	logger := c.getLogger().WithField("reconcileID", reconcileID)
	ctx = addReconcileID(ctx, reconcileID)

	result, err := c.reconcile(ctx, req)
	switch {
	case err != nil:
		if errors.Is(err, reconcile.TerminalError(nil)) {
			ctrlmetrics.TerminalReconcileErrors.WithLabelValues(c.name).Inc()
		} else {
			c.queue.AddRateLimited(req)
		}
		ctrlmetrics.ReconcileErrors.WithLabelValues(c.name).Inc()
		ctrlmetrics.ReconcileTotal.WithLabelValues(c.name, metricLabelError).Inc()
		logger.WithError(err).Errorf("could not reconcile req: %s", req.String())
	case result.RequeueAfter > 0:
		c.queue.Forget(obj)
		c.queue.AddAfter(req, result.RequeueAfter)
		ctrlmetrics.ReconcileTotal.WithLabelValues(c.name, metricLabelRequeueAfter).Inc()
	case result.Requeue:
		c.queue.AddRateLimited(req)
		ctrlmetrics.ReconcileTotal.WithLabelValues(c.name, metricLabelRequeue).Inc()
	default:
		c.queue.Forget(obj)
		ctrlmetrics.ReconcileTotal.WithLabelValues(c.name, metricLabelSuccess).Inc()
	}
}

func (c *genericController) reconcile(ctx context.Context, req reconcile.Request) (_ reconcile.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			for _, fn := range utilruntime.PanicHandlers {
				fn(r)
			}
			err = fmt.Errorf("panic: %v [recovered]", r)
			return
		}
	}()
	return c.reconciler.Reconcile(ctx, req)
}

func (c *genericController) initMetrics() {
	ctrlmetrics.ActiveWorkers.WithLabelValues(c.name).Set(0)
	ctrlmetrics.ReconcileErrors.WithLabelValues(c.name).Add(0)
	ctrlmetrics.ReconcileTotal.WithLabelValues(c.name, metricLabelError).Add(0)
	ctrlmetrics.ReconcileTotal.WithLabelValues(c.name, metricLabelRequeueAfter).Add(0)
	ctrlmetrics.ReconcileTotal.WithLabelValues(c.name, metricLabelRequeue).Add(0)
	ctrlmetrics.ReconcileTotal.WithLabelValues(c.name, metricLabelSuccess).Add(0)
	ctrlmetrics.WorkerCount.WithLabelValues(c.name).Set(float64(c.maxConcurrentReconciles))
}

func (c *genericController) getLogger() *logrus.Entry {
	return logrus.WithContext(c.ctx).WithField("name", c.name)
}

func (c *genericController) updateMetrics(reconcileTime time.Duration) {
	ctrlmetrics.ReconcileTime.WithLabelValues(c.name).Observe(reconcileTime.Seconds())
}

type reconcileIDKey struct{}

func addReconcileID(ctx context.Context, reconcileID types.UID) context.Context {
	return context.WithValue(ctx, reconcileIDKey{}, reconcileID)
}

func APIExists(restMapper meta.RESTMapper, gvk schema.GroupVersionKind) bool {
	_, err := restMapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	return err == nil
}
