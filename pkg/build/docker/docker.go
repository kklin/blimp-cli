package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	docker "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/kelda/blimp/pkg/auth"
	"github.com/kelda/blimp/pkg/build"
	"github.com/kelda/blimp/pkg/errors"
)

type client struct {
	client       *docker.Client
	regCreds     auth.RegistryCredentials
	dockerConfig *configfile.ConfigFile
	blimpToken   string

	composeImageCache map[string]types.ImageSummary
}

type CacheOptions struct {
	Disable     bool
	ProjectName string
}

func New(regCreds auth.RegistryCredentials, dockerConfig *configfile.ConfigFile, blimpToken string, cacheOpts CacheOptions) (build.Interface, error) {
	dockerClient, err := getDockerClient()
	if err != nil {
		return nil, err
	}

	c := client{
		client:       dockerClient,
		regCreds:     regCreds,
		dockerConfig: dockerConfig,
		blimpToken:   blimpToken,
	}

	if !cacheOpts.Disable {
		composeImageCache, err := getComposeImageCache(dockerClient, cacheOpts.ProjectName)
		if err == nil {
			c.composeImageCache = composeImageCache
		} else {
			log.WithError(err).Debug("Failed to get compose image cache")
		}
	}

	return c, nil
}

func (c client) BuildAndPush(serviceName, imageName string, opts build.Options) (digest string, err error) {
	prePushError := make(chan error)
	go func() {
		prePushError <- pushBaseImage(c.client, c.blimpToken, c.regCreds, filepath.Join(opts.Context, opts.Dockerfile), replaceTag(imageName, "base"))
	}()

	// If the image is in the docker cache, then just tag it to be imageName
	// rather than doing a full build.
	cached, ok := c.composeImageCache[serviceName]
	if ok {
		log.WithField("service", serviceName).Info("Using cached image from Docker Compose")
		if err := c.client.ImageTag(context.Background(), cached.ID, imageName); err != nil {
			return "", errors.WithContext("tag", err)
		}
	} else {
		if err := c.build(serviceName, imageName, opts); err != nil {
			return "", errors.WithContext("build", err)
		}
	}

	// Wait for the prepush to complete.
	if err := <-prePushError; err != nil {
		log.WithField("service", serviceName).WithError(err).Debug("Prepush failed. Proceeding with a full image push")
	}

	//imageName = "dev-kevin-blimp-registry.kelda.io/a98c0197112b7a4a96b72ea21ac0802b/web:60a7cc81c039c034c19acff5e793735889c289d9f46030ab9776d6cc6c63b977"
	if digest, err = c.push(imageName); err != nil {
		return "", errors.WithContext("push image", err)
	}

	return digest, nil
}

func (c *client) build(serviceName, imageName string, opts build.Options) error {
	buildContextTar, err := makeTar(opts.Context)
	if err != nil {
		return errors.WithContext("tar context", err)
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
		return errors.WithContext("start build", err)
	}
	defer buildResp.Body.Close()

	// Block until the build completes, and return any errors that happen
	// during the build.
	isTerminal := terminal.IsTerminal(int(os.Stderr.Fd()))
	err = jsonmessage.DisplayJSONMessagesStream(buildResp.Body, os.Stderr, os.Stderr.Fd(), isTerminal, nil)
	if err != nil {
		return errors.NewFriendlyError(
			"Image build for %q failed. This is likely an error with the Dockerfile, rather than Blimp.\n"+
				"Make sure that the image successfully builds with `docker build`.\n\n"+
				"The full error was:\n%s", serviceName, err)
	}
	return nil
}

func (c *client) push(image string) (string, error) {
	cred, ok := c.regCreds.LookupByImage(image)
	if !ok {
		return "", errors.New("no credentials for pushing image")
	}

	registryAuth, err := auth.RegistryAuthHeader(cred)
	if err != nil {
		return "", err
	}

	pushResp, err := c.client.ImagePush(context.Background(), image, types.ImagePushOptions{
		RegistryAuth: registryAuth,
	})
	if err != nil {
		return "", errors.WithContext("start image push", err)
	}
	defer pushResp.Close()

	var imageDigest string
	callback := func(msg jsonmessage.JSONMessage) {
		var digest struct{ Digest string }
		if err := json.Unmarshal(*msg.Aux, &digest); err != nil {
			log.WithError(err).Warn("Failed to parse digest")
			return
		}

		if digest.Digest != "" {
			imageDigest = digest.Digest
		}
	}
	isTerminal := terminal.IsTerminal(int(os.Stderr.Fd()))
	err = jsonmessage.DisplayJSONMessagesStream(pushResp, os.Stderr, os.Stderr.Fd(), isTerminal, callback)
	return imageDigest, err
}

// getDockerClient returns a working Docker client, or nil if we can't connect
// to a Docker client.
func getDockerClient() (*docker.Client, error) {
	dockerClient, err := docker.NewClientWithOpts(docker.FromEnv, docker.WithAPIVersionNegotiation())
	if err != nil {
		return nil, errors.WithContext("create docker client", err)
	}

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	_, err = dockerClient.Ping(ctx)
	if err == nil {
		// We successfully connected to the Docker daemon, so we're pretty
		// confident it's running and works.
		return dockerClient, nil
	}
	if !docker.IsErrConnectionFailed(err) {
		return nil, errors.WithContext("docker ping failed", err)
	}

	// If connection failed, Docker is probably not available at the default
	// address.

	// If a custom host is set, don't try any funny business.
	if dockerClient.DaemonHost() != docker.DefaultDockerHost {
		return nil, errors.WithContext("docker ping failed", err)
	}

	// If we're in WSL, see if we should use the TCP socket instead.
	procVersion, err := ioutil.ReadFile("/proc/version")
	if err != nil || !strings.Contains(string(procVersion), "Microsoft") {
		// Not WSL, so we just give up.
		return nil, errors.WithContext("docker ping failed", err)
	}

	dockerClient, err = docker.NewClientWithOpts(docker.FromEnv, docker.WithAPIVersionNegotiation(),
		docker.WithHost("tcp://localhost:2375"))
	if err != nil {
		return nil, errors.WithContext("create WSL TCP docker client", err)
	}

	ctx, _ = context.WithTimeout(context.Background(), 5*time.Second)
	_, err = dockerClient.Ping(ctx)
	return dockerClient, err
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

func getComposeImageCache(c *docker.Client, project string) (map[string]types.ImageSummary, error) {
	// See https://github.com/docker/compose/blob/854c14a5bcf566792ee8a972325c37590521656b/compose/service.py#L379
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	opts := types.ImageListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key: "reference",
			// This will match images built by Docker Compose.
			Value: fmt.Sprintf("%s_*:latest", project),
		}),
	}
	images, err := c.ImageList(ctx, opts)
	if err != nil {
		return nil, err
	}

	cache := map[string]types.ImageSummary{}
	for _, image := range images {
		for _, tag := range image.RepoTags {
			println(tag)
			if !strings.HasSuffix(tag, ":latest") || !strings.HasPrefix(tag, project+"_") {
				continue
			}
			cache[strings.TrimPrefix(tag, project+"_")] = image
		}
	}
	return cache, nil
}
