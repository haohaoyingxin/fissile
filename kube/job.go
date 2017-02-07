package kube

import (
	"github.com/hpcloud/fissile/model"
	"k8s.io/client-go/pkg/api/v1"
	extra "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

// NewJob creates a new Job for the given role, as well as any objects it depends on
func NewJob(role *model.Role, settings *KubeExportSettings) (*extra.Job, error) {
	podTemplate, err := NewPodTemplate(role, settings)
	if err != nil {
		return nil, err
	}
	// Jobs must have a restart policy that isn't "always"
	podTemplate.Spec.RestartPolicy = v1.RestartPolicyOnFailure
	return &extra.Job{
		ObjectMeta: v1.ObjectMeta{
			Name: role.Name,
		},
		Spec: extra.JobSpec{
			Template: podTemplate,
		},
	}, nil
}
