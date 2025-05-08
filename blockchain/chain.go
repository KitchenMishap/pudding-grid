package blockchain

// This file is related but different to the blockchain/chain.go in pudding-server

import (
	"github.com/KitchenMishap/pudding-shed/chainreadinterface"
	"github.com/KitchenMishap/pudding-shed/chainstorage"
)

type ChainReader struct {
	folder        string
	chainRead     chainreadinterface.IBlockChain
	handleCreator chainreadinterface.IHandleCreator
	parents       chainStorage.IParents
}

func NewChainReader(folder string) (*ChainReader, error) {
	reader := ChainReader{}
	reader.folder = folder
	creator, err := chainstorage.NewConcreteAppendableChainCreator(
		folder,
		[]string{"time", "mediantime", "difficulty", "strippedsize", "size", "weight"},
		[]string{"size", "vsize", "weight"},
		true)
	if err != nil {
		return nil, err
	}
	readableChain, handleCreator, parents, _, _ := creator.OpenReadOnly()
	reader.chainRead = readableChain
	reader.handleCreator = handleCreator
	reader.parents = parents
	return &reader, nil
}
