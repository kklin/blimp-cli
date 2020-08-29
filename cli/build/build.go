package build

import (
	"os"

	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/api/types"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/kelda/blimp/cli/util"
	"github.com/kelda/blimp/pkg/docker"
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
			dockerClient, err := util.GetDockerClient()
			if err != nil {
				log.WithError(err).Fatal("Failed to connect to local Docker daemon, " +
					"which is used for building images. Aborting")
			}

			dockerConfig, err := config.Load(config.Dir())
			if err != nil {
				log.WithError(err).Fatal("Failed to load docker config")
			}

			regCreds, err := docker.GetLocalRegistryCredentials(dockerConfig)
			if err != nil {
				log.WithError(err).Warn("Failed to get local registry credentials. Private images will fail to pull.")
				regCreds = map[string]types.AuthConfig{}
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

				// TODO: Need to store imageNamespace. Or just always make the
				// call to the manager createsandbox.
				imageName := fmt.Sprintf("%s/%s", cmd.imageNamespace, svc.Name)
				log.Infof("Building %s\n", svc.Name)
				opts := build.Options{
					BuildConfig: *svc.Build,
					PullParent:  pull,
					NoCache:     noCache,
				}
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
		// TODO: Any other callers to GetDockerClient?
		dockerClient, err := util.GetDockerClient()
		if err == nil {
			// TODO: docker.New could parse dockerConfig and regCreds itself.
			return docker.New(dockerClient, cmd.regCreds, cmd.dockerConfig, cmd.auth.AuthToken, cmd.composePath), nil
		}
		// TODO: Handle err != nil, return it if both fail.
	}

	// TODO: Create tunnelManager.
	// TODO: Test booting without existing namespace.

	buildkitClient, err := buildkit.New(cmd.tunnelManager, strings.SplitN(cmd.imageNamespace, "/", 2)[0], cmd.auth.AuthToken)
	if err != nil {
		return nil, errors.WithContext("create buildkit image builder", err)
	}
	return buildkitClient, nil
}
