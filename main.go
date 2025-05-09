package main

import (
	"fmt"
	"github.com/KitchenMishap/pudding-grid/blockchain"
)

const TransactionIndex = 62417 // Transaction index to draw" (Pizza transaction)
const Folder = "E:\\Data\\FleeSwallowImmune888888CswHashesDeleted"

func main() {
	chain, err := blockchain.NewChainReader(Folder)
	if err != nil {
		panic(err)
	}

	transHandle, err := chain.HandleCreator().TransactionHandleByHeight(TransactionIndex)
	if err != nil {
		panic(err)
	}

	fmt.Println("Starting...")
	err = createTransactionImage(transHandle, chain)
	if err != nil {
		panic(err)
	}

	fmt.Println("Finished")
}
