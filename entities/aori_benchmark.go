package entities

import (
	"context"
	"crypto/ecdsa"
	"encoding/csv"
	"encoding/json"
	gateway "github.com/bloXroute-Labs/aori-integration/protobuf"
	"github.com/bloXroute-Labs/aori-integration/utils"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"math/big"
	"math/rand"
	"os"
	"time"
)

type AoriBenchmarkSolver struct {
	signer        *bind.TransactOpts
	privateKey    *ecdsa.PrivateKey
	gatewayClient gateway.GatewayClient

	// The channel where solvers receive new intent to fulfill
	incomingAoriIntentFromGateway chan *AoriMakerOrderIntent

	// The channel where solvers send their solutions to intents
	aoriMakeOrderIntent chan *AoriMakerOrderIntent

	shutdownChan <-chan struct{}

	log        *log.Logger
	wsFeedConn *websocket.Conn

	csvWriter     *csv.Writer
	dAppAddress   string
	solverAddress string
	chainId       int64
}

func init() {
	myTypes["EIP712Domain"] = []apitypes.Type{
		{Name: "name", Type: "string"},
		{Name: "version", Type: "string"},
		{Name: "chainId", Type: "uint256"},
		{Name: "verifyingContract", Type: "address"},
	}

	myTypes["OfferItem"] = []apitypes.Type{
		{Name: "itemType", Type: "uint8"},
		{Name: "token", Type: "address"},
		{Name: "identifierOrCriteria", Type: "uint256"},
		{Name: "startAmount", Type: "uint256"},
		{Name: "endAmount", Type: "uint256"},
	}
	myTypes["ConsiderationItem"] = []apitypes.Type{
		{Name: "itemType", Type: "uint8"},
		{Name: "token", Type: "address"},
		{Name: "identifierOrCriteria", Type: "uint256"},
		{Name: "startAmount", Type: "uint256"},
		{Name: "endAmount", Type: "uint256"},
		{Name: "recipient", Type: "address"},
	}
	myTypes["OrderComponents"] = []apitypes.Type{
		{Name: "offerer", Type: "address"},
		{Name: "zone", Type: "address"},
		{Name: "offer", Type: "OfferItem[]"},
		{Name: "consideration", Type: "ConsiderationItem[]"},
		{Name: "orderType", Type: "uint8"},
		{Name: "startTime", Type: "uint256"},
		{Name: "endTime", Type: "uint256"},
		{Name: "zoneHash", Type: "bytes32"},
		{Name: "salt", Type: "uint256"},
		{Name: "conduitKey", Type: "bytes32"},
		{Name: "counter", Type: "uint256"},
	}
}

func NewAoriBenchmarkSolver(pk string, chainId int64, dappAddress string, solverName string, takeOrderRequestChan chan TakeOrderRequest) *AoriBenchmarkSolver {
	logFile, err := os.OpenFile("benchmark.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	mw := io.MultiWriter(logFile, os.Stdout)

	logger := log.New(mw, "[BENCHMARK]\t", log.LstdFlags|log.Lmsgprefix|log.Lmicroseconds)
	logger.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)

	csvFile, err := os.Create("benchmark.csv")
	if err != nil {
		log.Fatal(err)
	}

	csvWriter := csv.NewWriter(csvFile)

	// Write headers
	headers := []string{"Timestamp", "Name", "OrderHash"}
	err = csvWriter.Write(headers)

	privateKey, err := crypto.ToECDSA(common.Hex2Bytes(pk))
	if err != nil {
		logger.Fatalf("could not load solver's private key: %s", err)
	}

	signer, err := bind.NewKeyedTransactorWithChainID(privateKey, big.NewInt(chainId))
	if err != nil {
		logger.Fatalf("could not initialize solver's signer: %s", err)
	}

	wsFeedConn, err := connect(aoriStagingWSEndpoint, aoriAuthHeader)
	if err != nil {
		logger.Fatalf("could not initialize aori websocket connection: %s", err)
	}

	signerAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	s := &AoriBenchmarkSolver{
		signer:                        signer,
		privateKey:                    privateKey,
		wsFeedConn:                    wsFeedConn,
		incomingAoriIntentFromGateway: make(chan *AoriMakerOrderIntent),
		aoriMakeOrderIntent:           make(chan *AoriMakerOrderIntent),
		log:                           logger,
		dAppAddress:                   dappAddress,
		solverAddress:                 signerAddress.String(),
		csvWriter:                     csvWriter,
		chainId:                       chainId,
	}

	s.log.Printf("creating benchmarker with , dappAddress: %s", dappAddress)

	go s.run()
	return s
}

func (a *AoriBenchmarkSolver) GenerateMakeOrder() {

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
		}
	}
}

func (s *AoriBenchmarkSolver) run() {
	s.connectToBxGateway()
	for {
		select {
		case intent := <-s.incomingAoriIntentFromGateway:
			makerOrder := intent.AoriMakerOrder

			s.log.Printf("maker order received from gateway: %s", makerOrder)

			solution := AoriMakerOrderIntent{
				AoriMakerOrder: makerOrder,
				IntentID:       intent.IntentID,
			}

			//s.log.Printf("maker order after converting to AoriMakerOrderIntent gateway: %s", solution)

			s.aoriMakeOrderIntent <- &solution

		case <-s.shutdownChan:
			return
		}
	}
}

func (s *AoriBenchmarkSolver) connectToBxGateway() {
	client, err := utils.NewGRPCClient(utils.DefaultRPCOpts("54.254.48.158:5005"))
	if err != nil {
		s.log.Fatalf("could not connect to gateway: %s", err)
	}

	signerAddress := crypto.PubkeyToAddress(s.privateKey.PublicKey)

	hash := crypto.Keccak256Hash([]byte(signerAddress.String())).Bytes()
	sig, err := crypto.Sign(hash, s.privateKey)
	if err != nil {
		s.log.Fatalf("could not sign the message : %s", err)
	}

	// this context is in place to not have duplicate requests between solvers
	//authHeader := os.Getenv("AUTH_HEADER")
	//ctx := context.Background()
	//metadata.AppendToOutgoingContext(ctx, "authorization", authHeader)
	intentsStream, err := client.Intents(context.Background(), &gateway.IntentsRequest{
		// Filters:       fmt.Sprintf("{DappAddress} == '%s'", s.dAppAddress),
		SolverAddress: signerAddress.String(),
		Hash:          hash,
		Signature:     sig})
	//AuthHeader:    authHeader})

	if err != nil {
		s.log.Fatalf("could not subscribe to intents stream: %s", err)
	}

	s.log.Printf("successfully subscribed to gateway intent stream, waiting on intents...")

	go func() {
		for {
			swapIntentData, err := intentsStream.Recv()

			if err != nil {
				s.log.Printf("error receiving an intent from gateway intent stream: %s", err)
				time.Sleep(time.Second)
				continue
			}

			intent := &AoriMakerOrderIntent{
				IntentID: swapIntentData.IntentId,
			}

			err = json.Unmarshal(swapIntentData.Intent, &intent.AoriMakerOrder)
			if err != nil {
				s.log.Fatalf("failed to unmarshal swapIntentData: %w", err.Error())
			}

			s.log.Printf("received intent from gateway with intentID:%s, aoriOrderHash:%s", swapIntentData.IntentId, intent.AoriMakerOrder.Result.Data.OrderHash)
			err = s.csvWriter.Write([]string{time.Now().Format(time.RFC3339), "receiveFromGateway", intent.AoriMakerOrder.Result.Data.OrderHash})
			if err != nil {
				log.Fatal(err)
			}

			if intent.AoriMakerOrder.Result.Type == "OrderCreated" && intent.AoriMakerOrder.Result.Data.ChainId == 5 &&
				hasRightToken(intent) {

				intent.IntentID = swapIntentData.IntentId

				s.incomingAoriIntentFromGateway <- intent
			} else {
				continue
			}
		}
	}()

	go func() {
		for {
			makeOrderIntent := <-s.aoriMakeOrderIntent

			s.log.Printf("make order intent %s", makeOrderIntent)

			offers := makeOrderIntent.AoriMakerOrder.Result.Data.Order.Parameters.Offer
			makeOrderIntent.AoriMakerOrder.Result.Data.Order.Parameters.Offerer = s.solverAddress
			var newConsiderations []ConsiderationDetail

			for i := range offers {
				newConsiderations = append(newConsiderations, ConsiderationDetail{
					ItemType:             offers[i].ItemType,
					Token:                offers[i].Token,
					IdentifierOrCriteria: offers[i].IdentifierOrCriteria,
					StartAmount:          offers[i].StartAmount,
					EndAmount:            offers[i].EndAmount,
					Recipient:            s.solverAddress,
				})
			}

			considerations := makeOrderIntent.AoriMakerOrder.Result.Data.Order.Parameters.Consideration
			var newOffers []OfferDetail
			for i := range considerations {
				newOffers = append(newOffers, OfferDetail{
					ItemType:             considerations[i].ItemType,
					Token:                considerations[i].Token,
					IdentifierOrCriteria: considerations[i].IdentifierOrCriteria,
					StartAmount:          addFee(considerations[i].StartAmount),
					EndAmount:            addFee(considerations[i].EndAmount),
				})
			}
			makeOrderIntent.AoriMakerOrder.Result.Data.Order.Parameters.Consideration = newConsiderations
			makeOrderIntent.AoriMakerOrder.Result.Data.Order.Parameters.Offer = newOffers
			takeOrder := TakeOrderRequest{
				Id:      rand.Int() % 1000000,
				Jsonrpc: "2.0",
				Method:  "aori_takeOrder",
				Params: []TakeOrderRequestParams{
					{
						ChainID: makeOrderIntent.AoriMakerOrder.Result.Data.ChainId,
						Order:   makeOrderIntent.AoriMakerOrder.Result.Data.Order,
						OrderId: makeOrderIntent.AoriMakerOrder.Result.Data.OrderHash,
						SeatId:  0,
					},
				},
			}

			takeOrderIntent := AoriTakeOrderIntent{
				IntentID:         makeOrderIntent.IntentID,
				TakeOrderRequest: takeOrder,
			}

			signature, err := signOrder(s.privateKey, &takeOrderIntent, s.chainId)
			if err != nil {
				log.Fatal(err)
			}

			takeOrderIntent.TakeOrderRequest.Params[0].Order.Signature = signature

			solutionBytes, err := json.Marshal(takeOrderIntent)
			if err != nil {
				s.log.Fatalf("failed to marshal solution %w", err)
			}

			solutionHash := crypto.Keccak256Hash(solutionBytes).Bytes() // need a hash to sign, so we're hashing the payload here
			solutionSig, err := crypto.Sign(solutionHash, s.privateKey) //signing the hash
			if err != nil {
				s.log.Fatal("could not sign the message : %s", err)
			}

			signerAddress := crypto.PubkeyToAddress(s.privateKey.PublicKey)
			solutionRequest := &gateway.SubmitIntentSolutionRequest{
				SolverAddress:  signerAddress.String(),
				IntentId:       takeOrderIntent.IntentID,
				IntentSolution: solutionBytes,
				Hash:           solutionHash,
				Signature:      solutionSig,
				//AuthHeader:     os.Getenv("AUTH_HEADER"),
			}

			s.log.Printf("creating and submitting intent solution request to send to gateway for intentID %s", takeOrderIntent.IntentID)

			//client2, err := utils.NewGRPCClient(utils.DefaultRPCOpts("localhost:5005"))
			//if err != nil {
			//	s.log.Fatalf("could not connect to gateway: %s", err)
			//}

			intentSolutionReply, err := client.SubmitIntentSolution(context.Background(), solutionRequest)
			if err != nil {
				s.log.Printf("could not submit intent solution to gateway: %s", err.Error())
			}

			if err == nil {
				s.log.Printf("intent solution submitted to gateway with solutionID: %s", intentSolutionReply.SolutionId)
			}
		}
	}()
}
