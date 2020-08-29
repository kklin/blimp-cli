package build

import (
	"path/filepath"

	composeTypes "github.com/kelda/compose-go/types"
)

type Interface interface {
	BuildAndPush(serviceName, imageName string, opts Options) (string, error)
}

// TODO This is kind of duplicated. Should it be the options passed to
// ImageBuild? Or from the composeSpec? Probably the former.
type Options struct {
	Context    string
	Dockerfile string
	Args       map[string]*string
	Target     string
	Labels     map[string]string
	CacheFrom  []string
	PullParent bool
	NoCache    bool
}

func NewOptions(composePath string, composeSpec composeTypes.BuildConfig) Options {
	opts := Options{
		Context:    filepath.Join(filepath.Dir(composePath), composeSpec.Context),
		Dockerfile: composeSpec.Dockerfile,
		Args:       composeSpec.Args,
		Target:     composeSpec.Target,
		Labels:     composeSpec.Labels,
		CacheFrom:  composeSpec.CacheFrom,
	}
	if opts.Dockerfile == "" {
		opts.Dockerfile = "Dockerfile"
	}
	return opts
}
