package builder

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hpcloud/fissile/docker"
	"github.com/hpcloud/fissile/model"
	"github.com/hpcloud/fissile/util"

	"github.com/hpcloud/termui"
	"github.com/stretchr/testify/assert"
)

func TestGenerateRoleImageDockerfile(t *testing.T) {
	assert := assert.New(t)

	ui := termui.New(
		os.Stdin,
		ioutil.Discard,
		nil,
	)

	releaseVersion := "3.14.15"

	workDir, err := os.Getwd()
	assert.Nil(err)

	releasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease-0.3.5")
	compiledPackagesDir := filepath.Join(workDir, "../test-assets/tor-boshrelease-fake-compiled")
	targetPath, err := ioutil.TempDir("", "fissile-test")
	assert.Nil(err)
	defer os.RemoveAll(targetPath)

	release, err := model.NewRelease(releasePath)
	assert.Nil(err)

	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/tor-good.yml")
	rolesManifest, err := model.LoadRoleManifest(roleManifestPath, []*model.Release{release})
	assert.Nil(err)

	roleImageBuilder := NewRoleImageBuilder("foo", compiledPackagesDir, targetPath, "http://127.0.0.1:8500", "hcf", releaseVersion, "6.28.30", ui)

	dockerfileContents, err := roleImageBuilder.generateDockerfile(rolesManifest.Roles[0])
	assert.Nil(err)

	dockerfileString := string(dockerfileContents)
	assert.Contains(dockerfileString, "foo-role-base:6.28.30")
	assert.Contains(dockerfileString, "MAINTAINER", "release images should contain maintainer information")
	assert.Contains(
		dockerfileString,
		fmt.Sprintf(`LABEL "role"="%s" "version"="%s"`, rolesManifest.Roles[0].Name, releaseVersion),
		"Expected role label",
	)

	release.Dev = true
	dockerfileContents, err = roleImageBuilder.generateDockerfile(rolesManifest.Roles[0])
	assert.Nil(err)
	dockerfileString = string(dockerfileContents)
	assert.NotContains(dockerfileString, "MAINTAINER", "dev mode should not generate a maintainer layer")
}

func TestGenerateRoleImageRunScript(t *testing.T) {
	assert := assert.New(t)

	ui := termui.New(
		os.Stdin,
		ioutil.Discard,
		nil,
	)

	workDir, err := os.Getwd()
	assert.Nil(err)

	releasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease-0.3.5")
	compiledPackagesDir := filepath.Join(workDir, "../test-assets/tor-boshrelease-fake-compiled")
	targetPath, err := ioutil.TempDir("", "fissile-test")
	assert.Nil(err)
	defer os.RemoveAll(targetPath)

	release, err := model.NewRelease(releasePath)
	assert.Nil(err)

	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/tor-good.yml")
	rolesManifest, err := model.LoadRoleManifest(roleManifestPath, []*model.Release{release})
	assert.Nil(err)

	roleImageBuilder := NewRoleImageBuilder("foo", compiledPackagesDir, targetPath, "http://127.0.0.1:8500", "hcf", "3.14.15", "6.28.30", ui)

	runScriptContents, err := roleImageBuilder.generateRunScript(rolesManifest.Roles[0])
	assert.Nil(err)
	assert.Contains(string(runScriptContents), "/var/vcap/jobs-src/tor/templates/data/properties.sh.erb")
	assert.Contains(string(runScriptContents), "/opt/hcf/monitrc.erb")
	assert.Contains(string(runScriptContents), "/opt/hcf/startup/myrole.sh")
	assert.Contains(string(runScriptContents), "monit -vI")
	assert.Contains(string(runScriptContents), "\"templates\":[{\"name\":\"new_hostname\"},{\"name\":\"tor\"}]")

	runScriptContents, err = roleImageBuilder.generateRunScript(rolesManifest.Roles[1])
	assert.Nil(err)
	assert.NotContains(string(runScriptContents), "monit -vI")
	assert.NotContains(string(runScriptContents), "/etc/monitrc")
	assert.Contains(string(runScriptContents), "/var/vcap/jobs/tor/bin/run")
}

func TestGenerateRoleImageDockerfileDir(t *testing.T) {
	assert := assert.New(t)

	ui := termui.New(
		os.Stdin,
		ioutil.Discard,
		nil,
	)

	workDir, err := os.Getwd()
	assert.Nil(err)

	releasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease-0.3.5")
	compiledPackagesDir := filepath.Join(workDir, "../test-assets/tor-boshrelease-fake-compiled")
	targetPath, err := ioutil.TempDir("", "fissile-test")
	assert.Nil(err)
	defer os.RemoveAll(targetPath)

	release, err := model.NewRelease(releasePath)
	assert.Nil(err)

	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/tor-good.yml")
	rolesManifest, err := model.LoadRoleManifest(roleManifestPath, []*model.Release{release})
	assert.Nil(err)

	roleImageBuilder := NewRoleImageBuilder("foo", compiledPackagesDir, targetPath, "http://127.0.0.1:8500", "hcf", "3.14.15", "6.28.30", ui)

	dockerfileDir, err := roleImageBuilder.CreateDockerfileDir(rolesManifest.Roles[0])
	assert.Nil(err)
	defer os.RemoveAll(dockerfileDir)

	assert.Equal(filepath.Join(targetPath, "myrole"), dockerfileDir)

	for _, info := range []struct {
		path  string
		isDir bool
		desc  string
	}{
		{path: ".", isDir: true, desc: "role dir"},
		{path: "Dockerfile", isDir: false, desc: "Dockerfile"},
		{path: "root", isDir: true, desc: "image root"},
		{path: "root/opt/hcf/share/doc/tor/LICENSE.md", isDir: false, desc: "release license file"},
		{path: "root/opt/hcf/share/doc/tor/tor/tor/src/LICENSE.txt", isDir: false, desc: "tor role license file"},
		{path: "root/opt/hcf/share/doc/tor/libevent/libevent/LICENSE", isDir: false, desc: "libevent role license file"},
		{path: "root/opt/hcf/run.sh", isDir: false, desc: "run script"},
		{path: "root/opt/hcf/startup/", isDir: true, desc: "role startup scripts dir"},
		{path: "root/opt/hcf/startup/myrole.sh", isDir: false, desc: "role specific startup script"},
		{path: "root/var/vcap/jobs-src/tor/monit", isDir: false, desc: "job monit file"},
		{path: "root/var/vcap/jobs-src/tor/templates/bin/monit_debugger", isDir: false, desc: "job template file"},
		{path: "root/var/vcap/packages/tor", isDir: true, desc: "package dir"},
		{path: "root/var/vcap/packages/tor/bar", isDir: false, desc: "compilation artifact"},
	} {
		path := filepath.ToSlash(filepath.Join(targetPath, "myrole", info.path))
		assert.Nil(util.ValidatePath(path, info.isDir, info.desc))
	}

	// job.MF should not be there
	assert.NotNil(util.ValidatePath(filepath.ToSlash(filepath.Join(dockerfileDir, "root/var/vcap/jobs-src/tor/job.MF")), false, "job manifest file"))
}

type buildImageCallback func(name string) error

type mockDockerImageBuilder struct {
	callback buildImageCallback
}

func (m *mockDockerImageBuilder) BuildImage(dockerDirPath, name string, stdoutProcessor docker.ProcessOutStream) error {
	return m.callback(name)
}

func TestBuildRoleImages(t *testing.T) {

	origNewDockerImageBuilder := newDockerImageBuilder
	defer func() {
		newDockerImageBuilder = origNewDockerImageBuilder
	}()

	type dockerBuilderMock struct {
	}

	mockBuilder := mockDockerImageBuilder{}
	newDockerImageBuilder = func() (dockerImageBuilder, error) {
		return &mockBuilder, nil
	}

	assert := assert.New(t)

	ui := termui.New(
		os.Stdin,
		ioutil.Discard,
		nil,
	)

	workDir, err := os.Getwd()
	assert.Nil(err)

	releasePath := filepath.Join(workDir, "../test-assets/tor-boshrelease-0.3.5")
	compiledPackagesDir := filepath.Join(workDir, "../test-assets/tor-boshrelease-fake-compiled")
	targetPath, err := ioutil.TempDir("", "fissile-test")
	assert.Nil(err)
	defer os.RemoveAll(targetPath)

	release, err := model.NewRelease(releasePath)
	assert.Nil(err)

	roleManifestPath := filepath.Join(workDir, "../test-assets/role-manifests/tor-good.yml")
	rolesManifest, err := model.LoadRoleManifest(roleManifestPath, []*model.Release{release})
	assert.Nil(err)

	roleImageBuilder := NewRoleImageBuilder(
		"test-repository",
		compiledPackagesDir,
		targetPath,
		"http://127.0.0.1:8500",
		"hcf",
		"3.14.15",
		"6.28.30",
		ui,
	)

	// Check that making the first wait for the second job works
	secondJobReady := make(chan struct{})
	mockBuilder.callback = func(name string) error {
		if strings.Contains(name, "-myrole:") {
			<-secondJobReady
			return nil
		}
		if strings.Contains(name, "-foorole:") {
			close(secondJobReady)
			return nil
		}
		t.Errorf("Got unexpected job %s", name)
		return fmt.Errorf("Unknown docker image name %s", name)
	}

	err = roleImageBuilder.BuildRoleImages(
		rolesManifest.Roles,
		"test-repository",
		"3.14.15",
		false,
		2,
	)
	assert.Nil(err)

	err = os.RemoveAll(targetPath)
	assert.Nil(err, "Failed to remove target")

	// Should not allow invalid worker counts
	err = roleImageBuilder.BuildRoleImages(
		rolesManifest.Roles,
		"test-repository",
		"3.14.15",
		false,
		0,
	)
	assert.NotNil(err, "Invalid worker count should result in an error")
	assert.Contains(err.Error(), "count", "Building the image should have failed due to invalid worker count")

	// Check that failing the first job will not run the second job
	hasRunSecondJob := false
	mockBuilder.callback = func(name string) error {
		if strings.Contains(name, "-myrole:") {
			return fmt.Errorf("Deliberate failure")
		}
		if strings.Contains(name, "-foorole:") {
			assert.False(hasRunSecondJob, "Second job should not run if first job failed")
			hasRunSecondJob = true
		}
		t.Errorf("Got unexpected job %s", name)
		return fmt.Errorf("Unknown docker image name %s", name)
	}

	err = roleImageBuilder.BuildRoleImages(
		rolesManifest.Roles,
		"test-repository",
		"3.14.15",
		false,
		1,
	)
	assert.Contains(err.Error(), "Deliberate failure", "Returned error should be from first job failing")
	assert.False(hasRunSecondJob, "Second job should not have run")
}
