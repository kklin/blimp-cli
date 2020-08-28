package buildkit

import (
	"context"
	"fmt"
	"os"

	"github.com/containerd/console"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/progress/progressui"

	"github.com/kelda/blimp/pkg/errors"
)

func Build(buildkitClient *client.Client, name, imageName, contextDir, dockerfile string,
	authProvider *AuthProvider) (digest string, err error) {

	exportEntry := client.ExportEntry{
		Type:  client.ExporterImage,
		Attrs: map[string]string{},
	}
	exportEntry.Attrs["name"] = imageName
	exportEntry.Attrs["push"] = "true"
	exportEntry.Attrs["name-canonical"] = "true"

	solveOpt := client.SolveOpt{
		Frontend: "dockerfile.v0",
		FrontendAttrs: map[string]string{
			"filename": dockerfile,
		},
		LocalDirs: map[string]string{
			"context":    contextDir,
			"dockerfile": ".",
		},
		Exports: []client.ExportEntry{
			exportEntry,
		},
		Session: []session.Attachable{
			authProvider,
		},
	}

	ch := make(chan *client.SolveStatus)
	c, err := console.ConsoleFromFile(os.Stdout)
	if err != nil {
		return "", errors.WithContext("create buildkit console", err)
	}

	go progressui.DisplaySolveStatus(context.TODO(), fmt.Sprintf("Building %s", name), c, os.Stdout, ch)

	resp, err := buildkitClient.Solve(context.TODO(), nil, solveOpt, ch)
	if err != nil {
		return "", errors.WithContext("buildkit solve", err)
	}

	digest, ok := resp.ExporterResponse["containerimage.digest"]
	if !ok {
		return "", errors.New("didn't receive digest from buildkit")
	}

	return digest, nil
}
