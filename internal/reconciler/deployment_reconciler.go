package reconciler

import (
	"context"

	"github.com/josexy/logx"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type DeploymentReconciler struct {
	*CommonConfig
	Scheme *runtime.Scheme
	client.Client
}

func (r *DeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var dep appsv1.Deployment
	err := r.Get(ctx, req.NamespacedName, &dep)
	if err != nil {
		if apierrs.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var containers []map[string]string
	for _, container := range dep.Spec.Template.Spec.Containers {
		items := make(map[string]string)
		items["name"] = container.Name
		items["image"] = container.Image
		containers = append(containers, items)
	}

	r.Logger.Info("Reconcile Deployment",
		logx.String("req", req.String()),
		logx.Int32("replicas", dep.Status.Replicas),
		logx.Int32("ready", dep.Status.ReadyReplicas),
		logx.Int32("available", dep.Status.AvailableReplicas),
		logx.Any("labels", dep.Spec.Template.Labels),
		logx.Any("containers", containers),
	)

	if r.OnlyWatch {
		return ctrl.Result{}, nil
	}

	return r.reconcileDeployment(ctx, &dep)
}

func (r *DeploymentReconciler) reconcileDeployment(ctx context.Context, dep *appsv1.Deployment) (ctrl.Result, error) {
	if result, err := r.reconcileService(ctx, dep); err != nil {
		return result, err
	}
	if result, err := r.reconcilePod(ctx, dep); err != nil {
		return result, err
	}
	return ctrl.Result{}, nil
}

func (r *DeploymentReconciler) reconcilePod(ctx context.Context, dep *appsv1.Deployment) (ctrl.Result, error) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Get(ctx, client.ObjectKeyFromObject(dep), dep); err != nil {
			return err
		}
		fillPodTemplate(&dep.Spec.Template)
		return r.Update(ctx, dep)
	})
	return ctrl.Result{}, err
}

func (r *DeploymentReconciler) reconcileService(ctx context.Context, dep *appsv1.Deployment) (ctrl.Result, error) {
	return reconcileService(ctx, r.Client, dep, r.Scheme, dep.Spec.Template.Labels, r.Logger)
}

func (r *DeploymentReconciler) Setup(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.Deployment{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
