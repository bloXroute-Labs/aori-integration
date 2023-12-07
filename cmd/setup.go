package cmd

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/joho/godotenv"
	"log"
)

type Config struct {
	ChainId               int64          `json:"chainId"`
	DAppControllerAddress common.Address `json:"dAppController"`
	SolverContractAddress common.Address `json:"solverContract"`
}

type App struct {
	ChainId int64

	PrivateKeys map[string]string
	Addresses   map[string]common.Address
}

func Setup() *App {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	app := &App{
		PrivateKeys: make(map[string]string),
		Addresses:   make(map[string]common.Address),
	}

	app.ChainId = 5

	// Load .env file
	env, err := godotenv.Read(".env")
	if err != nil {
		log.Fatalf("could not load .env file: %s", err)
	}

	// Private keys
	app.PrivateKeys["bundler"] = env["BUNDLER_PK"]
	app.PrivateKeys["governance"] = env["GOVERNANCE_PK"]
	app.PrivateKeys["user"] = env["USER_PK"]
	app.PrivateKeys["solver"] = env["SOLVER_PK"]

	return app
}
