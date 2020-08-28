package auth

import (
	"encoding/base64"
	"encoding/json"

	"github.com/docker/cli/cli/config/configfile"
	clitypes "github.com/docker/cli/cli/config/types"
	"github.com/docker/docker/api/types"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/kelda/blimp/pkg/proto/cluster"
)

type RegistryCredentials map[string]types.AuthConfig

// GetLocalRegistryCredentials reads the user's registry credentials from their
// local machine.
func GetLocalRegistryCredentials(dockerConfig *configfile.ConfigFile) (RegistryCredentials, error) {
	// Get the insecure credentials that were saved directly to
	// the auths section of ~/.docker/config.json.
	creds := RegistryCredentials{}
	addCredentials := func(authConfigs map[string]clitypes.AuthConfig) {
		for host, cred := range authConfigs {
			// Don't add empty config sections.
			if cred.Username != "" ||
				cred.Password != "" ||
				cred.Auth != "" ||
				cred.Email != "" ||
				cred.IdentityToken != "" ||
				cred.RegistryToken != "" {
				creds[host] = types.AuthConfig{
					Username:      cred.Username,
					Password:      cred.Password,
					Auth:          cred.Auth,
					Email:         cred.Email,
					ServerAddress: cred.ServerAddress,
					IdentityToken: cred.IdentityToken,
					RegistryToken: cred.RegistryToken,
				}
			}
		}
	}
	addCredentials(dockerConfig.GetAuthConfigs())

	// Get the secure credentials that are set via credHelpers and credsStore.
	// These credentials take preference over any insecure credentials.
	credHelpers, err := dockerConfig.GetAllCredentials()
	if err != nil {
		return nil, err
	}
	addCredentials(credHelpers)

	return creds, nil
}

// TODO: All sorts of special cases?
func (creds RegistryCredentials) LookupByHost(host string) (types.AuthConfig, bool) {
	// TODO: Special handling copied from https://github.com/moby/buildkit/blob/master/session/auth/authprovider/authprovider.go#L38
	if host == "registry-1.docker.io" {
		host = "https://index.docker.io/v1/"
	}

	cred, ok := creds[host]
	if !ok {
		return types.AuthConfig{}, false
	}
	return cred, true
}

func (creds RegistryCredentials) LookupByImage(image string) (types.AuthConfig, bool) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return types.AuthConfig{}, false
	}

	return creds.LookupByHost(ref.Context().Registry.Name())
}

func (creds RegistryCredentials) ToProtobuf() map[string]*cluster.RegistryCredential {
	pb := map[string]*cluster.RegistryCredential{}
	for host, cred := range creds {
		pb[host] = &cluster.RegistryCredential{
			Username: cred.Username,
			Password: cred.Password,
		}
	}
	return pb
}

func RegistryAuthHeader(cred types.AuthConfig) (string, error) {
	authJSON, err := json.Marshal(types.AuthConfig{
		Username: cred.Username,
		Password: cred.Password,
	})
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(authJSON), nil
}
