package build

type Interface interface {
	BuildAndPush(serviceName, imageName string, opts Options) (string, error)
}

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
