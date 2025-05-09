package blockchain

// This file is related but different to the blockchain/chain.go in pudding-server

import (
	"encoding/binary"
	"github.com/KitchenMishap/pudding-shed/chainreadinterface"
	"github.com/KitchenMishap/pudding-shed/chainstorage"
)

type ChainReader struct {
	folder        string
	chainRead     chainreadinterface.IBlockChain
	handleCreator chainreadinterface.IHandleCreator
	parents       chainstorage.IParents
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

func (cr ChainReader) Blockchain() chainreadinterface.IBlockChain {
	return cr.chainRead
}
func (cr ChainReader) HandleCreator() chainreadinterface.IHandleCreator {
	return cr.handleCreator
}
func (cr ChainReader) Parents() chainstorage.IParents {
	return cr.parents
}

func (cr ChainReader) GetHashMSBs(handle chainreadinterface.ITransHandle) uint32 {
	trans, err := cr.Blockchain().TransInterface(handle)
	if err != nil {
		panic(err)
	}
	//if trans.HashSpecified() {
	//	panic(errors.New("transaction hash not available"))
	//}
	hash, err := trans.Hash()
	if err != nil {
		panic(err)
	}
	var intBytes [4]byte
	// LittleEndian, and we want MSBs, so read from last 4 bytes
	intBytes[0] = hash[28]
	intBytes[1] = hash[29]
	intBytes[2] = hash[30]
	intBytes[3] = hash[31]
	return binary.LittleEndian.Uint32(intBytes[0:4])
}
