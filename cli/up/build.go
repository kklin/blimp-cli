package up

import (
	"fmt"
	"path/filepath"
	"strings"

	composeTypes "github.com/kelda/compose-go/types"

	"github.com/kelda/blimp/cli/util"
	"github.com/kelda/blimp/pkg/build"
	"github.com/kelda/blimp/pkg/build/buildkit"
	"github.com/kelda/blimp/pkg/build/docker"
	"github.com/kelda/blimp/pkg/errors"
)

// buildImages builds the images referenced by services in the given Compose
// file. It builds all of the images first, and then tries to push them. When
// pushing, it first checks whether the image already exists remotely, and if
// it does, short circuits the push.
func (cmd *up) buildImages(composeFile composeTypes.Project) (map[string]string, error) {
	var buildServices composeTypes.Services
	for _, svc := range composeFile.Services {
		if svc.Build != nil {
			buildServices = append(buildServices, svc)
		}
	}

	if len(buildServices) == 0 {
		return map[string]string{}, nil
	}

	// TODO: Pull into a package so that it can be reused by `blimp build`.
	var builder build.Interface
	if !cmd.forceBuildkit {
		dockerClient, err := util.GetDockerClient()
		if err == nil {
			builder = docker.New(dockerClient, cmd.regCreds, cmd.dockerConfig, cmd.auth.AuthToken)
		}
	}

	if builder != nil {
		tunnelErr := make(chan error)
		tunnelReady := make(chan struct{})
		go func() {
			tunnelErr <- cmd.tunnelManager.Run("127.0.0.1", 1234, "buildkitd", 1234, tunnelReady)
		}()
		select {
		case err := <-tunnelErr:
			return nil, errors.WithContext("connect to buildkitd", err)
		case <-tunnelReady:
		}

		buildkitClient, err := buildkit.New("tcp://127.0.0.1:1234", strings.SplitN(cmd.imageNamespace, "/", 2)[0], cmd.auth.AuthToken)
		if err != nil {
			return nil, errors.WithContext("create buildkit image builder", err)
		}
		builder = buildkitClient
	}

	builtImages := map[string]string{}
	for _, svc := range buildServices {
		imageName := fmt.Sprintf("%s/%s", cmd.imageNamespace, svc.Name)

		opts := build.Options{
			Context:    filepath.Join(filepath.Dir(cmd.composePath), svc.Build.Context),
			Dockerfile: svc.Build.Dockerfile,
			Args:       svc.Build.Args,
			Target:     svc.Build.Target,
			Labels:     svc.Build.Labels,
			CacheFrom:  svc.Build.CacheFrom,
		}
		if opts.Dockerfile == "" {
			opts.Dockerfile = "Dockerfile"
		}

		dockerfilePath := filepath.Join(opts.Context, opts.Dockerfile)
		stat, err := os.Stat(dockerfilePath)
		if err != nil {
			return "", errors.NewFriendlyError(
				"Can't open Dockerfile for %s, please make sure it exists and can be accessed.\n"+
					"The Dockerfile should be at the path %s.\nThe underlying error was: %v",
				serviceName, dockerfilePath, err)
		}
		if !stat.Mode().IsRegular() {
			return "", errors.NewFriendlyError(
				"The Dockerfile for %s (%s) is not a regular file.",
				serviceName, dockerfilePath)
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
