package kube

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SUSE/fissile/helm"
	"github.com/SUSE/fissile/model"
	"github.com/SUSE/fissile/testhelpers"

	"github.com/stretchr/testify/assert"
)

func deploymentTestLoadRole(assert *assert.Assertions, roleName, manifestName string) *model.Role {
	workDir, err := os.Getwd()
	assert.NoError(err)

	manifestPath := filepath.Join(workDir, "../test-assets/role-manifests", manifestName)
	releasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease")
	releasePathBoshCache := filepath.Join(releasePath, "bosh-cache")

	release, err := model.NewDevRelease(releasePath, "", "", releasePathBoshCache)
	if !assert.NoError(err) {
		return nil
	}

	manifest, err := model.LoadRoleManifest(manifestPath, []*model.Release{release}, nil)
	if !assert.NoError(err) {
		return nil
	}

	role := manifest.LookupRole(roleName)
	if !assert.NotNil(role, "Failed to find role %s", roleName) {
		return nil
	}
	return role
}

type FakeGrapher struct {
}

func (f FakeGrapher) GraphNode(nodeName string, attrs map[string]string) error {
	return nil
}

func (f FakeGrapher) GraphEdge(fromNode, toNode string, attrs map[string]string) error {
	return nil
}

func TestNewDeploymentKube(t *testing.T) {
	assert := assert.New(t)

	role := deploymentTestLoadRole(assert, "role", "pod-with-valid-pod-anti-affinity.yml")
	if role == nil {
		return
	}

	settings := ExportSettings{}

	grapher := FakeGrapher{}

	deployment, svc, err := NewDeployment(role, settings, grapher)

	assert.NoError(err)
	assert.Nil(svc)
	assert.NotNil(deployment)
	assert.Equal(deployment.Get("kind").String(), "Deployment")
	assert.Equal(deployment.Get("metadata", "name").String(), "role")
}

func TestNewDeploymentHelm(t *testing.T) {
	assert := assert.New(t)

	role := deploymentTestLoadRole(assert, "role", "pod-with-valid-pod-anti-affinity.yml")
	if role == nil {
		return
	}

	settings := ExportSettings{
		CreateHelmChart: true,
		Repository:      "the_repos",
	}

	grapher := FakeGrapher{}

	deployment, svc, err := NewDeployment(role, settings, grapher)

	assert.NoError(err)
	assert.Nil(svc)
	assert.NotNil(deployment)
	assert.Equal(deployment.Get("kind").String(), "Deployment")
	assert.Equal(deployment.Get("metadata", "name").String(), "role")

	t.Run("Defaults", func(t *testing.T) {
		// Rendering fails with defaults, template needs information
		// about sizing and the like.
		_, err = testhelpers.RenderNode(deployment, nil)
		assert.EqualError(err,
			`template: :9:17: executing "" at <fail "role must have...>: error calling fail: role must have at least 1 instances`)
	})

	t.Run("Configured", func(t *testing.T) {
		config := map[string]interface{}{
			"Values.sizing.role.count":                 "1",
			"Values.sizing.role.affinity.nodeAffinity": "snafu",
			"Values.kube.registry.hostname":            "docker.suse.fake",
			"Values.kube.organization":                 "splat",
			"Values.env.KUBE_SERVICE_DOMAIN_SUFFIX":    "domestic",
		}

		actual, err := testhelpers.RoundtripNode(deployment, config)
		if !assert.NoError(err) {
			return
		}
		testhelpers.IsYAMLEqualString(assert, `---
			apiVersion: "extensions/v1beta1"
			kind: "Deployment"
			metadata:
				name: "role"
				labels:
					skiff-role-name: "role"
			spec:
				replicas: 1
				selector:
					matchLabels:
						skiff-role-name: "role"
				template:
					metadata:
						name: "role"
						labels:
							skiff-role-name: "role"
						annotations:
							checksum/config: 08c80ed11902eefef09739d41c91408238bb8b5e7be7cc1e5db933b7c8de65c3
					spec:
						affinity:
							podAntiAffinity:
								preferredDuringSchedulingIgnoredDuringExecution:
								-	podAffinityTerm:
										labelSelector:
											matchExpressions:
											-	key: "skiff-role-name"
												operator: "In"
												values:
												-	"role"
										topologyKey: "beta.kubernetes.io/os"
									weight: 100
							nodeAffinity: "snafu"
						containers:
						-	env:
							-	name: "KUBERNETES_NAMESPACE"
								valueFrom:
									fieldRef:
										fieldPath: "metadata.namespace"
							-	name: "KUBE_SERVICE_DOMAIN_SUFFIX"
								value: "domestic"
							image: "docker.suse.fake/splat/the_repos-role:bfff10016c4e9e46c9541d35e6bf52054c54e96a"
							lifecycle:
								preStop:
									exec:
										command:
										-	"/opt/fissile/pre-stop.sh"
							livenessProbe: ~
							name: "role"
							ports: ~
							readinessProbe: ~
							resources: ~
							securityContext: ~
							volumeMounts: ~
						dnsPolicy: "ClusterFirst"
						imagePullSecrets:
						- name: "registry-credentials"
						restartPolicy: "Always"
						terminationGracePeriodSeconds: 600
						volumes: ~
		`, actual)
	})
}

func TestGetAffinityBlock(t *testing.T) {
	assert := assert.New(t)

	role := deploymentTestLoadRole(assert, "role", "pod-with-valid-pod-anti-affinity.yml")
	if role == nil {
		return
	}

	affinity := getAffinityBlock(role)

	assert.NotNil(affinity.Get("podAntiAffinity"))
	assert.NotNil(affinity.Get("nodeAffinity"))
	assert.Equal(affinity.Names(), []string{"podAntiAffinity", "nodeAffinity"})
	assert.Equal(affinity.Get("nodeAffinity").Block(), "if .Values.sizing.role.affinity.nodeAffinity")

	role = deploymentTestLoadRole(assert, "role", "pod-with-no-pod-anti-affinity.yml")
	if role == nil {
		return
	}

	affinity = getAffinityBlock(role)

	assert.Nil(affinity.Get("podAntiAffinity"))
	assert.NotNil(affinity.Get("nodeAffinity"))
	assert.Equal(affinity.Names(), []string{"nodeAffinity"})
	assert.Equal(affinity.Get("nodeAffinity").Block(), "if .Values.sizing.role.affinity.nodeAffinity")
}

func createEmptySpec() *helm.Mapping {
	emptySpec := helm.NewMapping()
	template := helm.NewMapping()
	metadata := helm.NewMapping()
	annotations := helm.NewMapping()
	podspec := helm.NewMapping()

	metadata.Add("annotations", annotations)
	template.Add("metadata", metadata)
	template.Add("spec", podspec)
	emptySpec.Add("template", template)

	return emptySpec
}

func TestAddAffinityRules(t *testing.T) {
	assert := assert.New(t)

	emptySpec := createEmptySpec()

	//
	// Test role with valid anti affinity
	//
	role := deploymentTestLoadRole(assert, "role", "pod-with-valid-pod-anti-affinity.yml")
	if role == nil {
		return
	}

	spec := createEmptySpec()

	settings := ExportSettings{CreateHelmChart: true}

	err := addAffinityRules(role, spec, settings)

	assert.NotNil(spec.Get("template", "spec", "affinity", "podAntiAffinity"))
	assert.NotNil(spec.Get("template", "spec", "affinity", "nodeAffinity"))
	assert.NoError(err)

	//
	// Test role with pod affinity defined
	//
	role = deploymentTestLoadRole(assert, "role", "pod-with-invalid-pod-affinity.yml")
	if role == nil {
		return
	}

	spec = createEmptySpec()

	err = addAffinityRules(role, spec, settings)

	assert.Error(err)
	assert.Equal(spec, emptySpec)

	//
	// Test role with node affinity defined
	//
	role = deploymentTestLoadRole(assert, "role", "pod-with-invalid-node-affinity.yml")
	if role == nil {
		return
	}

	spec = createEmptySpec()

	err = addAffinityRules(role, spec, settings)

	assert.Error(err)
	assert.Equal(spec, emptySpec)

	//
	// Not creating the helm chart should only add the annotation
	//
	role = deploymentTestLoadRole(assert, "role", "pod-with-valid-pod-anti-affinity.yml")
	if role == nil {
		return
	}

	spec = createEmptySpec()

	settings = ExportSettings{CreateHelmChart: false}

	err = addAffinityRules(role, spec, settings)
	assert.Nil(spec.Get("template", "spec", "affinity", "podAntiAffinity"))
	assert.NoError(err)
}
