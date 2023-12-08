package entities

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"github.com/FastLane-Labs/atlas-examples/protobuf"
	"github.com/FastLane-Labs/atlas-examples/utils"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"math/rand"
	"time"

	"github.com/ethersphere/bee/pkg/crypto/eip712"
	"google.golang.org/grpc/metadata"
	"log"
	"math/big"
	"os"
	"strconv"
)

const (
	currentSeaportAddress = "0x00000000000000adc04c56bf30ac9d3c0aaf14dc"
	currentSeaportVersion = "1.5"
)

type AoriMakerOrderIntent struct {
	IntentID       string
	AoriMakerOrder AoriMakeOrder
}

type AoriTakeOrderIntent struct {
	IntentID         string
	TakeOrderRequest TakeOrderRequest
}

type TakeOrderRequestParams struct {
	OrderId string          `json:"orderId"`
	Order   AoriOrderDetail `json:"order"`
	SeatId  int             `json:"seatId"`
	ChainID int             `json:"chainId"`
}

type TakeOrderRequest struct {
	Id      int                      `json:"id"`
	Jsonrpc string                   `json:"jsonrpc"`
	Method  string                   `json:"method"`
	Params  []TakeOrderRequestParams `json:"params"`
}

type ConsiderationDetail struct {
	ItemType             uint8  `json:"itemType"`
	Token                string `json:"token"`
	IdentifierOrCriteria string `json:"identifierOrCriteria"`
	StartAmount          string `json:"startAmount"`
	EndAmount            string `json:"endAmount"`
	Recipient            string `json:"recipient"`
}

type OfferDetail struct {
	ItemType             uint8  `json:"itemType"`
	Token                string `json:"token"`
	IdentifierOrCriteria string `json:"identifierOrCriteria"`
	StartAmount          string `json:"startAmount"`
	EndAmount            string `json:"endAmount"`
}

type AoriOrderDetail struct {
	Parameters struct {
		Offerer                         string                `json:"offerer"`
		Zone                            string                `json:"zone"`
		ZoneHash                        string                `json:"zoneHash"`
		StartTime                       string                `json:"startTime"`
		EndTime                         string                `json:"endTime"`
		OrderType                       int                   `json:"orderType"`
		Offer                           []OfferDetail         `json:"offer"`
		Consideration                   []ConsiderationDetail `json:"consideration"`
		TotalOriginalConsiderationItems int                   `json:"totalOriginalConsiderationItems"`
		Salt                            string                `json:"salt"`
		ConduitKey                      string                `json:"conduitKey"`
		Counter                         string                `json:"counter"`
	} `json:"parameters"`
	Signature string `json:"signature"`
}

type AoriMakeOrder struct {
	Id     interface{} `json:"id"`
	Result struct {
		Type string `json:"type"`
		Data struct {
			Order         AoriOrderDetail `json:"order"`
			OrderHash     string          `json:"orderHash"`
			InputToken    string          `json:"inputToken"`
			OutputToken   string          `json:"outputToken"`
			InputAmount   string          `json:"inputAmount"`
			OutputAmount  string          `json:"outputAmount"`
			Rate          float64         `json:"rate"`
			ChainId       int             `json:"chainId"`
			Active        bool            `json:"active"`
			CreatedAt     int64           `json:"createdAt"`
			LastUpdatedAt int64           `json:"lastUpdatedAt"`
			IsPublic      bool            `json:"isPublic"`
		} `json:"data"`
	} `json:"result"`
}

type AoriSolver struct {
	signer        *bind.TransactOpts
	privateKey    *ecdsa.PrivateKey
	gatewayClient protobuf.GatewayClient

	// The channel where solvers receive new intent to fulfill
	incomingAoriIntentFromGateway chan *AoriMakerOrderIntent

	// The channel where solvers send their solutions to intents
	aoriMakeOrderIntent chan *AoriMakerOrderIntent

	shutdownChan <-chan struct{}

	log           *log.Logger
	dAppAddress   string
	solverAddress string
	chainId       int64
}

type EIP712OrderType struct {
	OrderComponents   []OrderComponent    `json:"OrderComponents"`
	OfferItem         []OfferItem         `json:"OfferItem"`
	ConsiderationItem []ConsiderationItem `json:"ConsiderationItem"`
}

type OrderComponent struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type OfferItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ConsiderationItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

var myTypes = make(eip712.Types)

func init() {
	myTypes["EIP712Domain"] = []eip712.Type{
		{Name: "name", Type: "string"},
		{Name: "version", Type: "string"},
		{Name: "chainId", Type: "uint256"},
		{Name: "verifyingContract", Type: "address"},
	}

	myTypes["OfferItem"] = []eip712.Type{
		{Name: "itemType", Type: "uint8"},
		{Name: "token", Type: "address"},
		{Name: "identifierOrCriteria", Type: "uint256"},
		{Name: "startAmount", Type: "uint256"},
		{Name: "endAmount", Type: "uint256"},
	}
	myTypes["ConsiderationItem"] = []eip712.Type{
		{Name: "itemType", Type: "uint8"},
		{Name: "token", Type: "address"},
		{Name: "identifierOrCriteria", Type: "uint256"},
		{Name: "startAmount", Type: "uint256"},
		{Name: "endAmount", Type: "uint256"},
		{Name: "recipient", Type: "address"},
	}
	myTypes["OrderComponents"] = []eip712.Type{
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

func NewAoriSolver(pk string, chainId int64, dappAddress string, solverName string, takeOrderRequestChan chan TakeOrderRequest) *AoriSolver {
	var logger *log.Logger

	logger = log.New(os.Stdout, solverName+"\t", log.LstdFlags|log.Lmsgprefix|log.Lmicroseconds)

	privateKey, err := crypto.ToECDSA(common.Hex2Bytes(pk))
	if err != nil {
		logger.Fatalf("could not load solver's private key: %s", err)
	}

	signer, err := bind.NewKeyedTransactorWithChainID(privateKey, big.NewInt(chainId))
	if err != nil {
		logger.Fatalf("could not initialize solver's signer: %s", err)
	}

	signerAddress := crypto.PubkeyToAddress(privateKey.PublicKey)
	fmt.Println("signerAddress : ", signerAddress)
	s := &AoriSolver{
		signer:                        signer,
		privateKey:                    privateKey,
		incomingAoriIntentFromGateway: make(chan *AoriMakerOrderIntent),
		aoriMakeOrderIntent:           make(chan *AoriMakerOrderIntent),
		log:                           logger,
		dAppAddress:                   dappAddress,
		solverAddress:                 signerAddress.String(),
		chainId:                       chainId,
	}

	s.log.Printf("creating solver with , dappAddress: %s", dappAddress)

	go s.run()
	return s
}

func (s *AoriSolver) run() {
	s.connectToBxGateway()
	for {
		select {
		case intent := <-s.incomingAoriIntentFromGateway:
			makerOrder := intent.AoriMakerOrder

			s.log.Printf("maker order from gateway: %s", makerOrder)

			solution := AoriMakerOrderIntent{
				AoriMakerOrder: makerOrder,
				IntentID:       intent.IntentID,
			}

			s.log.Printf("maker order after converting to AoriMakerOrderIntent gateway: %s", solution)

			s.aoriMakeOrderIntent <- &solution

		case <-s.shutdownChan:
			return
		}
	}
}

func (s *AoriSolver) connectToBxGateway() {
	client, err := utils.NewGRPCClient(utils.DefaultRPCOpts(""))
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
	authHeader := os.Getenv("AUTH_HEADER")
	ctx := context.Background()
	metadata.AppendToOutgoingContext(ctx, "authorization", authHeader)
	intentsStream, err := client.Intents(ctx, &protobuf.IntentsRequest{
		//Filters:       fmt.Sprintf("{DappAddress} == '%s'", s.dAppAddress),
		SolverAddress: signerAddress.String(),
		Hash:          hash,
		Signature:     sig,
		AuthHeader:    authHeader}) //ToDo implement correct request. See dapp example

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

			s.log.Printf("received make order payload from aori %s", string(swapIntentData.Intent))

			intent := &AoriMakerOrderIntent{
				IntentID: swapIntentData.IntentId,
			}

			err = json.Unmarshal(swapIntentData.Intent, &intent.AoriMakerOrder)
			if err != nil {
				s.log.Fatalf("failed to unmarshal swapIntentData: %w", err.Error())
			}

			s.log.Printf("received intent from gateway, intentID: %s, dAppAddress: %s, timestamp: %s, orderHash: %s",
				swapIntentData.IntentId,
				swapIntentData.DappAddress,
				swapIntentData.Timestamp,
				intent.AoriMakerOrder.Result.Data.OrderHash)

			intent.IntentID = swapIntentData.IntentId

			if intent.AoriMakerOrder.Result.Type == "OrderCreated" && intent.AoriMakerOrder.Result.Data.ChainId == 5 {
				//&&
				//	((intent.AoriMakerOrder.Result.Data.InputToken == "0xb4fbf271143f4fbf7b91a5ded31805e42b2208d6" && intent.AoriMakerOrder.Result.Data.OutputToken == "0x1f9840a85d5af5bf1d1762f925bdaddc4201f984") || (intent.AoriMakerOrder.Result.Data.InputToken == "0x1f9840a85d5af5bf1d1762f925bdaddc4201f984" && intent.AoriMakerOrder.Result.Data.OutputToken == "0xb4fbf271143f4fbf7b91a5ded31805e42b2208d6") ||
				//		(intent.AoriMakerOrder.Result.Data.InputToken == "0xb4fbf271143f4fbf7b91a5ded31805e42b2208d6" && intent.AoriMakerOrder.Result.Data.OutputToken == "0xb4fbf271143f4fbf7b91a5ded31805e42b2208d6") || (intent.AoriMakerOrder.Result.Data.InputToken == "0x1f9840a85d5af5bf1d1762f925bdaddc4201f984" && intent.AoriMakerOrder.Result.Data.OutputToken == "0x1f9840a85d5af5bf1d1762f925bdaddc4201f984"))

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
			solutionRequest := &protobuf.SubmitIntentSolutionRequest{
				SolverAddress:  signerAddress.String(),
				IntentId:       takeOrderIntent.IntentID,
				IntentSolution: solutionBytes,
				Hash:           solutionHash,
				Signature:      solutionSig,
				AuthHeader:     os.Getenv("AUTH_HEADER"),
			}

			s.log.Printf("creating and submitting intent solution request to send to gateway for intentID %s", takeOrderIntent.IntentID)

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

func toInt(str string) *big.Int {
	num, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		panic(err)
	}
	return big.NewInt(int64(num))
}

func addFee(str string) string {
	num, err := strconv.ParseFloat(str, 64)
	if err != nil {
		panic(err)
	}
	num = num * 1.0003
	return big.NewInt(int64(num)).String()
}

func signOrder(privateKey *ecdsa.PrivateKey, order *AoriTakeOrderIntent, chainID int64) (string, error) {
	domain := eip712.TypedDataDomain{
		Name:              "Seaport",
		Version:           currentSeaportVersion,
		ChainId:           math.NewHexOrDecimal256(chainID),
		VerifyingContract: currentSeaportAddress,
	}

	if len(order.TakeOrderRequest.Params) == 0 {
		return "", fmt.Errorf("params is required")
	}
	params := order.TakeOrderRequest.Params[0].Order.Parameters
	if len(params.Consideration) == 0 {
		return "", fmt.Errorf("consideration is required")
	}
	if len(params.Offer) == 0 {
		return "", fmt.Errorf("offer is required")
	}
	myMessage := make(eip712.TypedDataMessage)
	myMessage["offerer"] = params.Offerer
	myMessage["zone"] = params.Zone

	myMessage["offer"] = make(eip712.TypedDataMessage)
	offerSlice := make([]map[string]interface{}, 0)
	offerMap := map[string]interface{}{
		"itemType":             big.NewInt(int64(params.Offer[0].ItemType)),
		"token":                params.Offer[0].Token,
		"identifierOrCriteria": toInt(params.Offer[0].IdentifierOrCriteria),
		"startAmount":          toInt(params.Offer[0].StartAmount),
		"endAmount":            toInt(params.Offer[0].EndAmount),
	}
	offerSlice = append(offerSlice, offerMap)
	myMessage["offer"] = offerSlice

	considerationSlice := make([]map[string]interface{}, 0)
	considerationMap := map[string]interface{}{
		"itemType":             big.NewInt(int64(params.Consideration[0].ItemType)),
		"token":                params.Consideration[0].Token,
		"identifierOrCriteria": toInt(params.Consideration[0].IdentifierOrCriteria),
		"startAmount":          toInt(params.Consideration[0].StartAmount),
		"endAmount":            toInt(params.Consideration[0].EndAmount),
		"recipient":            params.Consideration[0].Recipient,
	}
	considerationSlice = append(considerationSlice, considerationMap)
	myMessage["consideration"] = considerationSlice

	myMessage["orderType"] = big.NewInt(int64(params.OrderType))
	myMessage["startTime"] = params.StartTime
	myMessage["endTime"] = params.EndTime
	myMessage["zoneHash"] = params.ZoneHash
	myMessage["salt"] = params.Salt
	myMessage["conduitKey"] = params.ConduitKey
	myMessage["counter"] = params.Counter

	typedDataIns := eip712.TypedData{
		Types:       myTypes,
		Domain:      domain,
		PrimaryType: "OrderComponents",
		Message:     myMessage,
	}
	sig, err := SignTypedData(&typedDataIns, privateKey)
	if err != nil {
		return "", err
	}
	if err != nil {
		return "", err
	}

	str := hexutil.Encode(sig)
	return str, nil
}

func SignTypedData(typedData *eip712.TypedData, privateKey *ecdsa.PrivateKey) ([]byte, error) {
	rawData, err := eip712.EncodeForSigning(typedData)
	if err != nil {
		return nil, err
	}

	sighash := crypto.Keccak256(rawData)
	if err != nil {
		return nil, err
	}
	sign, err := crypto.Sign(sighash, privateKey)
	return sign, err
}
