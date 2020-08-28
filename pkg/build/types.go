package build

import (
	composeTypes "github.com/kelda/compose-go/types"
)

type Interface interface {
	BuildAndPush(serviceName, imageName string, opts Options) (string, error)
}

type Options struct {
	composeTypes.BuildConfig
	PullParent bool
	NoCache    bool
}
