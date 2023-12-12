package utils

import (
	"context"
	"crypto/tls"
	gateway "github.com/bloXroute-Labs/aori-integration/protobuf"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"log"
)

type blxrCredentials struct {
	authorization string
}

type RPCOpts struct {
	Endpoint    string
	DisableAuth bool
	UseTLS      bool
	AuthHeader  string
}

func DefaultRPCOpts(endpoint string) RPCOpts {
	if endpoint == "" {
		// default port is 1809 or 5005, when running multiple gateways, typically use 500* as the second gateway and specify it as the endpoint string here
		endpoint = "localhost:5005"
	}
	env, err := godotenv.Read(".env")
	if err != nil {
		log.Fatalf("could not load .env file: %s", err)
	}

	return RPCOpts{
		Endpoint:   endpoint,
		AuthHeader: env["AUTH_HEADER"],
	}
}

func (bc blxrCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": bc.authorization,
	}, nil
}

func (bc blxrCredentials) RequireTransportSecurity() bool {
	return false
}

// NewGRPCClient connects to custom Intent Gateway
func NewGRPCClient(opts RPCOpts, dialOpts ...grpc.DialOption) (gateway.GatewayClient, error) {
	var (
		conn     grpc.ClientConnInterface
		err      error
		grpcOpts = make([]grpc.DialOption, 0)
	)

	transportOption := grpc.WithTransportCredentials(insecure.NewCredentials())
	if opts.UseTLS {
		transportOption = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{}))
	}

	grpcOpts = append(grpcOpts, transportOption)
	creddies := blxrCredentials{authorization: opts.AuthHeader}

	creds := grpc.WithPerRPCCredentials(creddies)

	if !opts.DisableAuth {
		grpcOpts = append(grpcOpts, creds)
	}

	conn, err = grpc.Dial(opts.Endpoint, grpcOpts...)
	if err != nil {
		return nil, err
	}

	grpcOpts = append(grpcOpts, dialOpts...)
	conn, err = grpc.Dial(opts.Endpoint, grpcOpts...)
	if err != nil {
		return nil, err
	}

	client := gateway.NewGatewayClient(conn)

	return client, nil

}
