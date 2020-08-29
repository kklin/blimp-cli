package up

import (
	"strings"

	composeTypes "github.com/kelda/compose-go/types"

	buildutil "github.com/kelda/blimp/pkg/build/util"
	"github.com/kelda/blimp/pkg/errors"
)

func (cmd *up) buildImages(composeFile composeTypes.Project) (map[string]string, error) {
	var buildServices composeTypes.Services
	for _, svc := range composeFile.Services {
		if svc.Build != nil {
			buildServices = append(buildServices, svc)
		}
	}

	// If there's nothing to build, then shortcircuit so that we don't error
	// out if we can't connect to an image builder.
	if len(buildServices) == 0 {
		return map[string]string{}, nil
	}

	builder, err := buildutil.GetBuilder(cmd.forceBuildkit, cmd.composePath, cmd.regCreds, cmd.dockerConfig, strings.SplitN(cmd.imageNamespace, "/", 2)[0], cmd.auth.AuthToken, cmd.tunnelManager)
	if err != nil {
		return nil, errors.WithContext("get image builder", err)
	}

	return buildutil.BuildAndPush(builder, buildServices, cmd.composePath, cmd.imageNamespace)
}
