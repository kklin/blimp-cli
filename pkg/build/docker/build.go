package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/kelda/blimp/pkg/auth"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/kelda/blimp/pkg/build"
	"github.com/kelda/blimp/pkg/errors"
)

type client struct {
	client   *docker.Client
	regCreds map[string]types.AuthConfig
	// TODO not pointer?
	dockerConfig *configfile.ConfigFile
	// TODO: Rename to blimp token or something.
	token string

	composeImageCache        []types.ImageSummary
	disableComposeImageCache bool
}

func New(dockerClient *docker.Client, regCreds map[string]types.AuthConfig, dockerConfig *configfile.ConfigFile, token, absComposePath string) build.Interface {
	c := client{
		client:       dockerClient,
		regCreds:     regCreds,
		dockerConfig: dockerConfig,
		token:        token,
	}

	composeImageCache, err := getComposeImageCache(dockerClient, absComposePath)
	if err == nil {
		c.composeImageCache = composeImageCache
	} else {
		log.WithError(err).Debug("Failed to get compose image cache")
	}

	return c
}

func (c client) BuildAndPush(serviceName, imageName string, opts build.Options) (digest string, err error) {
	// TODO: Do the prepush here.
	// TODO: Don't build if the docker compose cache already exists.
	// If the image is in the docker cache, then don't build, and just retag it to imageName.
	// Still push as normal.

	buildContextTar, err := makeTar(opts.Context)
	if err != nil {
		return "", errors.WithContext("tar context", err)
	}

	buildResp, err := c.client.ImageBuild(context.TODO(), buildContextTar, types.ImageBuildOptions{
		Tags:        []string{imageName},
		Dockerfile:  opts.Dockerfile,
		AuthConfigs: c.regCreds,
		BuildArgs:   c.dockerConfig.ParseProxyConfig(c.client.DaemonHost(), opts.Args),
		Target:      opts.Target,
		Labels:      opts.Labels,
		CacheFrom:   opts.CacheFrom,
		PullParent:  opts.PullParent,
		NoCache:     opts.NoCache,
	})
	if err != nil {
		return "", errors.WithContext("start build", err)
	}
	defer buildResp.Body.Close()

	// Block until the build completes, and return any errors that happen
	// during the build.
	var imageID string
	callback := func(msg jsonmessage.JSONMessage) {
		var id struct{ ID string }
		if err := json.Unmarshal(*msg.Aux, &id); err != nil {
			log.WithError(err).Warn("Failed to parse build ID")
			return
		}

		if id.ID != "" {
			imageID = id.ID
		}
	}

	isTerminal := terminal.IsTerminal(int(os.Stderr.Fd()))
	err = jsonmessage.DisplayJSONMessagesStream(buildResp.Body, os.Stderr, os.Stderr.Fd(), isTerminal, callback)
	if err != nil {
		return "", errors.NewFriendlyError(
			"Image build for %q failed. This is likely an error with the Dockerfile, rather than Blimp.\n"+
				"Make sure that the image successfully builds with `docker build`.\n\n"+
				"The full error was:\n%s", serviceName, err)
	}

	if err := c.push(imageName); err != nil {
		return "", errors.WithContext("push image", err)
	}

	// TODO: Return digest.
	return imageID, nil
}

func (c *client) push(image string) error {
	registryAuth, err := auth.RegistryAuthHeader(c.token)
	if err != nil {
		return errors.WithContext("make registry auth header", err)
	}

	pushResp, err := c.client.ImagePush(context.Background(), image, types.ImagePushOptions{
		RegistryAuth: registryAuth,
	})
	if err != nil {
		return errors.WithContext("start image push", err)
	}
	defer pushResp.Close()

	isTerminal := terminal.IsTerminal(int(os.Stderr.Fd()))
	return jsonmessage.DisplayJSONMessagesStream(pushResp, os.Stderr, os.Stderr.Fd(), isTerminal, nil)
}

func getHeader(fi os.FileInfo, path string) (*tar.Header, error) {
	var link string
	if fi.Mode()&os.ModeSymlink != 0 {
		var err error
		link, err = os.Readlink(path)
		if err != nil {
			return nil, err
		}
	}

	return tar.FileInfoHeader(fi, link)
}

func makeTar(dir string) (io.Reader, error) {
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	defer tw.Close()

	err := filepath.Walk(dir, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := getHeader(fi, path)
		if err != nil {
			return errors.WithContext("get header", err)
		}

		// Set the file's path within the archive to be relative to the build
		// context.
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return errors.WithContext(fmt.Sprintf("get normalized path %q", path), err)
		}
		// On Windows, relPath will use backslashes. ToSlash normalizes to use
		// forward slashes.
		header.Name = filepath.ToSlash(relPath)

		if err := tw.WriteHeader(header); err != nil {
			return errors.WithContext(fmt.Sprintf("write header %q", header.Name), err)
		}

		fileMode := fi.Mode()
		if !fileMode.IsRegular() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return errors.WithContext(fmt.Sprintf("open file %q", header.Name), err)
		}
		defer f.Close()

		if _, err := io.Copy(tw, f); err != nil {
			return errors.WithContext(fmt.Sprintf("write file %q", header.Name), err)
		}
		return nil
	})
	return &out, err
}

func getComposeImageCache(c *docker.Client, absComposePath string) ([]types.ImageSummary, error) {
	// See https://github.com/docker/compose/blob/854c14a5bcf566792ee8a972325c37590521656b/compose/service.py#L379
	// and https://github.com/docker/compose/blob/854c14a5bcf566792ee8a972325c37590521656b/compose/cli/command.py#L176.
	project := filepath.Base(filepath.Dir(absComposePath))
	badChar := regexp.MustCompile(`[^-_a-z0-9]`)
	composeImagePrefix := badChar.ReplaceAllString(strings.ToLower(project), "")

	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	opts := types.ImageListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key: "reference",
			// This will match images built by Docker Compose.
			Value: fmt.Sprintf("%s_*:latest", composeImagePrefix),
		}),
	}
	return c.ImageList(ctx, opts)
}
