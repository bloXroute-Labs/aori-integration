package entities

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"github.com/gorilla/websocket"
	"log"
	"math/big"
	"net/http"
	"os"

	gateway "github.com/bloXroute-Labs/aori-integration/protobuf"
	"github.com/bloXroute-Labs/aori-integration/utils"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const aoriAuthHeader = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ0eXBlIjoiQXBpS2V5IiwiZW1haWwiOiJkYW5pZWwudGFvQGJsb3hyb3V0ZS5jb20iLCJuYW1lIjoiRGFuaWVsIFRhbyIsInJhdGVMaW1pdCI6MTAwLCJpYXQiOjE3MDAwNzcyOTd9.O6STgok-EJ6IcCoQSpTZPp4sGJGe4ShZhz1rJLQ3fOI"
const aoriStagingWSEndpoint = "wss://staging.feed.aori.io"
const aoriSendTakeOrderWSEndpoint = "wss://staging.api.aori.io/"

type AoriBackend struct {
	// The backend is the bundler in this example
	bundlerSigner     *bind.TransactOpts
	bundlerPrivateKey *ecdsa.PrivateKey

	gatewayClient gateway.GatewayClient

	wsFeedConn      *websocket.Conn
	wsTakeOrderConn *websocket.Conn

	// The channel where users send their swap intent operations
	makeOrderFromAoriChan chan []byte

	solutionChannel chan *AoriTakeOrderIntent

	shutdownChan <-chan struct{}

	log         *log.Logger
	intentCount int
	dappAddress common.Address
}

func NewAoriBackend(pk string, chainId int64, makeOrderChan chan []byte, shutdownChan chan struct{}) *AoriBackend {
	logger := log.New(os.Stdout, "[BACKEND]\t", log.LstdFlags|log.Lmsgprefix|log.Lmicroseconds)

	bundlerPrivateKey, err := crypto.ToECDSA(common.Hex2Bytes(pk))
	if err != nil {
		logger.Fatalf("could not load user's private key: %s", err)
	}

	bundlerSigner, err := bind.NewKeyedTransactorWithChainID(bundlerPrivateKey, big.NewInt(chainId))
	if err != nil {
		logger.Fatalf("could not initialize user's signer: %s", err)
	}

	wsFeedConn, err := connect(aoriStagingWSEndpoint, aoriAuthHeader)
	if err != nil {
		logger.Fatalf("could not initialize aori websocket connection: %s", err)
	}

	wsTakeOrderConn, err := connect(aoriSendTakeOrderWSEndpoint, aoriAuthHeader)

	if err != nil {
		logger.Fatal(err)
	}
	dappAddress := crypto.PubkeyToAddress(bundlerPrivateKey.PublicKey)

	a := &AoriBackend{
		bundlerSigner:         bundlerSigner,
		wsFeedConn:            wsFeedConn,
		wsTakeOrderConn:       wsTakeOrderConn,
		bundlerPrivateKey:     bundlerPrivateKey,
		dappAddress:           dappAddress,
		makeOrderFromAoriChan: makeOrderChan,
		solutionChannel:       make(chan *AoriTakeOrderIntent),
		shutdownChan:          shutdownChan,
		log:                   logger,
		intentCount:           0,
	}

	go a.run()

	return a
}

func (a *AoriBackend) GenerateMakeOrder(broadcastMakerOrdersChan chan []byte) {

	a.log.Printf("generating an aori make order")

	b := "{ \"id\": 12, \"jsonrpc\": \"2.0\", \"method\": \"aori_subscribeOrderbook\",\"params\": []}"
	err := a.wsFeedConn.WriteMessage(websocket.TextMessage, []byte(b))
	if err != nil {
		// reconnect the websocket connection if connection read message fails
		a.log.Fatal(err)
	}

	for {
		_, msg, err := a.wsFeedConn.ReadMessage()
		if err != nil {
			// reconnect the websocket connection if connection read message fails
			log.Println("error when reading makeOrder", err)
			continue
		}

		a.log.Printf("event from aori = %s", msg)

		var order AoriMakeOrder
		//var intent AoriMakerOrderIntent

		err = json.Unmarshal(msg, &order)
		if err != nil {
			//sample := "{\"id\":1,\"jsonrpc\":\"2.0\",\"method\":\"aori_makeOrder\",\"params\":[{\"chainId\":5,\"isPublic\":true,\"order\":{\"parameters\":{\"conduitKey\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"consideration\":[{\"endAmount\":\"50000000000000000\",\"identifierOrCriteria\":\"0\",\"itemType\":1,\"recipient\":\"0x6D2B5eA0212c4C32A92C6faB09081e2B4AAD9558\",\"startAmount\":\"50000000000000000\",\"token\":\"0x1f9840a85d5aF5bf1D1762F925BDADdC4201F984\"}],\"counter\":\"0\",\"endTime\":\"1701892654560\",\"offer\":[{\"endAmount\":\"50000000000000000\",\"identifierOrCriteria\":\"0\",\"itemType\":1,\"startAmount\":\"50000000000000000\",\"token\":\"0xB4FBF271143F4FBf7B91A5ded31805e42b2208d6\"}],\"offerer\":\"0x6D2B5eA0212c4C32A92C6faB09081e2B4AAD9558\",\"orderType\":3,\"salt\":\"0\",\"startTime\":\"1701806254560\",\"totalOriginalConsiderationItems\":1,\"zone\":\"0xeA2b4e7F02b859305093f9F4778a19D66CA176d5\",\"zoneHash\":\"0x0000000000000000000000000000000000000000000000000000000000000000\"},\"signature\":\"0xa091e814051cc8fa66925b34b74f0fc006d87e47979f4d19288bb5d2519aaa8e61b644543f1f1e48f4ed49dbd58b216b74a514c00b969a3eefb6654d1063f1881b\"}}]}"

			//sample := "\n{\"id\":null,\"result\":{\"type\":\"OrderCreated\",\"data\":{\"order\":{\"parameters\":{\"offerer\":\"0x8B71BF74ecCF1B362A0a219Df4bAC1C5ecdc04E2\",\"zone\":\"0xeA2b4e7F02b859305093f9F4778a19D66CA176d5\",\"zoneHash\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"startTime\":\"1700502726\",\"endTime\":\"1700589126\",\"orderType\":3,\"offer\":[{\"itemType\":1,\"token\":\"0xbEfDe6cCa7C2BF6a57Bdc2BB41567577c8935EDe\",\"identifierOrCriteria\":\"0\",\"startAmount\":\"1000000000000000000\",\"endAmount\":\"1000000000000000000\"}],\"consideration\":[{\"itemType\":1,\"token\":\"0x62FA33af921f620f9D04E9664b06807bB3BAD57B\",\"identifierOrCriteria\":\"0\",\"startAmount\":\"1000000000000000000\",\"endAmount\":\"1000000000000000000\",\"recipient\":\"0x8B71BF74ecCF1B362A0a219Df4bAC1C5ecdc04E2\"}],\"totalOriginalConsiderationItems\":1,\"salt\":\"6262716\",\"conduitKey\":\"0x0000000000000000000000000000000000000000000000000000000000000000\",\"counter\":\"0\"},\"signature\":\"\"},\"orderHash\":\"0x7f0f4889b5f803ff00fb4373c9128938fa386e751346852f47970395e416544a\",\"inputToken\":\"0xbEfDe6cCa7C2BF6a57Bdc2BB41567577c8935EDe\",\"outputToken\":\"0x62FA33af921f620f9D04E9664b06807bB3BAD57B\",\"inputAmount\":\"1000000000000000000\",\"outputAmount\":\"1000000000000000000\",\"rate\":1,\"chainId\":5,\"active\":true,\"createdAt\":1700502729915,\"lastUpdatedAt\":1700502729915,\"isPublic\":true}}}"
			//
			//err := json.Unmarshal([]byte(sample), &order)
			//if err != nil {
			//	panic(fmt.Errorf("error when unmarshalling %v", err))
			//}
			//
			//intent = AoriMakerOrderIntent{
			//	AoriMakerOrder: order,
			//}
			//
			//intentBytes, err := json.Marshal(intent)

			//broadcastMakerOrdersChan <- intentBytes

			// reconnect the websocket connection if connection read message fails
			continue
		} else {
			broadcastMakerOrdersChan <- msg
		}
	}
}

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

func (a *AoriBackend) run() {
	a.connectToBxGateway()

	for {
		cancel := false

		select {
		case makeOrder := <-a.makeOrderFromAoriChan:
			// this swap intent is coming from the user "makeOrderIntentsReceive" chan
			intentHash := crypto.Keccak256Hash(makeOrder).Bytes()          // need a hash to sign, so we're hashing the payload here
			intentSig, err := crypto.Sign(intentHash, a.bundlerPrivateKey) //signing the hash
			if err != nil {
				log.Printf("could not sign the message : %s", err)
				continue
			}

			intentRequest := &gateway.SubmitIntentRequest{
				DappAddress: a.dappAddress.String(),
				Intent:      makeOrder,
				Hash:        intentHash,
				Signature:   intentSig,
			}

			reply, err := a.gatewayClient.SubmitIntent(context.Background(), intentRequest)
			if err != nil {
				a.log.Printf("could not submit the intent : %s", err)
				continue
			}

			a.log.Printf("intent submitted to gateway with id=%s", reply.IntentId)

			if cancel {
				//cancel right away for testing
				cancelHash := crypto.Keccak256Hash([]byte(reply.IntentId)).Bytes()
				sign, err := crypto.Sign(cancelHash, a.bundlerPrivateKey)
				if err != nil {
					a.log.Printf("could not sign the intent cancellation : %s", err)
					continue
				}

				cancelReply, err := a.gatewayClient.CancelIntent(context.Background(), &gateway.CancelIntentRequest{
					DappAddress: a.dappAddress.String(),
					IntentId:    reply.IntentId,
					Hash:        cancelHash,
					Signature:   sign,
					AuthHeader:  "NjFhNTEyMjItMTgyNy00YmZhLWJhM2ItZmM4NWY1ZTI4NGZlOjAxZDk0YTUxMjYxMGE2NmUzZDFjY2I4YmJlMmE0Y2E1",
				})
				if err != nil {
					a.log.Printf("could not cancel the intent : %s", err)
					continue
				}

				if cancelReply.Status != "success" {
					a.log.Printf("could not cancel the intent : %s", cancelReply.Reason)
					continue
				}
				a.log.Printf("intent canceled in the Gateway id=%s", reply.IntentId)
				cancel = false
				time.Sleep(time.Second)

				// re-submit
				secondReply, err := a.gatewayClient.SubmitIntent(context.Background(), intentRequest)
				if err != nil {
					log.Printf("could not submit the intent : %s", err)
					continue
				}
				a.log.Printf("intent submitted in the Gateway id=%s", secondReply.IntentId)
			}

		case solution := <-a.solutionChannel:
			a.log.Println("received a take order intent solution with intent id: %s", solution.IntentID)

			go func() {
				for {
					_, msg, err := a.wsTakeOrderConn.ReadMessage()
					if err != nil {
						// reconnect the websocket connection if connection read message fails
						a.log.Printf("error when reading makeOrder", err)
						time.Sleep(time.Second)
						continue
					}
					a.log.Printf("message from aori %s", string(msg))
				}
			}()

			b, _ := json.Marshal(solution.TakeOrderRequest)
			a.log.Printf("sending solution payload to aori %s", string(b))
			err := a.wsTakeOrderConn.WriteMessage(websocket.TextMessage, b)
			if err != nil {
				a.log.Printf("got an error when sending take order %v", err)
				continue
			}

			a.log.Printf("sent a take aori order %s", solution.IntentID)

			a.intentCount += 1

		case <-a.shutdownChan:
			return
		}
	}
}

func (a *AoriBackend) connectToBxGateway() {
	a.log.Printf("connecting to the gateway")
	var err error
	a.gatewayClient, err = utils.NewGRPCClient(utils.DefaultRPCOpts(""))
	if err != nil {
		a.log.Fatalf("could not connect to gateway: %s", err)
	}

	hash := crypto.Keccak256Hash([]byte(a.dappAddress.String())).Bytes()
	sig, err := crypto.Sign(hash, a.bundlerPrivateKey)
	if err != nil {
		a.log.Fatalf("could not sign the message : %s", err)
	}

	stream, err := a.gatewayClient.IntentSolutions(context.Background(), &gateway.IntentSolutionsRequest{
		DappAddress: a.dappAddress.String(),
		Hash:        hash,
		Signature:   sig,
		AuthHeader:  "NjFhNTEyMjItMTgyNy00YmZhLWJhM2ItZmM4NWY1ZTI4NGZlOjAxZDk0YTUxMjYxMGE2NmUzZDFjY2I4YmJlMmE0Y2E1",
	})

	if err != nil {
		a.log.Fatalf("could not subscribe to intents stream: %s", err)
	}

	go func() {
		for {
			solutionData, err := stream.Recv()
			if err != nil {
				a.log.Printf("intents solution stream connection error: %s", err)
				continue
			}

			solution := &AoriTakeOrderIntent{}

			err = json.Unmarshal(solutionData.IntentSolution, solution)
			if err != nil {
				a.log.Printf("failed to unmarshal intent solution data")
				continue
			}

			a.log.Printf("intent solution received from gateway for intentID=%s, intentCount=%d",
				solutionData.IntentId,
				a.intentCount)

			a.solutionChannel <- solution

		}
	}()
}

func (a *AoriBackend) sendTakeOrders() {
	go func() {
		for {
			_, msg, err := a.wsTakeOrderConn.ReadMessage()
			if err != nil {
				// reconnect the websocket connection if connection read message fails
				log.Println("error when reading makeOrder", err)
				continue
			}
			log.Printf("message from aori %s", string(msg))
		}
	}()

}
