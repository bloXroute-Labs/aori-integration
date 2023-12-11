package main

import (
	"github.com/bloXroute-Labs/aori-integration/cmd"
	"github.com/bloXroute-Labs/aori-integration/entities"
	"os"
	"os/signal"
)

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	app := cmd.Setup()

	shutdownChan := make(chan struct{})

	makeOrderChan := make(chan []byte)
	// Launch the backend goroutine
	aoriBackend := entities.NewAoriBackend(app.PrivateKeys["bundler"], app.ChainId, makeOrderChan, shutdownChan)

	aoriBackend.GenerateMakeOrder(makeOrderChan)

	// CTRL + C to stop the program
	<-interrupt

	close(shutdownChan)
	close(makeOrderChan)

}
