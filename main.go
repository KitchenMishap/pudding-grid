package main

import (
	"fmt"
	"pudding-grid/blockchain"
)

const TransactionIndex = 1867248 // Transaction index to draw" (Pizza transaction)
const Folder = "E:\\Data\\FleeSwallowImmune888888CswHashesDeleted"

func main() {
	chain, err := blockchain.NewChainReader(Folder)
	if err != nil {
		panic(err)
	}

	fmt.Println("Starting...")
	err = createTransactionImage(uint32(TransactionIndex), chain)
	if err != nil {
		panic(err)
	}

	fmt.Println("Finished")
}
