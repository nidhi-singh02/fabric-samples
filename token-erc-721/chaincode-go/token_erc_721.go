package main

import (
	"log"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"github.com/hyperledger/fabric-samples/token-erc-721/chaincode-go/chaincode"
)

func main() {
	tokenChaincode, err := contractapi.NewChaincode(&chaincode.NFTContract{})
	if err != nil {
		log.Panicf("Error creating token-erc-721 chaincode: %v", err)
	}

	if err := tokenChaincode.Start(); err != nil {
		log.Panicf("Error starting token-erc-721 chaincode: %v", err)
	}
}
