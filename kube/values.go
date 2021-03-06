package kube

import (
	"fmt"
	"strings"

	"github.com/SUSE/fissile/helm"
	"github.com/SUSE/fissile/model"
)

// MakeValues returns a Mapping with all default values for the Helm chart
func MakeValues(settings ExportSettings) (helm.Node, error) {
	env := helm.NewMapping()
	secrets := helm.NewMapping()
	generated := helm.NewMapping()

	for name, cv := range model.MakeMapOfVariables(settings.RoleManifest) {
		if strings.HasPrefix(name, "KUBE_SIZING_") || cv.Type == model.CVTypeEnv {
			continue
		}
		// Immutable secrets that are generated cannot be overridden by the user
		// and any default value would always be ignored.
		if cv.Immutable && cv.Generator != nil {
			continue
		}

		var value interface{}
		if !cv.Secret || cv.Generator == nil {
			var ok bool
			if ok, value = cv.Value(settings.Defaults); !ok {
				value = nil
			}
		}
		comment := cv.Description
		if cv.Example != "" && cv.Example != value {
			comment += fmt.Sprintf("\nExample: %s", cv.Example)
		}

		if cv.Secret {
			thisValue := "This value"
			if cv.Generator != nil {
				comment += "\n" + thisValue + " uses a generated default."
				thisValue = "It"
			}
			if cv.Immutable {
				comment += "\n" + thisValue + " is immutable and must not be changed once set."
			}
			if cv.Generator == nil {
				secrets.Add(name, helm.NewNode(value, helm.Comment(comment)))
			} else {
				generated.Add(name, helm.NewNode(value, helm.Comment(comment)))
			}
		} else {
			env.Add(name, helm.NewNode(value, helm.Comment(comment)))
		}
	}
	secrets.Sort()
	secrets.Merge(generated.Sort())

	memSizing := helm.NewMapping()
	memSizing.Add("requests", false, helm.Comment("Flag to activate memory requests"))
	memSizing.Add("limits", false, helm.Comment("Flag to activate memory limits"))

	cpuSizing := helm.NewMapping()
	cpuSizing.Add("requests", false, helm.Comment("Flag to activate cpu requests"))
	cpuSizing.Add("limits", false, helm.Comment("Flag to activate cpu limits"))

	sizing := helm.NewMapping()
	sizing.Add("HA", false, helm.Comment("Flag to activate high-availability mode"))
	sizing.Add("memory", memSizing, helm.Comment("Global memory configuration"))
	sizing.Add("cpu", cpuSizing, helm.Comment("Global CPU configuration"))

	for _, role := range settings.RoleManifest.Roles {
		if role.IsDevRole() || role.Run.FlightStage == model.FlightStageManual {
			continue
		}

		entry := helm.NewMapping()
		var comment string
		if role.Run.Scaling.Min == role.Run.Scaling.Max {
			comment = fmt.Sprintf("The %s role cannot be scaled.", role.Name)
		} else {
			comment = fmt.Sprintf("The %s role can scale between %d and %d instances.",
				role.Name, role.Run.Scaling.Min, role.Run.Scaling.Max)

			if role.Run.Scaling.MustBeOdd {
				comment += "\nThe instance count must be an odd number (not divisible by 2)."
			}
			if role.Run.Scaling.HA != role.Run.Scaling.Min {
				comment += fmt.Sprintf("\nFor high availability it needs at least %d instances.",
					role.Run.Scaling.HA)
			}
		}
		entry.Add("count", role.Run.Scaling.Min, helm.Comment(comment))
		if settings.UseMemoryLimits {
			var request helm.Node
			if role.Run.Memory.Request == nil {
				request = helm.NewNode(nil)
			} else {
				request = helm.NewNode(int(*role.Run.Memory.Request))
			}
			var limit helm.Node
			if role.Run.Memory.Limit == nil {
				limit = helm.NewNode(nil)
			} else {
				limit = helm.NewNode(int(*role.Run.Memory.Limit))
			}

			entry.Add("memory", helm.NewMapping(
				"request", request,
				"limit", limit),
				helm.Comment("Unit [MiB]"))
		}
		if settings.UseCPULimits {
			var request helm.Node
			if role.Run.CPU.Request == nil {
				request = helm.NewNode(nil)
			} else {
				request = helm.NewNode(1000. * *role.Run.CPU.Request)
			}
			var limit helm.Node
			if role.Run.CPU.Limit == nil {
				limit = helm.NewNode(nil)
			} else {
				limit = helm.NewNode(1000. * *role.Run.CPU.Limit)
			}

			entry.Add("cpu", helm.NewMapping(
				"request", request,
				"limit", limit),
				helm.Comment("Unit [millicore]"))
		}

		diskSizes := helm.NewMapping()
		for _, volume := range role.Run.Volumes {
			switch volume.Type {
			case model.VolumeTypePersistent, model.VolumeTypeShared:
				diskSizes.Add(makeVarName(volume.Tag), volume.Size)
			}
		}
		if len(diskSizes.Names()) > 0 {
			entry.Add("disk_sizes", diskSizes.Sort())
		}
		ports := helm.NewMapping()
		for _, port := range role.Run.ExposedPorts {
			config := helm.NewMapping()
			if port.PortIsConfigurable {
				config.Add("port", port.ExternalPort)
			}
			if port.CountIsConfigurable {
				config.Add("count", port.Count)
			}
			if len(config.Names()) > 0 {
				ports.Add(makeVarName(port.Name), config)
			}
		}
		if len(ports.Names()) > 0 {
			entry.Add("ports", ports.Sort())
		}

		entry.Add("affinity", helm.NewMapping(), helm.Comment("Node affinity rules can be specified here"))

		sizing.Add(makeVarName(role.Name), entry.Sort(), helm.Comment(role.GetLongDescription()))

	}

	registry := settings.Registry
	if registry == "" {
		// Use DockerHub as default registry because our templates will *always* include
		// the registry in image names: $REGISTRY/$ORG/$IMAGE:$TAG, and that doesn't work
		// if registry is blank.
		registry = "docker.io"
	}
	registryInfo := helm.NewMapping()
	registryInfo.Add("hostname", registry)
	registryInfo.Add("username", settings.Username)
	registryInfo.Add("password", settings.Password)

	kube := helm.NewMapping()
	kube.Add("external_ips", helm.NewList())
	kube.Add("secrets_generation_counter", 1, helm.Comment("Increment this counter to rotate all generated secrets"))
	kube.Add("storage_class", helm.NewMapping("persistent", "persistent", "shared", "shared"))
	kube.Add("hostpath_available", false, helm.Comment("Whether HostPath volume mounts are available"))
	kube.Add("registry", registryInfo)
	kube.Add("organization", settings.Organization)

	if settings.AuthType == "" {
		kube.Add("auth", nil)
	} else {
		kube.Add("auth", settings.AuthType)
	}

	values := helm.NewMapping()
	values.Add("env", env.Sort())
	values.Add("sizing", sizing.Sort())
	values.Add("secrets", secrets)
	values.Add("services", helm.NewMapping("loadbalanced", false))
	values.Add("kube", kube)

	return values, nil
}
