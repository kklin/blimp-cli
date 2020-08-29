package up

import (
	"fmt"

	composeTypes "github.com/kelda/compose-go/types"
	log "github.com/sirupsen/logrus"

	"github.com/kelda/blimp/pkg/build"
	"github.com/kelda/blimp/pkg/build/buildkit"
	"github.com/kelda/blimp/pkg/build/docker"
	"github.com/kelda/blimp/pkg/errors"
)

func (cmd *up) buildImages(composeFile composeTypes.Project) (map[string]string, error) {
	var buildServices composeTypes.Services
	for _, svc := range composeFile.Services {
		if svc.Build != nil {
			buildServices = append(buildServices, svc)
		}
	}

	builder, err := cmd.getImageBuilder(composeFile.Name)
	if err != nil {
		return nil, errors.WithContext("get image builder", err)
	}

	builtImages := map[string]string{}
	for _, svc := range buildServices {
		log.Infof("Building %s\n", svc.Name)
		imageName := fmt.Sprintf("%s/%s", cmd.imageNamespace, svc.Name)
		digest, err := builder.BuildAndPush(svc.Name, imageName, build.Options{BuildConfig: *svc.Build})
		if err != nil {
			return nil, errors.WithContext(fmt.Sprintf("build %s", svc.Name), err)
		}

		builtImages[svc.Name] = fmt.Sprintf("%s@%s", imageName, digest)
	}

	return builtImages, nil
}

func (cmd *up) getImageBuilder(projectName string) (build.Interface, error) {
	if !cmd.forceBuildkit {
		dockerClient, err := docker.New(cmd.regCreds, cmd.dockerConfig, docker.CacheOptions{ProjectName: projectName})
		if err == nil {
			return dockerClient, nil
		}
		// TODO: Handle err != nil, return it if both fail.
	}

	buildkitClient, err := buildkit.New(cmd.tunnelManager, cmd.regCreds)
	if err != nil {
		return nil, errors.WithContext("create buildkit image builder", err)
	}
	return buildkitClient, nil
}
