package buildkit

import (
	"context"
	"fmt"
	"os"

	"github.com/containerd/console"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/progress/progressui"

	"github.com/kelda/blimp/pkg/build"
	"github.com/kelda/blimp/pkg/errors"
)

type Client struct {
	client       *client.Client
	authProvider *AuthProvider
}

func New(buildkitHost, registryHost, token string) (build.Interface, error) {
	c, err := client.New(context.Background(), buildkitHost)
	if err != nil {
		return nil, errors.WithContext("connect to buildkit", err)
	}

	return Client{
		client: c,
		authProvider: &AuthProvider{
			Host:  registryHost,
			Token: token,
		},
	}, nil
}

func (c Client) BuildAndPush(serviceName, imageName string, opts build.Options) (digest string, err error) {
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
			"filename": opts.Dockerfile,
		},
		LocalDirs: map[string]string{
			"context":    opts.Context,
			"dockerfile": ".",
		},
		Exports: []client.ExportEntry{
			exportEntry,
		},
		Session: []session.Attachable{
			c.authProvider,
		},
	}

	ch := make(chan *client.SolveStatus)
	cons, err := console.ConsoleFromFile(os.Stdout)
	if err != nil {
		return "", errors.WithContext("create buildkit console", err)
	}

	go progressui.DisplaySolveStatus(context.TODO(), fmt.Sprintf("Building %s", serviceName), cons, os.Stdout, ch)

	resp, err := c.client.Solve(context.TODO(), nil, solveOpt, ch)
	if err != nil {
		return "", errors.WithContext("buildkit solve", err)
	}

	digest, ok := resp.ExporterResponse["containerimage.digest"]
	if !ok {
		return "", errors.New("didn't receive digest from buildkit")
	}

	return digest, nil
}
