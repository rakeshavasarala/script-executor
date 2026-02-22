package execution

import (
	"fmt"

	"github.com/rakeshavasarala/script-executor/internal/config"
	"github.com/rakeshavasarala/script-executor/internal/script"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// JobBuilder builds Kubernetes Jobs for script execution.
type JobBuilder struct {
	config *config.Config
}

// NewJobBuilder creates a JobBuilder.
func NewJobBuilder(cfg *config.Config) *JobBuilder {
	return &JobBuilder{config: cfg}
}

// Build creates a Job from ExecutionContext.
func (b *JobBuilder) Build(ctx *Context) (*batchv1.Job, error) {
	jobName := fmt.Sprintf("script-exec-%s", ctx.ExecutionID)
	if jobName == "script-exec-" {
		jobName = fmt.Sprintf("script-exec-%d", metav1.Now().Unix())
	}

	secCfg := b.config.ScriptExecutor.Security
	k8sCfg := b.config.ScriptExecutor.Kubernetes

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: k8sCfg.Namespace,
			Labels: map[string]string{
				"executor":     "script",
				"execution-id": ctx.ExecutionID,
				"runbook-id":   ctx.RunbookID,
				"user":         sanitizeLabel(ctx.User),
				"managed-by":   "opscontrolroom",
			},
			Annotations: map[string]string{
				"script-hash":  ctx.ScriptHash,
				"execution-id": ctx.ExecutionID,
				"runbook-id":   ctx.RunbookID,
				"user":         ctx.User,
				"image":        ctx.Image,
				"created-by":   "script-executor",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            ptr.To(ctx.BackoffLimit),
			TTLSecondsAfterFinished: ptr.To(ctx.TTLSecondsAfterFinished),
			ActiveDeadlineSeconds:   ptr.To(int64(ctx.Timeout.Seconds())),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"executor":     "script",
						"execution-id": ctx.ExecutionID,
					},
					Annotations: ctx.Annotations,
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					ServiceAccountName: ctx.ServiceAccount,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: ptr.To(secCfg.RunAsNonRoot),
						RunAsUser:    ptr.To(secCfg.RunAsUser),
						FSGroup:      ptr.To(secCfg.FSGroup),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					NodeSelector:      ctx.NodeSelector,
					Tolerations:       ctx.Tolerations,
					Affinity:         ctx.Affinity,
					PriorityClassName: ctx.PriorityClassName,
					Containers:       []corev1.Container{b.buildContainer(ctx)},
					Volumes:          b.buildVolumes(ctx),
				},
			},
		},
	}

	for k, v := range ctx.Labels {
		job.Labels[k] = v
	}
	for k, v := range ctx.Annotations {
		job.Annotations[k] = v
	}

	if ctx.ImagePullSecret != "" {
		job.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
			{Name: ctx.ImagePullSecret},
		}
	}

	return job, nil
}

func (b *JobBuilder) buildContainer(ctx *Context) corev1.Container {
	secCfg := b.config.ScriptExecutor.Security

	container := corev1.Container{
		Name:            "script",
		Image:           ctx.Image,
		ImagePullPolicy: ctx.ImagePullPolicy,
		WorkingDir:      ctx.WorkingDir,
		Env:             b.buildEnvVars(ctx),
		EnvFrom:         b.buildEnvFrom(ctx),
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: ptr.To(false),
			ReadOnlyRootFilesystem:   ptr.To(secCfg.ReadOnlyRootFilesystem),
			RunAsNonRoot:             ptr.To(secCfg.RunAsNonRoot),
			RunAsUser:                ptr.To(secCfg.RunAsUser),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		Resources:    ctx.Resources,
		VolumeMounts: b.buildVolumeMounts(ctx),
		Stdin:        ctx.Stdin != "",
		StdinOnce:    ctx.Stdin != "",
	}

	// Command and args depend on script source
	if ctx.ScriptSource != nil && ctx.ScriptSource.Type == script.SourcePath {
		container.Command = []string{ctx.Interpreter, ctx.ScriptSource.Path}
		container.Args = ctx.Args
	} else {
		container.Command = []string{ctx.Interpreter, "-c", ctx.Script}
		container.Args = ctx.Args
	}

	return container
}

func (b *JobBuilder) buildEnvVars(ctx *Context) []corev1.EnvVar {
	var env []corev1.EnvVar
	for k, v := range ctx.Env {
		env = append(env, corev1.EnvVar{Name: k, Value: v})
	}
	for name, ref := range ctx.EnvFromSecret {
		if ref.SecretName == "" || ref.Key == "" {
			continue
		}
		env = append(env, corev1.EnvVar{
			Name: name,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: ref.SecretName},
					Key:                  ref.Key,
					Optional:             ptr.To(ref.Optional),
				},
			},
		})
	}
	for name, ref := range ctx.EnvFromConfigMap {
		if ref.ConfigMapName == "" || ref.Key == "" {
			continue
		}
		env = append(env, corev1.EnvVar{
			Name: name,
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: ref.ConfigMapName},
					Key:                  ref.Key,
					Optional:             ptr.To(ref.Optional),
				},
			},
		})
	}
	return env
}

func (b *JobBuilder) buildEnvFrom(ctx *Context) []corev1.EnvFromSource {
	var envFrom []corev1.EnvFromSource
	for _, name := range ctx.SecretEnvAll {
		if name != "" {
			envFrom = append(envFrom, corev1.EnvFromSource{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: name},
				},
			})
		}
	}
	for _, name := range ctx.ConfigMapEnvAll {
		if name != "" {
			envFrom = append(envFrom, corev1.EnvFromSource{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: name},
				},
			})
		}
	}
	return envFrom
}

func (b *JobBuilder) buildVolumes(ctx *Context) []corev1.Volume {
	ephemeralLimit := "1Gi"
	if q, ok := ctx.Resources.Limits[corev1.ResourceEphemeralStorage]; ok {
		ephemeralLimit = q.String()
	}

	volumes := []corev1.Volume{
		{
			Name: "workspace",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: ptr.To(resource.MustParse(ephemeralLimit)),
				},
			},
		},
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{
					SizeLimit: ptr.To(resource.MustParse("100Mi")),
				},
			},
		},
	}

	// approved-scripts for script_path
	if ctx.ScriptSource != nil && ctx.ScriptSource.Type == script.SourcePath {
		volumes = append(volumes, corev1.Volume{
			Name: "scripts",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: script.ApprovedScriptsConfigMap,
					},
				},
			},
		})
	}

	for i, v := range ctx.VolumesFromSecret {
		vol := corev1.Volume{
			Name: fmt.Sprintf("secret-%d", i),
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  v.SecretName,
					Optional:    ptr.To(v.Optional),
				},
			},
		}
		if len(v.Items) > 0 {
			vol.VolumeSource.Secret.Items = make([]corev1.KeyToPath, 0, len(v.Items))
			for _, it := range v.Items {
				kp := corev1.KeyToPath{Key: it.Key, Path: it.Path}
				if it.Mode != nil {
					kp.Mode = it.Mode
				}
				vol.VolumeSource.Secret.Items = append(vol.VolumeSource.Secret.Items, kp)
			}
		}
		volumes = append(volumes, vol)
	}

	for i, v := range ctx.VolumesFromConfigMap {
		vol := corev1.Volume{
			Name: fmt.Sprintf("configmap-%d", i),
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: v.ConfigMapName,
					},
					Optional: ptr.To(v.Optional),
				},
			},
		}
		if len(v.Items) > 0 {
			vol.VolumeSource.ConfigMap.Items = make([]corev1.KeyToPath, 0, len(v.Items))
			for _, it := range v.Items {
				kp := corev1.KeyToPath{Key: it.Key, Path: it.Path}
				if it.Mode != nil {
					kp.Mode = it.Mode
				}
				vol.VolumeSource.ConfigMap.Items = append(vol.VolumeSource.ConfigMap.Items, kp)
			}
		}
		volumes = append(volumes, vol)
	}

	return volumes
}

func (b *JobBuilder) buildVolumeMounts(ctx *Context) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{
		{Name: "workspace", MountPath: "/workspace"},
		{Name: "tmp", MountPath: "/tmp"},
	}

	if ctx.ScriptSource != nil && ctx.ScriptSource.Type == script.SourcePath {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      "scripts",
			MountPath: "/scripts",
			ReadOnly:  true,
		})
	}

	for i, v := range ctx.VolumesFromSecret {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      fmt.Sprintf("secret-%d", i),
			MountPath: v.MountPath,
			ReadOnly:  true,
		})
	}
	for i, v := range ctx.VolumesFromConfigMap {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      fmt.Sprintf("configmap-%d", i),
			MountPath: v.MountPath,
			ReadOnly:  true,
		})
	}

	return mounts
}
