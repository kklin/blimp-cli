package util

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/docker/api/types"
	composeTypes "github.com/kelda/compose-go/types"

	"github.com/kelda/blimp/cli/util"
	"github.com/kelda/blimp/pkg/build"
	"github.com/kelda/blimp/pkg/build/buildkit"
	"github.com/kelda/blimp/pkg/build/docker"
	"github.com/kelda/blimp/pkg/errors"
	"github.com/kelda/blimp/pkg/tunnel"
)

func GetBuilder(forceBuildkit bool, composePath string, regCreds map[string]types.AuthConfig, dockerConfig *configfile.ConfigFile, blimpRegistry, token string, tunnelManager tunnel.Manager) (build.Interface, error) {
	if forceBuildkit {
		// TODO: Any other callers to GetDockerClient?
		dockerClient, err := util.GetDockerClient()
		if err == nil {
			// TODO: docker.New could parse dockerConfig and regCreds itself.
			return docker.New(dockerClient, regCreds, dockerConfig, token, composePath), nil
		}
		// TODO: Handle err != nil, return it if both fail.
	}

	tunnelErr := make(chan error)
	tunnelReady := make(chan struct{})
	go func() {
		tunnelErr <- tunnelManager.Run("127.0.0.1", 1234, "buildkitd", 1234, tunnelReady)
	}()
	select {
	case err := <-tunnelErr:
		return nil, errors.WithContext("connect to buildkitd", err)
	case <-tunnelReady:
	}

	buildkitClient, err := buildkit.New("tcp://127.0.0.1:1234", blimpRegistry, token)
	if err != nil {
		return nil, errors.WithContext("create buildkit image builder", err)
	}
	return buildkitClient, nil
}

func BuildAndPush(builder build.Interface, services composeTypes.Services, absComposePath, imageNamespace string) (map[string]string, error) {
	builtImages := map[string]string{}
	for _, svc := range services {
		// TODO: Maybe caller should build one image at a time, and be
		// responsible for creating the map themselves.
		// Caller could also pass in a `docker.CacheChecker` rather than `absComposePath`.
		log.Infof("Building %s\n", svc.Name)
		imageName := fmt.Sprintf("%s/%s", imageNamespace, svc.Name)

		opts := build.NewOptions(absComposePath, *svc.Build)
		dockerfilePath := filepath.Join(opts.Context, opts.Dockerfile)
		stat, err := os.Stat(dockerfilePath)
		if err != nil {
			return nil, errors.NewFriendlyError(
				"Can't open Dockerfile for %s, please make sure it exists and can be accessed.\n"+
					"The Dockerfile should be at the path %s.\nThe underlying error was: %v",
				svc.Name, dockerfilePath, err)
		}
		if !stat.Mode().IsRegular() {
			return nil, errors.NewFriendlyError(
				"The Dockerfile for %s (%s) is not a regular file.",
				svc.Name, dockerfilePath)
		}

		digest, err := builder.BuildAndPush(svc.Name, imageName, opts)
		if err != nil {
			return nil, errors.WithContext(fmt.Sprintf("build %s", svc.Name), err)
		}

		// TODO: Maybe this should return a fully qualified image name?
		builtImages[svc.Name] = fmt.Sprintf("%s@%s", imageName, digest)
	}

	return builtImages, nil
}
