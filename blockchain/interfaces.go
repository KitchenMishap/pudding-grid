package blockchain

import (
	"github.com/KitchenMishap/pudding-shed/chainreadinterface"
	"github.com/KitchenMishap/pudding-shed/chainstorage"
)

type AccessChain interface {
	Blockchain() chainreadinterface.IBlockChain
	HandleCreator() chainreadinterface.IHandleCreator
	Parents() chainstorage.IParents
	GetHashMSBs(handle chainreadinterface.ITransHandle) uint32
}
