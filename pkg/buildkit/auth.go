package buildkit

import (
	"context"

	"github.com/moby/buildkit/session/auth"
	"google.golang.org/grpc"
)

type AuthProvider struct {
	Host  string
	Token string
}

func (ap *AuthProvider) Register(server *grpc.Server) {
	auth.RegisterAuthServer(server, ap)
}

func (ap *AuthProvider) Credentials(_ context.Context, req *auth.CredentialsRequest) (*auth.CredentialsResponse, error) {
	if (req.Host != ap.Host) {
		return &auth.CredentialsResponse{}, nil
	}
	return &auth.CredentialsResponse{
		Username: "ignored",
		Secret:   ap.Token,
	}, nil
}
