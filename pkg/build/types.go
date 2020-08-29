package build

import (
	composeTypes "github.com/kelda/compose-go/types"
)

type Interface interface {
	BuildAndPush(serviceName, imageName string, opts Options) (string, error)
}

// TODO This is kind of duplicated. Should it be the options passed to
// ImageBuild? Or from the composeSpec? Probably the former.
type Options struct {
	composeTypes.BuildConfig
	PullParent bool
	NoCache    bool
}
