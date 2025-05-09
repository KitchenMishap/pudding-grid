package blockchain

// This file is related but different to the blockchain/chain.go in pudding-server

import (
	"encoding/binary"
	"github.com/KitchenMishap/pudding-server/derived"
	"github.com/KitchenMishap/pudding-shed/chainreadinterface"
	"github.com/KitchenMishap/pudding-shed/chainstorage"
)

type ChainReader struct {
	folder        string
	chainRead     chainreadinterface.IBlockChain
	handleCreator chainreadinterface.IHandleCreator
	parents       chainstorage.IParents
	derived       *derived.DerivedFiles
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
	derived, err := derived.NewDerivedFiles(folder)
	if err != nil {
		return nil, err
	}
	reader.derived = derived
	err = reader.derived.OpenReadOnly()
	if err != nil {
		return nil, err
	}
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

func (cr ChainReader) GetAddressHashMSBs(handle chainreadinterface.IAddressHandle) uint32 {
	addr, err := cr.Blockchain().AddressInterface(handle)
	if err != nil {
		panic(err)
	}
	hash := addr.Hash()
	var intBytes [4]byte
	// LittleEndian, and we want MSBs, so read from last 4 bytes
	intBytes[0] = hash[28]
	intBytes[1] = hash[29]
	intBytes[2] = hash[30]
	intBytes[3] = hash[31]
	return binary.LittleEndian.Uint32(intBytes[0:4])
}

func (cr ChainReader) GetTxoSpentTxi(handle chainreadinterface.ITxoHandle) chainreadinterface.ITxiHandle {
	if !handle.TxoHeightSpecified() {
		panic("txo height not specified in handle")
	}
	txiHeight, unspent, err := cr.derived.GetTxoSpentTxi(handle.TxoHeight())
	if err != nil {
		panic(err)
	}
	if unspent {
		return nil
	}
	txiHandle, err := cr.handleCreator.TxiHandleByHeight(txiHeight)
	if err != nil {
		panic(err)
	}
	return txiHandle
}
