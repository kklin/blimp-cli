package up

import (
	"fmt"
	"strings"

	composeTypes "github.com/kelda/compose-go/types"
	log "github.com/sirupsen/logrus"

	"github.com/kelda/blimp/cli/util"
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

	builder, err := cmd.getImageBuilder()
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

		// TODO: Maybe this should return a fully qualified image name?
		builtImages[svc.Name] = fmt.Sprintf("%s@%s", imageName, digest)
	}

	return builtImages, nil
}

func (cmd *up) getImageBuilder() (build.Interface, error) {
	if !cmd.forceBuildkit {
		// TODO: Any other callers to GetDockerClient?
		dockerClient, err := util.GetDockerClient()
		if err == nil {
			// TODO: docker.New could parse dockerConfig and regCreds itself.
			return docker.New(dockerClient, cmd.regCreds, cmd.dockerConfig, cmd.auth.AuthToken, cmd.composePath), nil
		}
		// TODO: Handle err != nil, return it if both fail.
	}

	buildkitClient, err := buildkit.New(cmd.tunnelManager, strings.SplitN(cmd.imageNamespace, "/", 2)[0], cmd.auth.AuthToken)
	if err != nil {
		return nil, errors.WithContext("create buildkit image builder", err)
	}
	return buildkitClient, nil
}
