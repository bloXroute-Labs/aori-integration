package utils

import (
	"context"
	"crypto/tls"
	gateway "github.com/FastLane-Labs/atlas-examples/protobuf"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"log"
)

var Gateway_Endpoint = "localhost:5005"

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
	var ep string
	if endpoint == "" {
		ep = "localhost:5005"
	}
	env, err := godotenv.Read(".env")
	if err != nil {
		log.Fatalf("could not load .env file: %s", err)
	}

	return RPCOpts{
		Endpoint:   ep,
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

// NewGRPCClient connects to custom Trader API
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
