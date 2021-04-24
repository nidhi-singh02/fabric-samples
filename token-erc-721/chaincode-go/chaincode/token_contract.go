package main

import (
	"encoding/json"
	"fmt"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// eventtoken provides an organized struct for emitting Token events
type eventtoken struct {
	from    string
	to      string
	TokenID int
}

type eventApprovedForAll struct {
	owner    string
	operator string
	approved bool
}

type eventApproved struct {
	owner    string
	approved string
	TokenID  int
}

const approvalPrefix = "approval"
const nftPrefix = "nft"
const balancePrefix = "balance"

// Define key names for options
const nameKey = "name"
const symbolKey = "symbol"

// NFTContract provides functions for  transferring NFT between accounts
type NFTContract struct {
	contractapi.Contract
}

type Token struct {
	TokenID  int    `json:"tokenID"`
	TokenURI string `json:"tokenURI"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Owner    string `json:"owner"`
	Approved string `json:"approved"`
}

//Mint a new non-fungible token
func (n *NFTContract) MintWithTokenURI(ctx contractapi.TransactionContextInterface, TokenID string, TokenURI string) error {

	// Check minter authorization - this sample assumes Org1 is the issuer with privilege to mint a new token
	clientMSPID := ctx.GetClientIdentity().getMSPID()
	if clientMSPID != "Org1MSP" {
		return fmt.Errorf("client is not authorized to mint new tokens")
	}

	// Get ID of submitting client identity
	minter := ctx.GetClientIdentity().getID()

	//Check if the token to be minted does not exist
	tokens, err := ReadNFT(ctx, TokenID)

	if err != nil {
		return fmt.Errorf("Cannot get token for %v : %v", TokenID, err)
	}

	if tokens.Owner != "" {
		return fmt.Errorf("token %v is already minted", TokenID)

	}

	TokenIDInt, err_conv := strconv.Atoi(TokenID)
	if err_conv != nil {
		return fmt.Errorf("tokenID  %v is invalid. tokenId must be an integer .%v", TokenID, err)
	}

	token := token{TokenID: TokenIDInt, TokenURI: TokenURI, Owner: minter}
	tokenJSON, err := json.Marshal(token)
	if err != nil {
		return err
	}

	nftKey := ctx.GetStub().CreateCompositeKey(nftPrefix, []string{TokenID})
	ctx.GetStub().PutState(nftKey, tokenJSON)

	// A composite key would be balancePrefix.owner.tokenId, which enables partial
	// composite key query to find and count all records matching balance.owner.*
	// An empty value would represent a delete, so we simply insert the null character.
	balanceKey := ctx.GetStub().CreateCompositeKey(balancePrefix, []string{minter, TokenID})
	ctx.GetStub().PutState(balanceKey, Buffer.from('\u0000'))

	// Emit the Transfer event
	transferEvent := eventtoken{"0x0", minter, TokenIDInt}
	transferEventJSON, err := json.Marshal(transferEvent)
	if err != nil {
		return fmt.Errorf("failed to obtain JSON encoding: %v", err)
	}
	err = ctx.GetStub().SetEvent("Transfer", transferEventJSON)
	if err != nil {
		return fmt.Errorf("failed to set event: %v", err)
	}

	return nil

}

//This function 'TransferFrom' to be used for transferring the ownership of a non-fungible token
//from one owner to another owner
func (n *NFTContract) TransferFrom(ctx contractapi.TransactionContextInterface, from string, to string, TokenID string) error {

	// Get ID of submitting client identity
	sender := ctx.GetClientIdentity().getID()

	TokenIDInt, err_conv := strconv.Atoi(TokenID)
	if err_conv != nil {
		return fmt.Errorf("tokenID  %v is invalid. tokenId must be an integer .%v", TokenID, err)
	}

	//Check TokenID exists or not
	tokens, err := ReadNFT(ctx, TokenID)

	if err != nil {
		return fmt.Errorf("Cannot get token for %v : %v", TokenID, err)
	}
	// Check if `from` is the current owner of the token
	owner := tokens.Owner
	approved := tokens.Approved

	operatorApproval, err := n.IsApprovedForAll(ctx, owner, from)

	if err != nil {
		return fmt.Errorf("Error getting approval for owner %v from %v is:%v", from, owner, err)

	}

	if owner != from && approved != from && !operatorApproval {
		return fmt.Errorf("from %v is not the current owner %v nor authorized operator of token %v", from, owner, TokenID)
	}
	// Overwrite a non-fungible token to assign a new owner.
	tokens.Owner = to

	// Clear the approved client for this non-fungible token

	tokens.Approved = ""

	tokenJSON, err := json.Marshal(tokens)
	if err != nil {
		return err
	}

	nftKey := ctx.GetStub().CreateCompositeKey(nftPrefix, []string{TokenID})

	err = ctx.GetStub().PutState(nftKey, tokenJSON)
	if err != nil {
		return fmt.Errorf("failed to put token %v  : %v", TokenID, err)
	}

	// Remove a composite key from the balance of the current owner
	balanceKeyFrom := ctx.GetStub().CreateCompositeKey(balancePrefix, []string{from, TokenID})
	err = ctx.GetStub().DeleteState(balanceKeyFrom)
	if err != nil {
		return fmt.Errorf("failed to delete composite key for balance %v  :", err)
	}
	// Save a composite key to count the balance of a new owner
	balanceKeyTo := ctx.GetStub().CreateCompositeKey(balancePrefix, []string{to, TokenID})
	err = ctx.GetStub().PutState(balanceKeyTo, Buffer.from('\u0000'))
	if err != nil {
		return fmt.Errorf("failed to put balance %v  : %v", balanceKeyTo, err)
	}

	// Emit the Transfer event
	transferEvent := eventtoken{from, to, TokenIDInt}
	transferEventJSON, err := json.Marshal(transferEvent)
	if err != nil {
		return fmt.Errorf("failed to obtain JSON encoding: %v", err)
	}
	err = ctx.GetStub().SetEvent("Transfer", transferEventJSON)
	if err != nil {
		return fmt.Errorf("failed to set event: %v", err)
	}

	return nil

}

//Approve changes or reaffirms the approved client for a non-fungible token
//approved :The new approved client
func (n *NFTContract) Approve(ctx contractapi.TransactionContextInterface, approved string, TokenID string) bool {

	//  Approval is  allowed only to the current owner of the token or an authorized person.

	TokenIDInt, err_conv := strconv.Atoi(TokenID)
	if err_conv != nil {
		return fmt.Errorf("tokenID  %v is invalid. tokenId must be an integer .%v", TokenID, err)
	}

	sender := ctx.GetClientIdentity().getID()
	tokens, err := ReadNFT(ctx, TokenID)

	if err != nil {
		fmt.Errorf("Cannot get token for %v : %v", TokenID, err)
		return false

	}

	tokenOwner := tokens.Owner

	//Check approved account exists or not
	ApprovedBytes, err := ctx.GetStub().GetState(approved)
	if err != nil {
		fmt.Errorf("failed to read 'approved' account %s : %v", approved, err)
		return false
	}

	if ApprovedBytes == nil {
		fmt.Errorf("'Approved' account %s is invalid.It does not exist", approved)
		return false

	}

	//Check 'owner' passed in is an authorized operator of the current owner
	operatorApproval, err := n.IsApprovedForAll(ctx, tokenOwner, sender)

	if err != nil {
		fmt.Errorf("Error getting approval for owner %v from %v is :%v", tokenOwner, sender, err)
		return false
	}
	//Check owner is the current owner of the token or
	//authorized operator of the current owner
	if sender != tokenOwner && !operatorApproval {
		fmt.Errorf("sender %v is not correct owner nor authorized person for token %v", sender, TokenID)
		return false
	}
	// Update the approved client of the non-fungible token
	tokens.Approved = approved

	tokensJSON, err := json.Marshal(tokens)
	if err != nil {
		fmt.Errorf("failed to marshal token %v", err)
		return false
	}

	nftKey, err := ctx.GetStub.CreateCompositeKey(nftPrefix, []string{TokenID})

	err = ctx.GetStub().PutState(nftKey, tokensJSON)
	if err != nil {
		fmt.Errorf("failed to put token %v : %v", TokenID, err)
		return false
	}

	// Emit the Approval event
	approvalEvent := eventApproved{owner, approved, TokenIDInt}

	approvalEventJSON, err := json.Marshal(approvalEvent)
	if err != nil {
		fmt.Errorf("failed to obtain JSON encoding: %v", err)
		return false
	}
	err = ctx.GetStub().SetEvent("Approval", approvalEventJSON)
	if err != nil {
		fmt.Errorf("failed to set event: %v", err)
		return false
	}

	return true

}

//SetApprovalForAll enables or disables approval for a third party ("operator")
//to manage all of message sender's assets
func (n *NFTContract) SetApprovalForAll(ctx contractapi.TransactionContextInterface, operator string, approved bool) (bool, error) {

	sender := ctx.GetClientIdentity().getID()

	// Create approvalKey
	approvalKey, err := ctx.GetStub().CreateCompositeKey(approvalPrefix, []string{sender, operator})
	if err != nil {
		return false, fmt.Errorf("failed to create the composite key for prefix %s: %v", approvalPrefix, err)
	}

	approval := eventApprovedForAll{sender, operator, approved}

	approvalJSON, err := json.Marshal(approval)
	if err != nil {
		return false, fmt.Errorf("failed to obtain JSON encoding: %v", err)
	}

	// Update the state of the smart contract by adding the approvalKey and value
	err = ctx.GetStub().PutState(approvalKey, approvalJSON)
	if err != nil {
		return false, fmt.Errorf("failed to update state of smart contract for key %s: %v", approvalKey, err)
	}

	// Emit the ApprovalForAll event
	approvalForAllEvent := eventApprovedForAll{sender, operator, approved}

	approvalEventJSON, err := json.Marshal(approvalForAllEvent)
	if err != nil {
		return false, fmt.Errorf("failed to obtain JSON encoding: %v", err)
	}
	err = ctx.GetStub().SetEvent("ApprovalForAll", approvalEventJSON)
	if err != nil {
		return false, fmt.Errorf("failed to set event: %v", err)
	}

	return true, nil
}

//IsApprovedForAll returns if a client is an authorized operator for another client
func (n *NFTContract) IsApprovedForAll(ctx contractapi.TransactionContextInterface, owner string, operator string) (bool, error) {

	// Create approvalKey
	approvalKey, err := ctx.GetStub().CreateCompositeKey(approvalPrefix, []string{owner, operator})
	if err != nil {
		return false, fmt.Errorf("failed to create the composite key for prefix %s: %v", approvalPrefix, err)
	}

	ApprovalBytes, err := ctx.GetStub().GetState(approvalKey)
	if err != nil {
		return false, fmt.Errorf("failed to read approval key %s from world state: %v", approvalKey, err)
	}
	var approved bool
	var ApprovalData eventApprovedForAll
	if ApprovalBytes != nil {
		_ = json.Unmarshal(ApprovalBytes, &ApprovalData)
		approved = ApprovalData.approved
	} else {
		approved = false
	}

	return approved, nil

}

//GetApproved returns the approved client for a single non-fungible token
func (n *NFTContract) GetApproved(ctx contractapi.TransactionContextInterface, TokenID string) (string, error) {
	token, err := ReadNFT(ctx, TokenID)
	if err != nil {
		return "", fmt.Errorf("Cannot get token for %v : %v", TokenID, err)

	}
	return token.Approved, nil
}

func ReadNFT(ctx contractapi.TransactionContextInterface, TokenID string) (token, error) {

	nftKey := ctx.GetStub().CreateCompositeKey(nftPrefix, []string{TokenID})

	nftBytes, err := ctx.GetStub().GetState(nftKey)
	if err != nil {
		return token{}, fmt.Errorf("nftKey %s can't be read: %v", nftKey, err)
	}

	var tokenData Token
	if !nftBytes || nftBytes.length == 0 {
		return token{}, fmt.Errorf("TokenID %s is invalid. It does not exist", TokenID)
	}

	err = json.Unmarshal(nftBytes, &tokenData)

	if err != nil {
		return token{}, fmt.Errorf("Unmarshalling failed :%v", err)

	}

	return tokenData, nil

}

//OwnerOf finds the owner of a non-fungible token
func (n *NFTContract) OwnerOf(ctx contractapi.TransactionContextInterface, TokenID string) (string, error) {

	token, err := ReadNFT(ctx, TokenID)
	if err != nil {
		return "", fmt.Errorf("Cannot get token for %v : %v", TokenID, err)

	}

	owner := token.Owner
	if owner == "" {
		return "", fmt.Errorf("No owner is assigned to token %v", TokenID)
	}

	return owner, nil
}

// Burn a non-fungible token, Return whether the burn was successful or not
func (n *NFTContract) Burn(ctx contractapi.TransactionContextInterface, TokenID string) bool {

	owner := ctx.GetClientIdentity().getID()

	TokenIDInt, err_conv := strconv.Atoi(TokenID)
	if err_conv != nil {
		return fmt.Errorf("tokenID  %v is invalid. tokenId must be an integer .%v", TokenID, err)
	}

	// Check if a caller is the owner of the non-fungible token
	token, err := ReadNFT(ctx, TokenID)
	if err != nil {
		fmt.Errorf("Cannot get token for %v : %v", TokenID, err)
		return false

	}

	NftOwner := token.Owner
	if NftOwner != owner {
		fmt.Errorf("Non-fungible token %v is not owned by %v", TokenID, owner)
		return false
	}

	// Delete the token
	nftKey := ctx.GetStub().CreateCompositeKey(nftPrefix, []string{TokenID})
	err := ctx.GetStub().DeleteState(nftKey)
	if err != nil {
		fmt.Errorf("failed to delete nft key: %v", err)
		return false
	}

	// Remove a composite key from the balance of the owner
	balanceKey := ctx.GetStub().CreateCompositeKey(balancePrefix, []string{owner, TokenID})
	err = ctx.GetStub().DeleteState(balanceKey)
	if err != nil {
		fmt.Errorf("failed to delete balance key: %v", err)
		return false
	}

	// Emit the Transfer event
	transferEvent := eventtoken{owner, "0x0", TokenIDInt}
	transferEventJSON, err := json.Marshal(transferEvent)
	if err != nil {
		fmt.Errorf("failed to obtain JSON encoding: %v", err)
		return false
	}
	err = ctx.GetStub().SetEvent("Transfer", transferEventJSON)
	if err != nil {
		fmt.Errorf("failed to set event: %v", err)
		return false
	}
	return true

}

//BalanceOf counts all non-fungible tokens assigned to an owner
func (n *NFTContract) BalanceOf(ctx contractapi.TransactionContextInterface, Owner string) int {

	// There is a key record for every non-fungible token in the format of balancePrefix.Owner.tokenId.
	// BalanceOf() queries for and counts all records matching balancePrefix.Owner.*
	iterator, err := ctx.GetStub().GetStateByPartialCompositeKey(balancePrefix, []string{Owner})

	if err != nil {
		fmt.Printf("Error while getting state :%v", err)
	}
	// Count the number of returned composite keys
	balance := 0
	result := iterator.next()
	for !result.done {
		balance++
		result = iterator.next()
	}
	return balance

}

//ClientAccountBalance returns the balance of the requesting client's account.
func (n *NFTContract) ClientAccountBalance(ctx contractapi.TransactionContextInterface) int {
	// Get ID of submitting client identity
	clientAccountID := ctx.GetClientIdentity().getID()
	return n.BalanceOf(ctx, clientAccountID)
}

//ClientAccountID returns the id of the requesting client's account.
// In this implementation, the client account ID is the clientId itself.
// Users can use this function to get their own account id, which they can then give to others as the payment address
func (n *NFTContract) ClientAccountID(ctx contractapi.TransactionContextInterface) string {

	// Get ID of submitting client identity
	clientAccountID := ctx.GetClientIdentity().getID()
	return clientAccountID
}
