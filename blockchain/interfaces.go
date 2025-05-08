package blockchain

type ChainLikeFiles interface {
	GetFirstTxi(tx uint32) uint64
	GetFirstTxo(tx uint32) uint64
	GetTxiTx(txi uint64) uint32
	GetTxiVout(txi uint64) uint32
	GetTxoValue(txo uint64) uint64
	GetRed(tx uint32) byte
	GetGreen(tx uint32) byte
	GetBlue(tx uint32) byte
	GetOctarine(tx uint32) byte
	GetHashMSBs(tx uint32) []uint32
}
