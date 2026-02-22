package execution

import (
	"fmt"
	"strings"

	executorv1 "github.com/rakeshavasarala/script-executor/gen/go/proto/executor/v1"
	"github.com/rakeshavasarala/script-executor/internal/config"
	"github.com/rakeshavasarala/script-executor/internal/script"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"google.golang.org/protobuf/types/known/structpb"
)

// BuildContext builds ExecutionContext from request parameters.
func BuildContext(
	params *structpb.Struct,
	execCtx *executorv1.ExecutionContext,
	scriptContent string,
	source *script.Source,
	scriptHash string,
	image string,
	imagePullPolicy string,
	imagePullSecret string,
	cfg *config.Config,
) (*Context, error) {
	ctx := &Context{
		Script:       scriptContent,
		ScriptSource: source,
		ScriptHash:   scriptHash,
		Image:        image,
		ImagePullPolicy: corev1.PullPolicy(parsePullPolicy(getString(params, "image_pull_policy", ""))),
		ImagePullSecret: imagePullSecret,
		Interpreter:  getString(params, "interpreter", "/bin/bash"),
		WorkingDir:   getString(params, "working_dir", "/workspace"),
		Stdin:        getString(params, "stdin", ""),
		Env:          make(map[string]string),
		EnvFromSecret: make(map[string]SecretKeyRef),
		EnvFromConfigMap: make(map[string]ConfigMapKeyRef),
		NodeSelector: make(map[string]string),
		Labels:       make(map[string]string),
		Annotations:  make(map[string]string),
		ServiceAccount: cfg.ScriptExecutor.Kubernetes.ServiceAccount,
		TTLSecondsAfterFinished: int32(cfg.ScriptExecutor.Kubernetes.JobDefaults.TTLSecondsAfterFinished),
		BackoffLimit: int32(cfg.ScriptExecutor.Kubernetes.JobDefaults.BackoffLimit),
	}

	if ctx.ImagePullPolicy == "" {
		ctx.ImagePullPolicy = corev1.PullPolicy(cfg.ScriptExecutor.Image.DefaultImagePullPolicy)
	}

	// Execution context
	if execCtx != nil {
		ctx.ExecutionID = execCtx.ExecutionId
		ctx.RunbookID = execCtx.RunbookId
		ctx.User = execCtx.User
	}

	// Args
	ctx.Args = getStringSlice(params, "args")
	if ctx.Args == nil {
		ctx.Args = []string{}
	}

	// Timeout
	timeoutStr := getString(params, "timeout", cfg.ScriptExecutor.Security.DefaultTimeout)
	dur, err := parseDuration(timeoutStr)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout %q: %w", timeoutStr, err)
	}
	ctx.Timeout = dur
	if ctx.Timeout == 0 {
		ctx.Timeout = cfg.DefaultTimeout()
	}

	// Env (literal)
	if envMap := getMap(params, "env"); envMap != nil && envMap.Fields != nil {
		for k, v := range envMap.Fields {
			if v != nil {
				ctx.Env[k] = v.GetStringValue()
			}
		}
	}

	// env_from_secret
	if envSec := getMap(params, "env_from_secret"); envSec != nil && envSec.Fields != nil {
		for name, v := range envSec.Fields {
			if v == nil || v.GetStructValue() == nil {
				continue
			}
			s := v.GetStructValue()
			ctx.EnvFromSecret[name] = SecretKeyRef{
				SecretName: getString(s, "secret_name", ""),
				Key:        getString(s, "key", ""),
				Optional:   getBool(s, "optional"),
			}
		}
	}

	// env_from_configmap
	if envCM := getMap(params, "env_from_configmap"); envCM != nil && envCM.Fields != nil {
		for name, v := range envCM.Fields {
			if v == nil || v.GetStructValue() == nil {
				continue
			}
			s := v.GetStructValue()
			ctx.EnvFromConfigMap[name] = ConfigMapKeyRef{
				ConfigMapName: getString(s, "configmap_name", ""),
				Key:           getString(s, "key", ""),
				Optional:      getBool(s, "optional"),
			}
		}
	}

	// secret_env_all, configmap_env_all
	ctx.SecretEnvAll = getStringSlice(params, "secret_env_all")
	ctx.ConfigMapEnvAll = getStringSlice(params, "configmap_env_all")

	// volumes_from_secret
	if volList := getList(params, "volumes_from_secret"); volList != nil {
		for _, v := range volList {
			if v == nil || v.GetStructValue() == nil {
				continue
			}
			s := v.GetStructValue()
			vol := SecretVolume{
				SecretName: getString(s, "secret_name", ""),
				MountPath:  getString(s, "mount_path", ""),
				Optional:   getBool(s, "optional"),
			}
			if items := getList(s, "items"); items != nil {
				for _, it := range items {
					if it == nil || it.GetStructValue() == nil {
						continue
					}
					is := it.GetStructValue()
					vol.Items = append(vol.Items, KeyToPath{
						Key:  getString(is, "key", ""),
						Path: getString(is, "path", ""),
					})
				}
			}
			ctx.VolumesFromSecret = append(ctx.VolumesFromSecret, vol)
		}
	}

	// volumes_from_configmap
	if volList := getList(params, "volumes_from_configmap"); volList != nil {
		for _, v := range volList {
			if v == nil || v.GetStructValue() == nil {
				continue
			}
			s := v.GetStructValue()
			vol := ConfigMapVolume{
				ConfigMapName: getString(s, "configmap_name", ""),
				MountPath:     getString(s, "mount_path", ""),
				Optional:      getBool(s, "optional"),
			}
			if items := getList(s, "items"); items != nil {
				for _, it := range items {
					if it == nil || it.GetStructValue() == nil {
						continue
					}
					is := it.GetStructValue()
					vol.Items = append(vol.Items, KeyToPath{
						Key:  getString(is, "key", ""),
						Path: getString(is, "path", ""),
					})
				}
			}
			ctx.VolumesFromConfigMap = append(ctx.VolumesFromConfigMap, vol)
		}
	}

	// node_selector
	if ns := getMap(params, "node_selector"); ns != nil && ns.Fields != nil {
		for k, v := range ns.Fields {
			if v != nil {
				ctx.NodeSelector[k] = v.GetStringValue()
			}
		}
	}

	// Resources
	ctx.Resources = buildResources(params, cfg)

	// TTL, backoff
	if ttl := getInt(params, "ttl_seconds_after_finished"); ttl > 0 {
		ctx.TTLSecondsAfterFinished = int32(ttl)
	}
	if bl := getInt(params, "backoff_limit"); bl >= 0 {
		ctx.BackoffLimit = int32(bl)
		if ctx.BackoffLimit > 3 {
			ctx.BackoffLimit = 3
		}
	}

	// labels, annotations
	if labels := getMap(params, "labels"); labels != nil && labels.Fields != nil {
		for k, v := range labels.Fields {
			if v != nil {
				ctx.Labels[k] = v.GetStringValue()
			}
		}
	}
	if ann := getMap(params, "annotations"); ann != nil && ann.Fields != nil {
		for k, v := range ann.Fields {
			if v != nil {
				ctx.Annotations[k] = v.GetStringValue()
			}
		}
	}

	// priority_class_name
	ctx.PriorityClassName = getString(params, "priority_class_name", "")

	return ctx, nil
}

func buildResources(params *structpb.Struct, cfg *config.Config) corev1.ResourceRequirements {
	req := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	defaultReqs := cfg.ScriptExecutor.Kubernetes.DefaultResources
	defaultLimits := cfg.ScriptExecutor.Kubernetes.MaxResources.Limits

	// Defaults
	if defaultReqs.Requests.CPU != "" {
		req.Requests[corev1.ResourceCPU] = resource.MustParse(defaultReqs.Requests.CPU)
	}
	if defaultReqs.Requests.Memory != "" {
		req.Requests[corev1.ResourceMemory] = resource.MustParse(defaultReqs.Requests.Memory)
	}
	if defaultReqs.Limits.CPU != "" {
		req.Limits[corev1.ResourceCPU] = resource.MustParse(defaultReqs.Limits.CPU)
	}
	if defaultReqs.Limits.Memory != "" {
		req.Limits[corev1.ResourceMemory] = resource.MustParse(defaultReqs.Limits.Memory)
	}
	if defaultReqs.Limits.EphemeralStorage != "" {
		req.Limits[corev1.ResourceEphemeralStorage] = resource.MustParse(defaultReqs.Limits.EphemeralStorage)
	}

	// Override from params
	if res := getMap(params, "resources"); res != nil && res.Fields != nil {
		if reqs := getMap(res, "requests"); reqs != nil && reqs.Fields != nil {
			if c := getString(reqs, "cpu", ""); c != "" {
				req.Requests[corev1.ResourceCPU] = resource.MustParse(c)
			}
			if m := getString(reqs, "memory", ""); m != "" {
				req.Requests[corev1.ResourceMemory] = resource.MustParse(m)
			}
		}
		if lims := getMap(res, "limits"); lims != nil && lims.Fields != nil {
			if c := getString(lims, "cpu", ""); c != "" {
				req.Limits[corev1.ResourceCPU] = resource.MustParse(c)
			}
			if m := getString(lims, "memory", ""); m != "" {
				req.Limits[corev1.ResourceMemory] = resource.MustParse(m)
			}
			if e := getString(lims, "ephemeral_storage", ""); e != "" {
				req.Limits[corev1.ResourceEphemeralStorage] = resource.MustParse(e)
			}
		}
	}

	// Cap at max
	if defaultLimits.CPU != "" {
		maxCPU := resource.MustParse(defaultLimits.CPU)
		if lim, ok := req.Limits[corev1.ResourceCPU]; ok && lim.Cmp(maxCPU) > 0 {
			req.Limits[corev1.ResourceCPU] = maxCPU
		}
	}
	if defaultLimits.Memory != "" {
		maxMem := resource.MustParse(defaultLimits.Memory)
		if lim, ok := req.Limits[corev1.ResourceMemory]; ok && lim.Cmp(maxMem) > 0 {
			req.Limits[corev1.ResourceMemory] = maxMem
		}
	}

	return req
}

func sanitizeLabel(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}
