package controllers

import (
	"fmt"

	dbv1 "github.com/PayU/Redis-Operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *RedisOperatorReconciler) leaderPod(redisOperator *dbv1.RedisOperator, number int) (corev1.Pod, error) {
	podLabels := make(map[string]string)
	redisContainerEnvVariables := []corev1.EnvVar{
		{Name: "PORT", Value: "6379"},
	}

	podLabels["app"] = redisOperator.Spec.PodLabelSelector.App
	podLabels["redis-node-role"] = "leader"

	for _, envStruct := range redisOperator.Spec.RedisContainerEnvVariables {
		if envStruct.Name != "PORT" {
			redisContainerEnvVariables = append(redisContainerEnvVariables, corev1.EnvVar{
				Name:  envStruct.Name,
				Value: envStruct.Value,
			})
		}
	}

	imagePullSecrets := []corev1.LocalObjectReference{
		{Name: redisOperator.Spec.ImagePullSecrets},
	}

	containers := []corev1.Container{
		{
			Name:  "redis-master",
			Image: redisOperator.Spec.Image,
			Env:   redisContainerEnvVariables,
			Ports: []corev1.ContainerPort{
				{ContainerPort: 6379, Name: "redis", Protocol: "TCP"},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "redis-node-configuration", MountPath: "/usr/local/etc/redis"},
			},
		},
	}

	volumes := []corev1.Volume{
		{
			Name: "redis-node-configuration",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "redis-node-settings-config-map",
					},
				},
			},
		},
	}

	var affinity *corev1.Affinity = nil
	if redisOperator.Spec.Affinity != (dbv1.TopologyKeys{}) {
		affinity = &corev1.Affinity{PodAntiAffinity: &corev1.PodAntiAffinity{}}
		if redisOperator.Spec.Affinity.HostTopologyKey != "" {
			affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{Key: "app", Operator: metav1.LabelSelectorOpIn, Values: []string{redisOperator.Spec.PodLabelSelector.App}},
						},
					},
					TopologyKey: redisOperator.Spec.Affinity.HostTopologyKey,
				},
			}
		}
		if redisOperator.Spec.Affinity.ZoneTopologyKey != "" {
			affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution = []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{Key: "redis-node-role", Operator: metav1.LabelSelectorOpIn, Values: []string{"leader"}},
							},
						},
						TopologyKey: redisOperator.Spec.Affinity.ZoneTopologyKey,
					},
				},
			}
		}
	}

	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("redis-leader-%d", number),
			Namespace:   redisOperator.ObjectMeta.Namespace,
			Labels:      podLabels,
			Annotations: redisOperator.Spec.PodAnnotations,
		},
		Spec: corev1.PodSpec{
			ImagePullSecrets: imagePullSecrets,
			Containers:       containers,
			Volumes:          volumes,
			Affinity:         affinity,
		},
	}

	if err := ctrl.SetControllerReference(redisOperator, &pod, r.Scheme); err != nil {
		return pod, err
	}

	return pod, nil
}

func (r *RedisOperatorReconciler) serviceResource(redisOperator *dbv1.RedisOperator) (corev1.Service, error) {
	r.Log.Info("creating cluster service")

	serviceSelector := make(map[string]string)
	serviceSelector["app"] = redisOperator.Spec.PodLabelSelector.App

	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-cluster-service",
			Namespace: redisOperator.ObjectMeta.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "redis-client-port",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
				},
			},
			Selector: serviceSelector,
		},
	}

	if err := ctrl.SetControllerReference(redisOperator, &service, r.Scheme); err != nil {
		return service, err
	}

	return service, nil
}

func (r *RedisOperatorReconciler) headlessServiceResource(redisOperator *dbv1.RedisOperator) (corev1.Service, error) {
	r.Log.Info("creating cluster headless service")

	serviceSelector := make(map[string]string)
	serviceSelector["app"] = redisOperator.Spec.PodLabelSelector.App

	headlessService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-cluster-headless-service",
			Namespace: redisOperator.ObjectMeta.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "redis-client-port",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
				},
			},
			Selector:  serviceSelector,
			ClusterIP: "None",
		},
	}

	if err := ctrl.SetControllerReference(redisOperator, &headlessService, r.Scheme); err != nil {
		return headlessService, err
	}

	return headlessService, nil
}

func (r *RedisOperatorReconciler) createSettingsConfigMap(redisOperator *dbv1.RedisOperator) (corev1.ConfigMap, error) {
	r.Log.Info("creating cluster config map")

	data := make(map[string]string)
	data["redis.conf"] = redisConf

	redisSettingConfigMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-node-settings-config-map",
			Namespace: redisOperator.ObjectMeta.Namespace,
		},
		Data: data,
	}

	if err := ctrl.SetControllerReference(redisOperator, &redisSettingConfigMap, r.Scheme); err != nil {
		return redisSettingConfigMap, err
	}

	return redisSettingConfigMap, nil
}
