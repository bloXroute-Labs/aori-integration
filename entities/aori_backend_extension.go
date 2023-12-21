package entities

import (
	"context"
	"crypto/ecdsa"
	"encoding/csv"
	"encoding/json"
	"github.com/bloXroute-Labs/aori-integration/utils"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"math/big"
	"os"

	gateway "github.com/bloXroute-Labs/aori-integration/protobuf"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type writer struct {
	io.Writer
	timeFormat string
}

func (w writer) Write(b []byte) (n int, err error) {
	return w.Writer.Write(append([]byte(time.Now().Format(w.timeFormat)), b...))
}

type makeOrderFromAoriChanWithOrderHash struct {
	makeOrderFromAoriChan []byte
	orderHash             string
}

type AoriExtension struct {
	// The backend is the bundler in this example
	bundlerSigner     *bind.TransactOpts
	bundlerPrivateKey *ecdsa.PrivateKey

	gatewayClient gateway.GatewayClient

	wsFeedConn      *websocket.Conn
	wsTakeOrderConn *websocket.Conn

	// The channel where users send their swap intent operations
	makeOrderFromAoriChan chan []byte
	makeOrderWithHashChan chan *makeOrderFromAoriChanWithOrderHash

	solutionChannel chan *AoriTakeOrderIntent

	shutdownChan <-chan struct{}

	log         *log.Logger
	csvWriter   *csv.Writer
	intentCount int
	dappAddress common.Address
}

func NewAoriExtension(pk string, chainId int64, makeOrderChan chan []byte, shutdownChan chan struct{}) *AoriExtension {
	logFile, err := os.OpenFile("dapp_extension.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	mw := io.MultiWriter(logFile, os.Stdout)

	logger := log.New(mw, "[BACKEND_EXTENSION]\t", log.LstdFlags|log.Lmsgprefix|log.Lmicroseconds)
	logger.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)

	csvFile, err := os.Create("dapp_extension.csv")
	if err != nil {
		log.Fatal(err)
	}

	csvWriter := csv.NewWriter(csvFile)

	// Write headers
	headers := []string{"Timestamp", "Name", "OrderHash"}
	err = csvWriter.Write(headers)

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

	a := &AoriExtension{
		bundlerSigner:         bundlerSigner,
		wsFeedConn:            wsFeedConn,
		wsTakeOrderConn:       wsTakeOrderConn,
		bundlerPrivateKey:     bundlerPrivateKey,
		dappAddress:           dappAddress,
		makeOrderFromAoriChan: makeOrderChan,
		solutionChannel:       make(chan *AoriTakeOrderIntent),
		shutdownChan:          shutdownChan,
		makeOrderWithHashChan: make(chan *makeOrderFromAoriChanWithOrderHash),
		log:                   logger,
		intentCount:           0,
		csvWriter:             csvWriter,
	}

	go a.run()

	return a
}

func (a *AoriExtension) GenerateMakeOrder(broadcastMakerOrdersChan chan []byte) {
	//a.log.Printf("generating an aori make order")

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

		//a.log.Printf("event received from aori = %s", msg)

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
			a.log.Printf("received make order from aori feed, orderHash: %s", order.Result.Data.OrderHash)
			//broadcastMakerOrdersChan <- msg

			err := a.csvWriter.Write([]string{time.Now().Format(time.RFC3339), "receiveAoriFeed", order.Result.Data.OrderHash})
			if err != nil {
				log.Fatal(err)
			}

			var wrappedMsg makeOrderFromAoriChanWithOrderHash
			wrappedMsg.makeOrderFromAoriChan = msg
			wrappedMsg.orderHash = order.Result.Data.OrderHash

			a.makeOrderWithHashChan <- &wrappedMsg

		}
	}
}

func (a *AoriExtension) run() {
	a.connectToBxGateway()

	for {
		//cancel := false

		select {
		case makeOrder := <-a.makeOrderWithHashChan:
			// this swap intent is coming from the user "makeOrderIntentsReceive" chan
			intentHash := crypto.Keccak256Hash(makeOrder.makeOrderFromAoriChan).Bytes() // need a hash to sign, so we're hashing the payload here
			intentSig, err := crypto.Sign(intentHash, a.bundlerPrivateKey)              //signing the hash
			if err != nil {
				log.Fatalf("could not sign the message : %s", err)
			}

			intentRequest := &gateway.SubmitIntentRequest{
				DappAddress: a.dappAddress.String(),
				Intent:      makeOrder.makeOrderFromAoriChan,
				Hash:        intentHash,
				Signature:   intentSig,
			}

			err = a.csvWriter.Write([]string{time.Now().Format(time.RFC3339), "submitGateway", makeOrder.orderHash})
			if err != nil {
				log.Fatal(err)
			}

			a.log.Printf("submitting intent to gateway with aori order hash %s", makeOrder.orderHash)

			reply, err := a.gatewayClient.SubmitIntent(context.Background(), intentRequest)
			if err != nil {
				a.log.Fatalf("could not submit the intent : %s", err)
			}

			a.log.Printf("intent submitted to gateway with id=%s", reply.IntentId)

			//if cancel {
			//	//cancel right away for testing
			//	cancelHash := crypto.Keccak256Hash([]byte(reply.IntentId)).Bytes()
			//	sign, err := crypto.Sign(cancelHash, a.bundlerPrivateKey)
			//	if err != nil {
			//		a.log.Fatalf("could not sign the intent cancellation : %s", err)
			//	}
			//
			//	cancelReply, err := a.gatewayClient.CancelIntent(context.Background(), &gateway.CancelIntentRequest{
			//		DappAddress: a.dappAddress.String(),
			//		IntentId:    reply.IntentId,
			//		Hash:        cancelHash,
			//		Signature:   sign,
			//		AuthHeader:  "NjFhNTEyMjItMTgyNy00YmZhLWJhM2ItZmM4NWY1ZTI4NGZlOjAxZDk0YTUxMjYxMGE2NmUzZDFjY2I4YmJlMmE0Y2E1",
			//	})
			//	if err != nil {
			//		a.log.Fatalf("could not cancel the intent : %s", err)
			//	}
			//
			//	if cancelReply.Status != "success" {
			//		a.log.Fatalf("could not cancel the intent : %s", cancelReply.Reason)
			//	}
			//	a.log.Printf("intent canceled in the Gateway id=%s", reply.IntentId)
			//	cancel = false
			//	time.Sleep(time.Second)
			//
			//	// re-submit
			//	secondReply, err := a.gatewayClient.SubmitIntent(context.Background(), intentRequest)
			//	if err != nil {
			//		log.Fatalf("could not submit the intent : %s", err)
			//	}
			//	a.log.Printf("intent submitted in the Gateway id=%s", secondReply.IntentId)
			//}

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
				a.log.Fatal(err)
			}

			a.log.Printf("sent a take aori order %s", solution.IntentID)

			a.intentCount += 1

		case <-a.shutdownChan:
			return
		}
	}
}

func (a *AoriExtension) connectToBxGateway() {
	a.log.Printf("connecting to the gateway")
	var err error
	//a.gatewayClient, err = utils.NewGRPCClient(utils.DefaultRPCOpts("54.144.86.226:5005"))
	a.gatewayClient, err = utils.NewGRPCClient(utils.DefaultRPCOpts("3.74.153.37:5005"))

	if err != nil {
		a.log.Fatalf("could not connect to gateway: %s", err)
	}

	//hash := crypto.Keccak256Hash([]byte(a.dappAddress.String())).Bytes()
	//sig, err := crypto.Sign(hash, a.bundlerPrivateKey)
	//if err != nil {
	//	a.log.Fatalf("could not sign the message : %s", err)
	//}
	//
	//stream, err := a.gatewayClient.IntentSolutions(context.Background(), &gateway.IntentSolutionsRequest{
	//	DappAddress: a.dappAddress.String(),
	//	Hash:        hash,
	//	Signature:   sig,
	//	//AuthHeader:  "NjFhNTEyMjItMTgyNy00YmZhLWJhM2ItZmM4NWY1ZTI4NGZlOjAxZDk0YTUxMjYxMGE2NmUzZDFjY2I4YmJlMmE0Y2E1",
	//})

	//if err != nil {
	//	a.log.Fatalf("could not subscribe to intents stream: %s", err)
	//}

	//go func() {
	//solutionData, err := stream.Recv()
	//if err != nil {
	//	a.log.Fatalf("intents solution stream connection error: %s", err)
	//}
	//
	//solution := &AoriTakeOrderIntent{}
	//
	//err = json.Unmarshal(solutionData.IntentSolution, solution)
	//if err != nil {
	//	a.log.Fatalf("failed to unmarshal intent solution data")
	//}
	//
	//a.log.Printf("intent solution received from gateway for intentID=%s, intentCount=%d",
	//	solutionData.IntentId,
	//	a.intentCount)
	//
	//a.solutionChannel <- solution

	//}
	//}()
}

func (a *AoriExtension) sendTakeOrders(aoriTakeOrdersRequest TakeOrderRequest) {
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
