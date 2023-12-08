package main

import (
	"github.com/FastLane-Labs/atlas-examples/cmd"
	"github.com/FastLane-Labs/atlas-examples/entities"
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

//
//func main() {
//	interrupt := make(chan os.Signal, 1)
//	signal.Notify(interrupt, os.Interrupt)
//	// subscribe to aori maker orders
//	// and create intent for each maker order
//	broadcastChan := make(chan []byte)
//	solutionChan := make(chan []byte)
//	takeOrderRequestChan := make(chan entities.TakeOrderRequest, 10)
//	//ws, err := NewWS("wss://feed.aori.io", authHeader)
//	ws, err := NewWS("wss://staging.feed.aori.io", authHeader)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	go func() {
//		subscribeSolutions(solutionChan)
//	}
//
//	go func(ws *WS) {
//		ws.subscribeToMakeOrders(broadcastChan)
//	}(ws)
//
//	go func(ws *WS) {
//		ws.sendTakeOrders(takeOrderRequestChan)
//	}(ws)
//
//	env, err := godotenv.Read(".env")
//	if err != nil {
//		log.Fatalf("could not load .env file: %s", err)
//	}
//
//	bundlerPrivateKey, err := crypto.ToECDSA(common.Hex2Bytes(env["BUNDLER_PK"]))
//	if err != nil {
//		log.Fatalf("could not load user's private key: %s", err)
//	}
//
//	gatewayClient, err := utils.NewGRPCClient(utils.DefaultRPCOpts(""))
//	if err != nil {
//		log.Fatalf("could not connect to gateway: %s", err)
//	}
//
//	signerAddress := crypto.PubkeyToAddress(bundlerPrivateKey.PublicKey)
//
//	go func() {
//		cancel := false
//		for {
//			select {
//
//			case intentOperation := <-broadcastChan:
//
//				intentHash := crypto.Keccak256Hash(intentOperation).Bytes()  // need a hash to sign, so we're hashing the payload here
//				intentSig, err := crypto.Sign(intentHash, bundlerPrivateKey) //signing the hash
//				if err != nil {
//					log.Fatalf("could not sign the message : %s", err)
//				}
//				intentRequest := &gateway.SubmitIntentRequest{
//					DappAddress: signerAddress.String(),
//					Intent:      intentOperation,
//					Hash:        intentHash,
//					Signature:   intentSig,
//					//AuthHeader:  "NjFhNTEyMjItMTgyNy00YmZhLWJhM2ItZmM4NWY1ZTI4NGZlOjAxZDk0YTUxMjYxMGE2NmUzZDFjY2I4YmJlMmE0Y2E1",
//				}
//
//				reply, err := gatewayClient.SubmitIntent(context.Background(), intentRequest)
//				if err != nil {
//					log.Fatalf("could not submit the intent : %s", err)
//				}
//
//				log.Printf("intent submitted to gateway with id=%s", reply.IntentId)
//
//				if cancel {
//					//cancel right away for testing
//					cancelHash := crypto.Keccak256Hash([]byte(reply.IntentId)).Bytes()
//					sign, err := crypto.Sign(cancelHash, bundlerPrivateKey)
//					if err != nil {
//						log.Fatalf("could not sign the intent cancellation : %s", err)
//					}
//
//					cancelReply, err := gatewayClient.CancelIntent(context.Background(), &gateway.CancelIntentRequest{
//						DappAddress: signerAddress.String(),
//						IntentId:    reply.IntentId,
//						Hash:        cancelHash,
//						Signature:   sign,
//						AuthHeader:  "NjFhNTEyMjItMTgyNy00YmZhLWJhM2ItZmM4NWY1ZTI4NGZlOjAxZDk0YTUxMjYxMGE2NmUzZDFjY2I4YmJlMmE0Y2E1",
//					})
//					if err != nil {
//						log.Fatalf("could not cancel the intent : %s", err)
//					}
//
//					if cancelReply.Status != "success" {
//						log.Fatalf("could not cancel the intent : %s", cancelReply.Reason)
//					}
//					log.Printf("intent canceled in the Gateway id=%s", reply.IntentId)
//					cancel = false
//					time.Sleep(time.Second)
//
//					// re-submit
//					secondReply, err := gatewayClient.SubmitIntent(context.Background(), intentRequest)
//					if err != nil {
//						log.Fatalf("could not submit the intent : %s", err)
//					}
//					log.Printf("intent submitted in the Gateway id=%s", secondReply.IntentId)
//				}
//
//			case solution := <-solutionChan:
//				takeOrderIntent := &entities.AoriTakeOrderIntent{}
//				err = json.Unmarshal(solution, takeOrderIntent)
//				if err != nil {
//					log.Fatalf("failed to unmarshal intent solution data")
//				}
//
//				takeOrderRequestChan <- takeOrderIntent.TakeOrderRequest
//				log.Printf("intent solution received from gateway with intentID=%s",
//					takeOrderIntent.IntentID)
//				log.Println("Received a solver operation")
//
//			}
//		}
//	}()
//	<-interrupt
//}
//
//func (w *WS) sendTakeOrders(broadcastTakeOrdersChan chan entities.TakeOrderRequest) {
//	w, err := NewWS("wss://dev.api.beta.order.aori.io/", authHeader)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	go func() {
//		for {
//			_, msg, err := w.conn.ReadMessage()
//			if err != nil {
//				// reconnect the websocket connection if connection read message fails
//				log.Println("error when reading makeOrder", err)
//				continue
//			}
//			log.Printf("message from aori %s", string(msg))
//		}
//	}()
//
//	for {
//		select {
//		case msg := <-broadcastTakeOrdersChan:
//			b, _ := json.Marshal(msg)
//			fmt.Println("sending this payload to aori", string(b))
//			err := w.conn.WriteMessage(websocket.TextMessage, b)
//			if err != nil {
//				log.Fatal(err)
//			}
//
//			log.Printf("sent a take aori order %s", msg.Id)
//		}
//	}
//}
//
//func startSendingTakeOrderDirectly(broadcastTakeOrdersChan chan entities.TakeOrderRequest) {
//
//}
