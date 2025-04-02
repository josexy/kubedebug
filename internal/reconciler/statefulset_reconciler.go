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

type StatefulsetReconciler struct {
	*CommonConfig
	Scheme *runtime.Scheme
	client.Client
}

func (r *StatefulsetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var sts appsv1.StatefulSet
	err := r.Get(ctx, req.NamespacedName, &sts)
	if err != nil {
		if apierrs.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var containers []map[string]string
	for _, container := range sts.Spec.Template.Spec.Containers {
		items := make(map[string]string)
		items["name"] = container.Name
		items["image"] = container.Image
		containers = append(containers, items)
	}

	r.Logger.Info("Reconcile Statefulset",
		logx.String("req", req.String()),
		logx.Int32("replicas", sts.Status.Replicas),
		logx.Int32("ready", sts.Status.ReadyReplicas),
		logx.Int32("available", sts.Status.AvailableReplicas),
		logx.Any("labels", sts.Spec.Template.Labels),
		logx.Any("containers", containers),
	)

	if r.OnlyWatch {
		return ctrl.Result{}, nil
	}

	return r.reconcileStatefulset(ctx, &sts)
}

func (r *StatefulsetReconciler) reconcileStatefulset(ctx context.Context, sts *appsv1.StatefulSet) (ctrl.Result, error) {
	if result, err := r.reconcileService(ctx, sts); err != nil {
		return result, err
	}
	if result, err := r.reconcilePod(ctx, sts); err != nil {
		return result, err
	}
	return ctrl.Result{}, nil
}

func (r *StatefulsetReconciler) reconcilePod(ctx context.Context, sts *appsv1.StatefulSet) (ctrl.Result, error) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.Get(ctx, client.ObjectKeyFromObject(sts), sts); err != nil {
			return err
		}
		fillPodTemplate(&sts.Spec.Template)
		return r.Update(ctx, sts)
	})
	return ctrl.Result{}, err
}

func (r *StatefulsetReconciler) reconcileService(ctx context.Context, sts *appsv1.StatefulSet) (ctrl.Result, error) {
	return reconcileService(ctx, r.Client, sts, r.Scheme, sts.Spec.Template.Labels, r.Logger)
}

func (r *StatefulsetReconciler) Setup(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.StatefulSet{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
