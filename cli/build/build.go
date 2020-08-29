package build

import (
	"os"

	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/kelda/blimp/pkg/auth"
	"github.com/kelda/blimp/pkg/build"
	"github.com/kelda/blimp/pkg/build/buildkit"
	"github.com/kelda/blimp/pkg/build/docker"
	"github.com/kelda/blimp/pkg/dockercompose"
)

func New() *cobra.Command {
	var composePaths []string
	var pull bool
	var noCache bool
	cobraCmd := &cobra.Command{
		Use:   "build [OPTIONS] [SERVICE...]",
		Short: "Build or rebuild services.",
		Long: "Services are built once and then cached.\n" +
			"If you change a service's `Dockerfile` or the contents of its build directory, " +
			"you can run `blimp build` to rebuild it.",
		Run: func(_ *cobra.Command, services []string) {
			dockerConfig, err := config.Load(config.Dir())
			if err != nil {
				log.WithError(err).Fatal("Failed to load docker config")
			}

			regCreds, err := auth.GetLocalRegistryCredentials(cmd.dockerConfig)
			if err != nil {
				log.WithError(err).Debug("Failed to get local registry credentials. Private images will fail to pull.")
				regCreds = map[string]types.AuthConfig{}
			}
			// Add the registry credentials for pushing to the blimp registry.
			// TODO: Cleanest thing is probably to add an RPC for getting the image namespace.
			regCreds[strings.SplitN(cmd.imageNamespace, "/", 2)[0]] = types.AuthConfig{
				Username: "ignored",
				Password: cmd.auth.AuthToken,
			}

			// Convert the compose path to an absolute path so that the code
			// that makes identifiers for bind volumes are unique for relative
			// paths.
			composePath, overridePaths, err := dockercompose.GetPaths(composePaths)
			if err != nil {
				if os.IsNotExist(err) {
					log.Fatal("Docker Compose file not found.\n" +
						"Blimp must be run from the same directory as docker-compose.yml.\n" +
						"If you don't have a docker-compose.yml, you can use one of our examples:\n" +
						"https://kelda.io/blimp/docs/examples/")
				}
				log.WithError(err).Fatal("Failed to get absolute path to Compose file")
			}

			parsedCompose, err := dockercompose.Load(composePath, overridePaths, services)
			if err != nil {
				log.WithError(err).Fatal("Failed to load compose file")
			}

			builder, err := getBuilder()
			if err != nil {
				log.WithError(err).Fatal("Get image builder")
			}

			for _, svc := range parsedCompose.Services {
				if svc.Build == nil {
					log.Infof("%s uses an image, skipping\n", svc.Name)
					continue
				}

				log.Infof("Building %s\n", svc.Name)
				opts := build.Options{
					BuildConfig: *svc.Build,
					PullParent:  pull,
					NoCache:     noCache,
				}
				imageName := fmt.Sprintf("%s/%s", cmd.imageNamespace, svc.Name)
				_, err := builder.BuildAndPush(svc.Name, imageName, opts)
				if err != nil {
					log.WithError(err).WithField("service", svc.Name).Warn("Failed to build service")
				}
			}
		},
	}
	cobraCmd.Flags().StringSliceVarP(&composePaths, "file", "f", nil,
		"Specify an alternate compose file\nDefaults to docker-compose.yml and docker-compose.yaml")
	cobraCmd.Flags().BoolVarP(&pull, "pull", "", false,
		"Always attempt to pull a newer version of the image.")
	cobraCmd.Flags().BoolVarP(&noCache, "no-cache", "", false,
		"Do not use cache when building the image")
	return cobraCmd
}

func (cmd *up) getImageBuilder() (build.Interface, error) {
	if !cmd.forceBuildkit {
		dockerClient, err := docker.New(dockerClient, regCreds, dockerConfig, docker.CacheOptions{Disable: true})
		if err == nil {
			return dockerClient, nil
		}
		// TODO: Handle err != nil, return it if both fail.
	}

	// TODO: Create tunnelManager.
	// TODO: Test building without existing namespace.

	buildkitClient, err := buildkit.New(tunnelManager, regCreds)
	if err != nil {
		return nil, errors.WithContext("create buildkit image builder", err)
	}
	return buildkitClient, nil
}
