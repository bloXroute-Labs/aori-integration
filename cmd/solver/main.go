package main

import (
	"encoding/json"
	"fmt"
	"github.com/bloXroute-Labs/aori-integration/entities"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	env, err := godotenv.Read(".env")
	if err != nil {
		log.Fatalf("could not load .env file: %s", err)
	}
	takeOrderRequestChan := make(chan entities.TakeOrderRequest, 10)

	bundlerPrivateKey, err := crypto.ToECDSA(common.Hex2Bytes(env["BUNDLER_PK"]))
	if err != nil {
		log.Fatalf("could not load user's private key: %s", err)
	}

	dappAddress := crypto.PubkeyToAddress(bundlerPrivateKey.PublicKey)

	entities.NewAoriSolver(env["SOLVER_PK"], 5, dappAddress.String(), "AORI SOLVER", takeOrderRequestChan)
	// CTRL + C to stop the program
	<-interrupt
}

var authHeader = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0eXBlIjoiQXBpS2V5IiwiZW1haWwiOiJkYW5pZWwudGFvQGJsb3hyb3V0ZS5jb20iLCJuYW1lIjoiRGFuaWVsIFRhbyIsInJhdGVMaW1pdCI6MTAwLCJpYXQiOjE3MDAwNzcyOTd9.O6STgok-EJ6IcCoQSpTZPp4sGJGe4ShZhz1rJLQ3fOI"

func connect(endpoint string, auth string) (*websocket.Conn, error) {
	dialer := websocket.Dialer{HandshakeTimeout: time.Duration(3) * time.Second}
	header := http.Header{}
	header.Set("Authorization", auth)

	conn, _, err := dialer.Dial(endpoint, header)
	if err != nil {
		return nil, err
	}

	return conn, err
}

type WS struct {
	conn *websocket.Conn
}

func NewWS(endpoint string, authHeader string) (*WS, error) {
	conn, err := connect(endpoint, authHeader)
	if err != nil {
		return nil, err
	}
	return &WS{conn: conn}, nil
}

func startSendingTakeOrderDirectly(broadcastTakeOrdersChan chan entities.TakeOrderRequest) {
	w, err := NewWS("wss://dev.api.beta.order.aori.io/", authHeader)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		for {
			_, msg, err := w.conn.ReadMessage()
			if err != nil {
				// reconnect the websocket connection if connection read message fails
				log.Println("error when reading makeOrder", err)
				continue
			}
			fmt.Println("message from aori", string(msg))
		}
	}()

	for {
		select {
		case msg := <-broadcastTakeOrdersChan:
			b, _ := json.Marshal(msg)
			fmt.Println("sending this payload to aori", string(b))
			err := w.conn.WriteMessage(websocket.TextMessage, b)
			//err := w.conn.WriteMessage(websocket.BinaryMessage, b)
			//err := w.conn.WriteJSON(msg)
			if err != nil {
				// reconnect the websocket connection if connection n read message fails
				log.Fatal(err)
			}

			fmt.Println("sent a take aori order", msg.Id)
		}

	}
}
