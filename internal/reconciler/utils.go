package reconciler

import (
	"context"
	"fmt"

	"github.com/josexy/kubedebug/internal/config"
	"github.com/josexy/logx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	dlvExeVolumeName   = "dlv-exe-volume"
	debugExeVolumeName = "debug-exe-volume"

	dlvExeVolumeMountPath   = "/data/dlv"
	debugExeVolumeMountPath = "/data/debug-binary"
)

func findContainer(template *corev1.PodTemplateSpec, name string) (corev1.Container, int) {
	for i, container := range template.Spec.Containers {
		if container.Name == name {
			return container, i
		}
	}
	return corev1.Container{}, -1
}

func findVolume(template *corev1.PodTemplateSpec, name string) (corev1.Volume, int) {
	for i, volume := range template.Spec.Volumes {
		if volume.Name == name {
			return volume, i
		}
	}
	return corev1.Volume{}, -1
}

func fillPodTemplate(template *corev1.PodTemplateSpec) {
	if container, index := findContainer(template, config.GetConfig().ContainerName); index != -1 {
		container.Command = []string{dlvExeVolumeMountPath}
		execPath := config.GetConfig().CommandArgs[0]
		if config.GetConfig().DebugExePath != "" {
			execPath = debugExeVolumeMountPath
		}
		container.Command = append(container.Command, "--listen=:2345", "--headless=true", "--api-version=2", "--log", "exec")
		container.Args = []string{execPath}
		if len(config.GetConfig().CommandArgs) > 1 {
			container.Args = append(container.Args, "--")
			container.Args = append(container.Args, config.GetConfig().CommandArgs[1:]...)
		}

		if _, index := findVolume(template, dlvExeVolumeName); index == -1 {
			template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
				Name: dlvExeVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: config.GetConfig().DlvExePath,
					},
				},
			})
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name:      dlvExeVolumeName,
				MountPath: dlvExeVolumeMountPath,
			})
		} else {
			template.Spec.Volumes[index].VolumeSource.HostPath.Path = config.GetConfig().DlvExePath
		}
		if config.GetConfig().DebugExePath != "" {
			if _, index := findVolume(template, debugExeVolumeName); index == -1 {
				template.Spec.Volumes = append(template.Spec.Volumes, corev1.Volume{
					Name: debugExeVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: config.GetConfig().DebugExePath,
						},
					},
				})
				container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
					Name:      debugExeVolumeName,
					MountPath: debugExeVolumeMountPath,
				})
			} else {
				template.Spec.Volumes[index].VolumeSource.HostPath.Path = config.GetConfig().DebugExePath
			}
		}
		template.Spec.Containers[index] = container
	}
}

func reconcileService(ctx context.Context, c client.Client,
	object client.Object, scheme *runtime.Scheme,
	labels map[string]string, logger logx.Logger) (ctrl.Result, error) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%d-dlv", object.GetName(), config.GetConfig().Name, config.GetConfig().NodePort),
			Namespace: object.GetNamespace(),
		},
	}
	var result ctrl.Result
	_, err := ctrl.CreateOrUpdate(ctx, c, svc, func() error {
		logger.Info("Creating or Updating Service", logx.String("svc", svc.Name))
		if err := ctrl.SetControllerReference(object, svc, scheme); err != nil {
			result = ctrl.Result{Requeue: true}
			return nil
		}
		svc.Labels = labels
		svc.Spec = corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:       "dlv",
					Protocol:   corev1.ProtocolTCP,
					Port:       2345,
					TargetPort: intstr.FromInt(2345),
					NodePort:   int32(config.GetConfig().NodePort),
				},
			},
			Selector: labels,
		}
		return nil
	})
	return result, err
}
