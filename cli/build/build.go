package build

import (
	"fmt"
	"os"
	"strings"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/docker/api/types"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/kelda/blimp/cli/authstore"
	"github.com/kelda/blimp/pkg/auth"
	"github.com/kelda/blimp/pkg/build"
	"github.com/kelda/blimp/pkg/build/buildkit"
	"github.com/kelda/blimp/pkg/build/docker"
	"github.com/kelda/blimp/pkg/dockercompose"
	"github.com/kelda/blimp/pkg/errors"
	"github.com/kelda/blimp/pkg/tunnel"
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
			authConfig, err := authstore.New()
			if err != nil {
				log.WithError(err).Fatal("Failed to parse auth store")
			}
			// TODO: Error if not logged in.

			dockerConfig, err := config.Load(config.Dir())
			if err != nil {
				log.WithError(err).Fatal("Failed to load docker config")
			}

			// TODO: Cleanest thing is probably to add an RPC for getting the image namespace.
			imageNamespace := "dev-kevin-blimp-registry.kelda.io/a98c0197112b7a4a96b72ea21ac0802b"

			regCreds, err := auth.GetLocalRegistryCredentials(dockerConfig)
			if err != nil {
				log.WithError(err).Debug("Failed to get local registry credentials. Private images will fail to pull.")
				regCreds = map[string]types.AuthConfig{}
			}
			// Add the registry credentials for pushing to the blimp registry.
			regCreds[strings.SplitN(imageNamespace, "/", 2)[0]] = types.AuthConfig{
				Username: "ignored",
				Password: authConfig.AuthToken,
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

			// TODO
			builder, err := getImageBuilder(regCreds, dockerConfig, false)
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
				imageName := fmt.Sprintf("%s/%s", imageNamespace, svc.Name)
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

func getImageBuilder(regCreds auth.RegistryCredentials, dockerConfig *configfile.ConfigFile, forceBuildkit bool) (build.Interface, error) {
	if !forceBuildkit {
		dockerClient, err := docker.New(regCreds, dockerConfig, docker.CacheOptions{Disable: true})
		if err == nil {
			return dockerClient, nil
		}
		// TODO: Handle err != nil, return it if both fail.
	}

	// TODO: Create tunnelManager.
	tunnelManager := tunnel.Manager{}
	// TODO: Test building without existing namespace.

	buildkitClient, err := buildkit.New(tunnelManager, regCreds)
	if err != nil {
		return nil, errors.WithContext("create buildkit image builder", err)
	}
	return buildkitClient, nil
}
