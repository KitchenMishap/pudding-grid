package main

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"github.com/KitchenMishap/pudding-grid/blockchain"
	"github.com/KitchenMishap/pudding-shed/chainreadinterface"
	"math"
	"os"
	"sort"
	"strconv"
	"sync"
)

type floatCoords = float32 // A float type to use for co-ordinates
type floatColour = float32 // A float type to use for colour components

// Represents the operation of constructing a pixel image from a transaction
type transactionPixels struct {
	rootTransactionIndex int64
	// Pixels
	width  int
	height int
	// The transaction is treated as the unit square
	// The pixel grid maps to the following transaction rectangle relative to the unit square
	leftX   floatCoords
	rightX  floatCoords
	topY    floatCoords
	bottomY floatCoords
	// Each pixel holds dynamic information as its colour is accumulated
	// Each is nil once completely dealt with
	partialPixels [][]*transPixel
	// Once completed, each pixel has 4 bytes of colour components (red, green, blue, octarine)
	completedPixels [][][]byte
	// Pixels may only be accessed via the mutex
	pixelMutexes [][]sync.Mutex
	// How much of a pixel needs to be known before we consider it done
	doneThreshold floatCoords
	// How deep do we want to go?
	maxDepth int
	// minDepth overrides areaThreshold
	minDepth int
	// Discard tiny tiny contributions to pixels
	minContribution floatCoords
	// How deep (area) do we want to go?
	areaThreshold floatCoords
	// How many contributions to a pixel before we collapse them?
	maxContributions int
	// How many pixels remain incomplete
	incompletePixels int
	// How many goroutines are allowed
	maxGoroutines int
	// How many goroutines we still have free
	goroutines int

	// The data in the old arrays are now provided by the following object
	chain blockchain.AccessChain
}

// Represents the data gathered regarding a pixel before a pixel is completely known
type transPixel struct {
	redAccumulations      []floatColour
	greenAccumulations    []floatColour
	blueAccumulations     []floatColour
	octarineAccumulations []floatColour
	areaAccumulations     []floatCoords
	areaEstimate          floatCoords
}

func createTransactionImage(transHandle chainreadinterface.ITransHandle, chain blockchain.AccessChain) error {
	if !transHandle.HeightSpecified() {
		return errors.New("handle doesn't specify height")
	}
	transHeight := transHandle.Height()
	transaction, err := chain.Blockchain().TransInterface(transHandle)
	if err != nil {
		return err
	}
	//for zoom := 1; zoom <= 1048576*1048576; zoom *= 4 {
	for zoom := 1; zoom <= 1; zoom *= 4 {
		tp := NewTransactionPixels(transHeight, chain, floatCoords(zoom))
		tp.drawTransactionPixels(transaction)
		err := tp.outputGraphicsFile(zoom)
		if err != nil {
			return err
		}
	}

	return nil
}

func NewTransactionPixels(rootTransactionIndex int64, chain blockchain.AccessChain, zoom floatCoords) *transactionPixels {
	tp := new(transactionPixels)
	tp.rootTransactionIndex = rootTransactionIndex
	tp.width = 1000
	tp.height = 1000
	//tp.width = 1920 // Full HD
	//tp.height = 1080
	//tp.width = 3440	// My monitor
	//tp.height = 1440
	//tp.width = 4950 // A3 300 DPI
	//tp.height = 3510
	//tp.width = 5905 // 50cm x 50cm 300 dpi
	//tp.height = 5905
	//tp.width = 2882 // Marinas block
	//tp.height = 2882
	//tp.width = 2953 // Marinas shelf
	//tp.height = 2953
	//tp.width = 5918 // Nigels first BTC
	//tp.height = 5918
	//tp.width = 5727 // Nigels second BTC
	//tp.height = 5727
	//tp.width = 9374 // Nigels third half BTC
	//tp.height = 9374
	//tp.width = 8072 // Nigels fourth pair BTC
	//tp.height = 8072
	//tp.width = 7141 // Nigels fifth BTC
	//tp.height = 7141
	tp.leftX = 0.5 - 0.5/zoom
	tp.rightX = 0.5 + 0.5/zoom
	tp.topY = 0.5 - 0.5/zoom
	tp.bottomY = 0.5 + 0.5/zoom
	tp.incompletePixels = tp.width * tp.height
	// 2D slice of slices
	tp.partialPixels = make([][]*transPixel, tp.height)
	tp.pixelMutexes = make([][]sync.Mutex, tp.height)
	for y := 0; y < tp.height; y++ {
		tp.partialPixels[y] = make([]*transPixel, tp.width)
		tp.pixelMutexes[y] = make([]sync.Mutex, tp.width)
		for x := 0; x < tp.width; x++ {
			tp.partialPixels[y][x] = new(transPixel)
			tp.partialPixels[y][x].areaEstimate = 0
			tp.partialPixels[y][x].areaAccumulations = nil
			tp.partialPixels[y][x].redAccumulations = nil
			tp.partialPixels[y][x].greenAccumulations = nil
			tp.partialPixels[y][x].blueAccumulations = nil
			tp.partialPixels[y][x].octarineAccumulations = nil
		}
	}
	// 2D slice of slices of [4]array
	tp.completedPixels = make([][][]byte, tp.height)
	for y := 0; y < tp.height; y++ {
		tp.completedPixels[y] = make([][]byte, tp.width)
		for x := 0; x < tp.width; x++ {
			// 4 components per pixel: red, green, blue, octarine
			tp.completedPixels[y][x] = make([]byte, 4)
			// Start pixel off as blue, to highlight if pixels fail to be painted
			tp.completedPixels[y][x][0] = 0
			tp.completedPixels[y][x][1] = 0
			tp.completedPixels[y][x][2] = 255
			tp.completedPixels[y][x][3] = 0
		}
	}
	tp.doneThreshold = 254.0 / 255.0
	tp.maxDepth = 760000 // Silly but let's see!
	tp.minDepth = 1      // minDepth overrides areaThreshold
	tp.minContribution = 0.0
	tp.areaThreshold = 0.5
	tp.maxContributions = 30
	tp.maxGoroutines = 10
	tp.goroutines = tp.maxGoroutines

	// An interface providing values a bit like the arrays we had before
	tp.chain = chain

	return tp
}

func (tp *transactionPixels) requestGoroutine() bool {
	if tp.goroutines <= 0 {
		return false
	}
	tp.goroutines -= 1
	return true
}

func (tp *transactionPixels) releaseGoroutines(n int) {
	tp.goroutines += n
}

// Returns true if completed all pixels
func (tp *transactionPixels) setPixelCompleted(x int, y int, r byte, g byte, b byte, o byte) bool {
	if tp.partialPixels[y][x] == nil {
		// Already completed this pixel
		return false
	}
	tp.completedPixels[y][x][0] = r
	tp.completedPixels[y][x][1] = g
	tp.completedPixels[y][x][2] = b
	tp.completedPixels[y][x][3] = o
	tp.partialPixels[y][x] = nil
	tp.incompletePixels--
	if tp.incompletePixels%10000 == 0 {
		percentPoints := int(100 * (tp.width*tp.height - tp.incompletePixels) / (tp.width * tp.height))
		println(tp.incompletePixels, " ", percentPoints, " %")
	}
	if tp.incompletePixels == 0 {
		println("Last pixel done")
		return true
	}
	return false
}

// Returns true if completed all pixels
func (tp *transactionPixels) contributeColour(x int, y int, proportion floatCoords, r byte, g byte, b byte, o byte) bool {
	if x >= tp.width || y >= tp.height {
		panic("Pixel co-ords out of range")
	}
	if proportion == 0 {
		return false
	}
	if proportion <= 0 {
		panic("Non positive proportion")
	}
	if proportion < tp.minContribution {
		return false
	}
	if tp.partialPixels[y][x] == nil {
		// Already completed pixel
		return false
	}
	tp.pixelMutexes[y][x].Lock()
	defer tp.pixelMutexes[y][x].Unlock()

	if proportion > tp.doneThreshold {
		return tp.setPixelCompleted(x, y, byte(r), byte(g), byte(b), byte(o))
	}

	tp.partialPixels[y][x].areaEstimate += proportion
	tp.partialPixels[y][x].areaAccumulations = append(tp.partialPixels[y][x].areaAccumulations, proportion)
	tp.partialPixels[y][x].redAccumulations = append(tp.partialPixels[y][x].redAccumulations, floatColour(float64(r)*float64(proportion)))
	tp.partialPixels[y][x].greenAccumulations = append(tp.partialPixels[y][x].greenAccumulations, floatColour(float64(g)*float64(proportion)))
	tp.partialPixels[y][x].blueAccumulations = append(tp.partialPixels[y][x].blueAccumulations, floatColour(float64(b)*float64(proportion)))
	tp.partialPixels[y][x].octarineAccumulations = append(tp.partialPixels[y][x].octarineAccumulations, floatColour(float64(o)*float64(proportion)))
	// Are we near enough complete for this pixel?
	if tp.partialPixels[y][x].areaEstimate >= tp.doneThreshold {
		rr, gg, bb, oo, aa := tp.summmarizePixel(x, y)
		return tp.setPixelCompleted(x, y, byte(float64(rr)/float64(aa)), byte(float64(gg)/float64(aa)), byte(float64(bb)/float64(aa)), byte(float64(oo)/float64(aa)))
	} else if len(tp.partialPixels[y][x].redAccumulations) == tp.maxContributions {
		// Or are we so clogged up we wish to summarize?
		rr, gg, bb, oo, aa := tp.summmarizePixel(x, y)
		tp.partialPixels[y][x].redAccumulations = []floatColour{rr}
		tp.partialPixels[y][x].greenAccumulations = []floatColour{gg}
		tp.partialPixels[y][x].blueAccumulations = []floatColour{bb}
		tp.partialPixels[y][x].octarineAccumulations = []floatColour{oo}
		tp.partialPixels[y][x].areaAccumulations = []floatCoords{aa}
		return false
	} else {
		return false
	}
}

func (tp *transactionPixels) replaceColour(x int, y int, r byte, g byte, b byte, o byte) {
	if tp.partialPixels[y][x] == nil {
		// Pixel is completed, amend it there
		tp.completedPixels[y][x][0] = r
		tp.completedPixels[y][x][1] = g
		tp.completedPixels[y][x][2] = b
		tp.completedPixels[y][x][3] = o
	} else {
		_, _, _, _, aa := tp.summmarizePixel(x, y)
		tp.partialPixels[y][x].redAccumulations = []floatColour{floatColour(r)}
		tp.partialPixels[y][x].greenAccumulations = []floatColour{floatColour(g)}
		tp.partialPixels[y][x].blueAccumulations = []floatColour{floatColour(b)}
		tp.partialPixels[y][x].octarineAccumulations = []floatColour{floatColour(o)}
		tp.partialPixels[y][x].areaAccumulations = []floatCoords{aa}
		if tp.partialPixels[y][x].areaEstimate >= tp.doneThreshold {
			rr, gg, bb, oo, aa := tp.summmarizePixel(x, y)
			tp.setPixelCompleted(x, y, byte(float64(rr)/float64(aa)), byte(float64(gg)/float64(aa)), byte(float64(bb)/float64(aa)), byte(float64(oo)/float64(aa)))
		}
	}
}

func (tp *transactionPixels) incompletePixelsReport() {
	totalCompleteness := floatCoords(0)
	countIncomplete := uint64(0)
	for y := 0; y < tp.height; y++ {
		for x := 0; x < tp.width; x++ {
			if tp.partialPixels[y][x] != nil {
				countIncomplete++
				_, _, _, _, a := tp.summmarizePixel(x, y)
				totalCompleteness += a
			}
		}
	}
	println("There are ", countIncomplete, " incomplete pixels.")
	if countIncomplete != 0 {
		percentage := int(100*totalCompleteness) / int(countIncomplete)
		println("of which... Average completeness ", percentage, "%")
	}
}

func (tp *transactionPixels) summmarizePixel(x int, y int) (r floatColour, g floatColour, b floatColour, o floatColour, a floatCoords) {
	// The arrays are all the same size
	length := len(tp.partialPixels[y][x].redAccumulations)
	// Convert to float64 array so we can use sort.Float64s()
	accR := make([]float64, length)
	accG := make([]float64, length)
	accB := make([]float64, length)
	accO := make([]float64, length)
	accA := make([]float64, length)
	for i := 0; i < length; i++ {
		accR[i] = float64(tp.partialPixels[y][x].redAccumulations[i])
		accG[i] = float64(tp.partialPixels[y][x].greenAccumulations[i])
		accB[i] = float64(tp.partialPixels[y][x].blueAccumulations[i])
		accO[i] = float64(tp.partialPixels[y][x].octarineAccumulations[i])
		accA[i] = float64(tp.partialPixels[y][x].areaAccumulations[i])
	}
	// For accuracy, we sort low to high before summing
	sort.Float64s(accR)
	sort.Float64s(accG)
	sort.Float64s(accB)
	sort.Float64s(accO)
	sort.Float64s(accA)
	r = floatColour(0)
	g = floatColour(0)
	b = floatColour(0)
	o = floatColour(0)
	a = floatCoords(0)
	for i := 0; i < length; i++ {
		r += floatColour(accR[i])
		g += floatColour(accG[i])
		b += floatColour(accB[i])
		o += floatColour(accO[i])
		a += floatCoords(accA[i])
	}
	return r, g, b, o, a
}

func overlapArea(left1 floatCoords, right1 floatCoords, top1 floatCoords, bottom1 floatCoords,
	left2 floatCoords, right2 floatCoords, top2 floatCoords, bottom2 floatCoords) floatCoords {

	// Avoid the 'flipping inside out' problem of the clipping scheme below
	if left1 >= right2 {
		return 0
	}
	if right1 <= left2 {
		return 0
	}
	if top1 >= bottom2 {
		return 0
	}
	if bottom1 <= top2 {
		return 0
	}

	// Clip '1' to be within '2'
	l1 := math.Max(float64(left1), float64(left2))
	r1 := math.Min(float64(right1), float64(right2))
	t1 := math.Max(float64(top1), float64(top2))
	b1 := math.Min(float64(bottom1), float64(bottom2))
	// Calculate the new area of '1'
	if l1 > r1 {
		panic("Oops")
	}
	if t1 > b1 {
		panic("Oops")
	}
	return floatCoords((r1 - l1) * (b1 - t1))
}

func (tp *transactionPixels) pixelToTransactionCoords(x int, y int) (left floatCoords, right floatCoords, top floatCoords, bottom floatCoords) {
	xScale := (tp.rightX - tp.leftX) / floatCoords(tp.width)
	yScale := (tp.bottomY - tp.topY) / floatCoords(tp.height)
	left = tp.leftX + floatCoords(x)*xScale
	right = left + xScale
	top = tp.topY + floatCoords(y)*yScale
	bottom = top + yScale
	return left, right, top, bottom
}

func (tp *transactionPixels) transactionToPixelCoords(x floatCoords, y floatCoords) (px floatCoords, py floatCoords) {
	xScale := (tp.rightX - tp.leftX) / floatCoords(tp.width)
	yScale := (tp.bottomY - tp.topY) / floatCoords(tp.height)
	px = (x - tp.leftX) / xScale
	py = (y - tp.topY) / yScale
	return px, py
}

func (tp *transactionPixels) drawTransactionPixels(transaction chainreadinterface.ITransaction) {
	println("Starting")
	finished := false
	// First mark pixels outside transaction as black
	// We'll need to know how much each pixel overlaps the transaction
	// The transaction covers the unit square, so convert this to pixel co-ords
	pLeft, pTop := tp.transactionToPixelCoords(0.0, 0.0)
	pRight, pBottom := tp.transactionToPixelCoords(1.0, 1.0)
	for y := 0; y < tp.height; y++ {
		for x := 0; x < tp.width; x++ {
			overlap := overlapArea(floatCoords(x), floatCoords(x+1), floatCoords(y), floatCoords(y+1),
				pLeft, pRight, pTop, pBottom)
			blackOverlap := 1.0 - overlap
			if blackOverlap >= tp.doneThreshold {
				// 100% black
				finished = finished || tp.contributeColour(x, y, 1.0, 0, 0, 0, 0)
			} else if blackOverlap > 0 {
				// n% black (incomplete colour, will find the rest later)
				finished = finished || tp.contributeColour(x, y, blackOverlap, 0, 0, 0, 0)
			}
		}
	}
	if finished {
		println("Finished just with the black outer")
		return
	}

	// Second, delegate the transaction to the recursive function to
	// find the colours within the transaction
	finished = finished || tp.drawTransactionRecurse(transaction, 0, false, 0.0, 1.0, 0.0, 1.0, 0, true, true, true)
	if finished {
		println("Properly finished")
	} else {
		println("Finished but not finished!")
		tp.incompletePixelsReport()
	}
}

// Returns true if finished overall
// The names X,Y are used for "image co-ords" which go from 0 to 1 across the whole image
// The names U,V are used for "sense co-ords", and either equal (X,Y) or (Y,X). U spans across transaction banding profile, V spans across transaction inputs.
func (tp *transactionPixels) drawTransactionRecurse(transaction chainreadinterface.ITransaction, addressHashMSBs uint32, useUnitSquare bool, leftX floatCoords, rightX floatCoords, topY floatCoords, bottomY floatCoords, depth int, insistOneLastDepth bool, isFirstTxi bool, isLastTxi bool) bool {
	// transaction can be nil if we are a UTXO and we have reversed time
	const REVERSE_TIME = false
	useUnitSquare = false
	// splitter := SplitRectSlabs{}
	splitter := SquarifySplitter{}

	finished := false
	// Gather / calculate some things...

	var hashMSBs uint32 // Either the (MSBs of the) hash of the transaction, or the hash of a utxo's address

	// Is T a block reward? (no txis)
	var isBlockReward bool
	if !REVERSE_TIME {
		txiCount, err := transaction.TxiCount()
		if err != nil {
			panic(err)
		}
		isBlockReward = (txiCount == 0)
		hashMSBs = tp.chain.GetHashMSBs(transaction)
	} else {
		// Is T a UTXO?
		isBlockReward = (transaction == nil)
		if isBlockReward {
			hashMSBs = addressHashMSBs // Use the hash of the txo's address instead
		}
	}
	// The average colour of the transaction before we recurse to higher detail and before we apply the taint.
	// We no longer generate a transaction colour database, as it quickly mushes to grey anyway.
	// Nowadays, to save time, the colour is taken from the transaction hash as before, but if (as in most cases)
	// the transaction has inputs, its assumed to be "mixed" into grey immediately, without true mixing
	r := byte((hashMSBs & 0xFF000000) >> 24)
	g := byte((hashMSBs & 0x00FF0000) >> 16)
	b := byte((hashMSBs & 0x0000FF00) >> 8)
	o := byte(hashMSBs & 0x000000FF)
	if !isBlockReward {
		r = 128
		g = 128
		b = 128
		o = 128
	}

	// We need to get the intersection of the image and T
	// We check whether there IS any intersection initially
	if tp.leftX >= rightX {
		return false // Image is wholly to right of transaction
	}
	if tp.rightX <= leftX {
		return false // image is wholly to left of transaction
	}
	if tp.topY >= bottomY {
		return false // image is wholly below transaction
	}
	if tp.bottomY <= topY {
		return false // image is wholly above transaction
	}
	// Start by presuming the intersection IS the image, then clip it to the transaction
	interLeftX := tp.leftX
	interRightX := tp.rightX
	interTopY := tp.topY
	interBottomY := tp.bottomY
	if interLeftX < leftX {
		interLeftX = leftX
	}
	if interRightX > rightX {
		interRightX = rightX
	}
	if interTopY < topY {
		interTopY = topY
	}
	if interBottomY > bottomY {
		interBottomY = bottomY
	}
	// Now we convert from transaction co-ordinates to pixel co-ordinates (as floats)
	pixLeft, pixTop := tp.transactionToPixelCoords(interLeftX, interTopY)
	pixRight, pixBottom := tp.transactionToPixelCoords(interRightX, interBottomY)
	// If we have a zero area rectangle we can't expect progress!
	if pixLeft == pixRight || pixTop == pixBottom {
		return false
	}
	// We now use rounding to gather the range of pixels affected (both partially and wholly)
	pixLeftAffected := int(math.Floor(float64(pixLeft)))
	pixTopAffected := int(math.Floor(float64(pixTop)))
	pixRightAffected := int(math.Floor(float64(pixRight)))
	pixBottomAffected := int(math.Floor(float64(pixBottom)))

	// The following vals will help us decide whether to go deeper
	width := pixRight - pixLeft
	height := pixBottom - pixTop
	letsGoDeeper := width*height > tp.areaThreshold || depth < tp.minDepth
	oneLastDepthWorthwhile := false

	var goDeeper bool
	if isBlockReward {
		goDeeper = false // There is no deeper to go!
	} else if depth == tp.maxDepth {
		goDeeper = false // Hard limit reached
	} else if insistOneLastDepth {
		goDeeper = true                // Our caller insists!
		oneLastDepthWorthwhile = false // But don't let it go further
	} else {
		goDeeper = letsGoDeeper // We think here its a good idea
	}
	// (We will pass on oneLastDepthWorthwhile to the recursion below, as the parameter insistOneLastDepth)

	if !goDeeper {
		// We mark pixels as either totally or partially affected by the colour.

		// The top line of pixels, apart from corners, is (potentially) partially affected by the same amount,
		// so calculate the precise overlap percentage
		overlap := floatCoords(pixTopAffected+1) - pixTop
		// Contribute to the top row (not including corners) by percentage overlap using colour r,g,b,o
		if overlap > 0 {
			for x := pixLeftAffected + 1; x <= pixRightAffected-1; x++ {
				finished = finished || tp.contributeColour(x, pixTopAffected, overlap, r, g, b, o)
			}
		}
		// Similarly for bottom row (not including corners)
		overlap = pixBottom - floatCoords(pixBottomAffected)
		if overlap > 0 {
			for x := pixLeftAffected + 1; x <= pixRightAffected-1; x++ {
				finished = finished || tp.contributeColour(x, pixBottomAffected, overlap, r, g, b, o)
			}
		}
		// Similarly for left column (not including corners)
		overlap = floatCoords(pixLeftAffected+1) - pixLeft
		if overlap > 0 {
			for y := pixTopAffected + 1; y <= pixBottomAffected-1; y++ {
				finished = finished || tp.contributeColour(pixLeftAffected, y, overlap, r, g, b, o)
			}
		}
		// Similarly for right column (not including corners)
		overlap = pixRight - floatCoords(pixRightAffected)
		if overlap > 0 {
			for y := pixTopAffected + 1; y <= pixBottomAffected-1; y++ {
				finished = finished || tp.contributeColour(pixRightAffected, y, overlap, r, g, b, o)
			}
		}
		// Corners are a faff but are very much needed, because very small rectangles are very common!
		// Of particular faff is that all four corners (or fewer) may be the same pixel :-O so only count once
		leftEqualsRight := (pixLeftAffected == pixRightAffected)
		topEqualsBottom := (pixTopAffected == pixBottomAffected)
		overlapTL := overlapArea(pixLeft, pixRight, pixTop, pixBottom, floatCoords(pixLeftAffected), floatCoords(pixLeftAffected+1), floatCoords(pixTopAffected), floatCoords(pixTopAffected+1))
		overlapTR := overlapArea(pixLeft, pixRight, pixTop, pixBottom, floatCoords(pixRightAffected), floatCoords(pixRightAffected+1), floatCoords(pixTopAffected), floatCoords(pixTopAffected+1))
		overlapBL := overlapArea(pixLeft, pixRight, pixTop, pixBottom, floatCoords(pixLeftAffected), floatCoords(pixLeftAffected+1), floatCoords(pixBottomAffected), floatCoords(pixBottomAffected+1))
		overlapBR := overlapArea(pixLeft, pixRight, pixTop, pixBottom, floatCoords(pixRightAffected), floatCoords(pixRightAffected+1), floatCoords(pixBottomAffected), floatCoords(pixBottomAffected+1))
		// Top left
		if overlapTL > 0 {
			finished = finished || tp.contributeColour(pixLeftAffected, pixTopAffected, overlapTL, r, g, b, o)
		}
		// Maybe top right
		if !leftEqualsRight && overlapTR > 0 {
			finished = finished || tp.contributeColour(pixRightAffected, pixTopAffected, overlapTR, r, g, b, o)
		}
		// Maybe bottom left and / or right
		if !topEqualsBottom {
			if overlapBL > 0 {
				finished = finished || tp.contributeColour(pixLeftAffected, pixBottomAffected, overlapBL, r, g, b, o)
			}
			if !leftEqualsRight && overlapBR > 0 {
				finished = finished || tp.contributeColour(pixRightAffected, pixBottomAffected, overlapBR, r, g, b, o)
			}
		}
		// The totally affected pixels in the middle (if any) are easy...
		for y := pixTopAffected + 1; y <= pixBottomAffected-1; y++ {
			for x := pixLeftAffected + 1; x <= pixRightAffected-1; x++ {
				finished = finished || tp.contributeColour(x, y, 1.0, r, g, b, o)
			}
		}
	} else if pixLeftAffected == pixRightAffected && pixTopAffected == pixBottomAffected {
		// Else if T is totally contained in one pixel
		overlap := overlapArea(pixLeft, pixRight, pixTop, pixBottom, floatCoords(pixLeftAffected), floatCoords(pixLeftAffected+1), floatCoords(pixTopAffected), floatCoords(pixTopAffected+1))
		finished = finished || tp.contributeColour(pixLeftAffected, pixTopAffected, overlap, r, g, b, o)
	} else {
		// Recurse txi's of T
		// First add up the bitcoin values of inputs so we know the ratios
		total := int64(0)
		var sats int64
		if !REVERSE_TIME {
			count, err := transaction.TxiCount()
			if err != nil {
				panic(err)
			}
			for txiInd := int64(0); txiInd < count; txiInd++ {
				txiHandle, err := transaction.NthTxi(txiInd)
				if err != nil {
					panic(err)
				}
				txi, err := tp.chain.Blockchain().TxiInterface(txiHandle)
				if err != nil {
					panic(err)
				}
				// Get the txo corresponding to the txi
				txoHandle, err := txi.SourceTxo()
				txo, err := tp.chain.Blockchain().TxoInterface(txoHandle)
				if err != nil {
					panic(err)
				}
				// Get the bitcoin value of the txo (in satoshis)
				sats, err = txo.Satoshis()
				if err != nil {
					panic(err)
				}
				total += sats
			}
		} else {
			var count int64
			var err error
			if transaction == nil {
				count = 0
			} else {
				count, err = transaction.TxoCount()
				if err != nil {
					panic(err)
				}
				for txoInd := int64(0); txoInd < count; txoInd++ {
					txoHandle, err := transaction.NthTxo(txoInd)
					if err != nil {
						panic(err)
					}
					txo, err := tp.chain.Blockchain().TxoInterface(txoHandle)
					if err != nil {
						panic(err)
					}
					// Get the bitcoin value of the txo (in satoshis)
					sats, err := txo.Satoshis()
					if err != nil {
						panic(err)
					}
					total += sats
				}
			}
		}
		// Now recurse
		satSum := uint64(0) // The satoshis we have encountered so far

		// We want to do the biggest input first, as it is more likely to complete
		// some pixels (and that will mean others might not need to be examined in depth).
		// So gather the info we need about the inputs into arrays before we start with biggest.
		var txs []chainreadinterface.ITransaction
		var addrHashMSBs []uint32
		var sats2 []int64
		var weights []floatCoords // = sats2
		var subRects []rect
		var dones []bool
		var isFirstTxi []bool
		var isLastTxi []bool
		var txxIndex []uint64
		thisRect := rect{left: leftX, right: rightX, top: topY, bottom: bottomY}
		if !REVERSE_TIME {
			count, err := transaction.TxiCount()
			if err != nil {
				panic(err)
			}
			for txiInd := int64(0); txiInd < count; txiInd++ {
				txiHandle, err := transaction.NthTxi(txiInd)
				if err != nil {
					panic(err)
				}
				txi, err := tp.chain.Blockchain().TxiInterface(txiHandle)
				if err != nil {
					panic(err)
				}
				// Get the txo the txi came from
				txoHandle, err := txi.SourceTxo()
				if err != nil {
					panic(err)
				}
				if txoHandle.ParentSpecified() {
					parentTransHandle := txoHandle.ParentTrans()
					parentTrans, err := tp.chain.Blockchain().TransInterface(parentTransHandle)
					if err != nil {
						panic(err)
					}
					txs = append(txs, parentTrans)
				} else if txoHandle.TxoHeightSpecified() {
					parentTransHeight, err := tp.chain.Parents().ParentTransOfTxo(txoHandle.TxoHeight())
					if err != nil {
						panic(err)
					}
					parentTransHandle, err := tp.chain.HandleCreator().TransactionHandleByHeight(parentTransHeight)
					if err != nil {
						panic(err)
					}
					parentTrans, err := tp.chain.Blockchain().TransInterface(parentTransHandle)
					if err != nil {
						panic(err)
					}
					txs = append(txs, parentTrans)
				} else {
					panic("txo has neither parent transaction nor txo height specified")
				}
				txo, err := tp.chain.Blockchain().TxoInterface(txoHandle)
				if err != nil {
					panic(err)
				}
				// Get the bitcoin value of the txo (in satoshis)
				sat, err := txo.Satoshis()
				if err != nil {
					panic(err)
				}
				sats2 = append(sats2, sat)
				weights = append(weights, floatCoords(sat))
				satSum += uint64(sat)
				dones = append(dones, false)
				isFirstTxi = append(isFirstTxi, txiInd == 0)
				isLastTxi = append(isLastTxi, txiInd == count-1)
				txxIndex = append(txxIndex, uint64(txiInd))
				addr, err := txo.Address()
				if err != nil {
					panic(err)
				}
				addrHashMSBs = append(addrHashMSBs, tp.chain.GetAddressHashMSBs(addr))
			}
			subRects = splitter.SplitRectIntoWeightedRects(thisRect, weights, depth, useUnitSquare)
		} else {
			count, err := transaction.TxoCount()
			if err != nil {
				panic(err)
			}
			for txoInd := int64(0); txoInd < count; txoInd++ {
				txoHandle, err := transaction.NthTxo(txoInd)
				if err != nil {
					panic(err)
				}
				txo, err := tp.chain.Blockchain().TxoInterface(txoHandle)
				if err != nil {
					panic(err)
				}
				// Get the bitcoin value of the txo (in satoshis)
				sat, err := txo.Satoshis()
				if err != nil {
					panic(err)
				}
				sats2 = append(sats2, sat)
				weights = append(weights, floatCoords(sat))
				satSum += uint64(sat)
				dones = append(dones, false)
				isFirstTxi = append(isFirstTxi, txoInd == 0)
				isLastTxi = append(isLastTxi, txoInd == count-1)
				txxIndex = append(txxIndex, uint64(txoInd))
				addr, err := txo.Address()
				if err != nil {
					panic(err)
				}
				addrHashMSBs = append(addrHashMSBs, tp.chain.GetAddressHashMSBs(addr))
				// Get the transaction the output goes to
				txiHandle := tp.chain.GetTxoSpentTxi(txoHandle)
				if !txiHandle.TxiHeightSpecified() {
					panic("txi height not specified")
				}
				txiHeight := txiHandle.TxiHeight()
				parentTransHeight, err := tp.chain.Parents().ParentTransOfTxi(txiHeight)
				if err != nil {
					panic(err)
				}
				parentTransHandle, err := tp.chain.HandleCreator().TransactionHandleByHeight(parentTransHeight)
				if err != nil {
					panic(err)
				}
				parentTrans, err := tp.chain.Blockchain().TransInterface(parentTransHandle)
				if err != nil {
					panic(err)
				}
				txs = append(txs, parentTrans)
			}
			subRects = splitter.SplitRectIntoWeightedRects(thisRect, weights, depth, useUnitSquare)
		}

		// Decide whether there's a mixture of orientations at this level
		horizontal := int(0)
		vertical := int(0)
		for _, rectCopy := range subRects {
			if rectCopy.right-rectCopy.left > rectCopy.bottom-rectCopy.top {
				horizontal++
			} else {
				vertical++
			}
		}
		//allSameWayUp := (horizontal*vertical == 0)

		// Now recurse, biggest first, until all dones are true
		// Here is where we start thinking about multithreading using goroutines
		var wg sync.WaitGroup
		routines := 0

		for {
			biggestSats := int64(0)
			biggestIndex := -1
			// Find the i with the biggest sats[i]
			for i := 0; i < len(txs); i++ {
				if !dones[i] && sats2[i] > biggestSats {
					biggestSats = sats2[i]
					biggestIndex = i
				}
			}

			// Do the i with the biggest sats[i]
			if biggestSats > 0 {
				if tp.requestGoroutine() {
					// Recurse as new goroutine
					wg.Add(1)
					routines++
					// Pay attention here! Running an anonymous function as a goroutine
					go func(_tx chainreadinterface.ITransaction, _msbs uint32, _rect rect, _isFirst bool, _isLast bool, txxIndex uint64) {
						defer wg.Done()
						tp.drawTransactionRecurse(_tx, _msbs, useUnitSquare, _rect.left, _rect.right, _rect.top, _rect.bottom, depth+1, oneLastDepthWorthwhile, _isFirst, _isLast)
					}(txs[biggestIndex], addrHashMSBs[biggestIndex], subRects[biggestIndex], isFirstTxi[biggestIndex], isLastTxi[biggestIndex], txxIndex[biggestIndex])
				} else {
					// Recurse as standard in the current goroutine
					// This could be because we are running in a single-threaded mode
					// But COULD be multithreaded; we've just run out of goroutines
					finished = finished || tp.drawTransactionRecurse(txs[biggestIndex], addrHashMSBs[biggestIndex], useUnitSquare, subRects[biggestIndex].left, subRects[biggestIndex].right, subRects[biggestIndex].top, subRects[biggestIndex].bottom, depth+1, oneLastDepthWorthwhile, isFirstTxi[biggestIndex], isLastTxi[biggestIndex])

					if finished {
						return true
					}
				}
				dones[biggestIndex] = true
			} else {
				// We've now done all the txi's
				break
			}
		}
		// Wait for all the goroutines we kicked off to finish
		wg.Wait()
		tp.releaseGoroutines(routines)
	}

	// Work out the taint colour that the tx itself paints over its children as transaction banding
	transR := byte((hashMSBs & 0xFF000000) >> 24)
	transG := byte((hashMSBs & 0x00FF0000) >> 16)
	transB := byte((hashMSBs & 0x0000FF00) >> 8)
	transO := byte((hashMSBs & 0x000000FF))

	const HUE_AS_DATE = false
	const STRIPE_COLOUR_IS_HASH = !HUE_AS_DATE
	if HUE_AS_DATE {
		transH, transS, transV := rgb_to_hsv(transR, transG, transB)
		startH := transH // Deterministically choose a start hue, so not all pics same colour
		var eraStart, eraEnd int64
		if REVERSE_TIME {
			eraStart = tp.rootTransactionIndex
			eraEnd = 888888 // ToDo get this from somewhere
		} else {
			eraStart = 0
			eraEnd = tp.rootTransactionIndex
		}
		if !transaction.HeightSpecified() {
			panic(errors.New("transaction height not specified"))
		}
		transH = float64(transaction.Height()-eraStart) / float64(eraEnd-eraStart)
		transH += startH             // Rotate hue by start hue
		transH -= math.Floor(transH) // Keep between 0 and 1
		transR, transG, transB = hsv2rgb(byte(255*transH), byte(255*transS), byte(255*transV))
	}

	// Next we "paint over" the transaction banding (a new form of tainting)...
	// Much of this code is borrowed from above

	// The top line of pixels, apart from corners, is (potentially) partially affected by the same amount,
	// so calculate the precise overlap percentage
	overlap := floatCoords(pixTopAffected+1) - pixTop
	// Wash or banding paint to the top row (not including corners) by percentage overlap
	if overlap > 0 {
		for x := pixLeftAffected + 1; x <= pixRightAffected-1; x++ {
			tp.ApplyTransactionBandingToPixel(x, pixTopAffected,
				pixLeft, pixRight, pixTop, pixBottom, overlap,
				transR, transG, transB, transO, isBlockReward, depth,
				isFirstTxi, isLastTxi)
		}
	}
	// Similarly for bottom row (not including corners)
	overlap = pixBottom - floatCoords(pixBottomAffected)
	if overlap > 0 {
		for x := pixLeftAffected + 1; x <= pixRightAffected-1; x++ {
			tp.ApplyTransactionBandingToPixel(x, pixBottomAffected,
				pixLeft, pixRight, pixTop, pixBottom, overlap,
				transR, transG, transB, transO, isBlockReward, depth,
				isFirstTxi, isLastTxi)
		}
	}
	// Similarly for left column (not including corners)
	overlap = floatCoords(pixLeftAffected+1) - pixLeft
	if overlap > 0 {
		for y := pixTopAffected + 1; y <= pixBottomAffected-1; y++ {
			tp.ApplyTransactionBandingToPixel(pixLeftAffected, y,
				pixLeft, pixRight, pixTop, pixBottom, overlap,
				transR, transG, transB, transO, isBlockReward, depth,
				isFirstTxi, isLastTxi)
		}
	}
	// Similarly for right column (not including corners)
	overlap = pixRight - floatCoords(pixRightAffected)
	if overlap > 0 {
		for y := pixTopAffected + 1; y <= pixBottomAffected-1; y++ {
			tp.ApplyTransactionBandingToPixel(pixRightAffected, y,
				pixLeft, pixRight, pixTop, pixBottom, overlap,
				transR, transG, transB, transO, isBlockReward, depth,
				isFirstTxi, isLastTxi)
		}
	}
	// Corners are a faff but are very much needed, because very small rectangles are very common!
	// Of particular faff is that all four corners (or fewer) may be the same pixel :-O so only count once
	leftEqualsRight := (pixLeftAffected == pixRightAffected)
	topEqualsBottom := (pixTopAffected == pixBottomAffected)
	overlapTL := overlapArea(pixLeft, pixRight, pixTop, pixBottom, floatCoords(pixLeftAffected), floatCoords(pixLeftAffected+1), floatCoords(pixTopAffected), floatCoords(pixTopAffected+1))
	overlapTR := overlapArea(pixLeft, pixRight, pixTop, pixBottom, floatCoords(pixRightAffected), floatCoords(pixRightAffected+1), floatCoords(pixTopAffected), floatCoords(pixTopAffected+1))
	overlapBL := overlapArea(pixLeft, pixRight, pixTop, pixBottom, floatCoords(pixLeftAffected), floatCoords(pixLeftAffected+1), floatCoords(pixBottomAffected), floatCoords(pixBottomAffected+1))
	overlapBR := overlapArea(pixLeft, pixRight, pixTop, pixBottom, floatCoords(pixRightAffected), floatCoords(pixRightAffected+1), floatCoords(pixBottomAffected), floatCoords(pixBottomAffected+1))
	// Top left
	if overlapTL > 0 {
		tp.ApplyTransactionBandingToPixel(pixLeftAffected, pixTopAffected,
			pixLeft, pixRight, pixTop, pixBottom, overlapTL,
			transR, transG, transB, transO, isBlockReward, depth,
			isFirstTxi, isLastTxi)
	}
	// Maybe top right
	if !leftEqualsRight && overlapTR > 0 {
		tp.ApplyTransactionBandingToPixel(pixRightAffected, pixTopAffected,
			pixLeft, pixRight, pixTop, pixBottom, overlapTR,
			transR, transG, transB, transO, isBlockReward, depth,
			isFirstTxi, isLastTxi)
	}
	// Maybe bottom left and / or right
	if !topEqualsBottom {
		if overlapBL > 0 {
			tp.ApplyTransactionBandingToPixel(pixLeftAffected, pixBottomAffected,
				pixLeft, pixRight, pixTop, pixBottom, overlapBL,
				transR, transG, transB, transO, isBlockReward, depth,
				isFirstTxi, isLastTxi)
		}
		if !leftEqualsRight && overlapBR > 0 {
			tp.ApplyTransactionBandingToPixel(pixRightAffected, pixBottomAffected,
				pixLeft, pixRight, pixTop, pixBottom, overlapBR,
				transR, transG, transB, transO, isBlockReward, depth,
				isFirstTxi, isLastTxi)
		}
	}

	// The totally affected pixels in the middle (if any) are easy...
	for x := pixLeftAffected + 1; x <= pixRightAffected-1; x++ {
		for y := pixTopAffected + 1; y <= pixBottomAffected-1; y++ {
			tp.ApplyTransactionBandingToPixel(x, y,
				pixLeft, pixRight, pixTop, pixBottom, 1.0,
				transR, transG, transB, transO, isBlockReward, depth,
				isFirstTxi, isLastTxi)
		}
	}

	return finished
}

func (tp *transactionPixels) ApplyTransactionBandingToPixel(x int, y int,
	transLeft floatCoords, transRight floatCoords, transTop floatCoords, transBottom floatCoords, overlapAmount floatCoords,
	transR byte, transG byte, transB byte, transO byte,
	isBlockReward bool, depth int,
	isFirstTxi bool, isLastTxi bool) {

	// Skip transactions where there is only one txi
	if isFirstTxi && isLastTxi {
		return
	}

	tp.pixelMutexes[y][x].Lock()

	u := floatCoords(x)
	uStart := transLeft
	uEnd := transRight
	washProportion := transactionBandingProfile(u, uStart, uEnd, isFirstTxi, isLastTxi)

	var newR, newG, newB, newO byte
	var existingR, existingG, existingB, existingO byte
	if tp.partialPixels[y][x] == nil {
		// A complete pixel, get it from there
		existingR = tp.completedPixels[y][x][0]
		existingG = tp.completedPixels[y][x][1]
		existingB = tp.completedPixels[y][x][2]
		existingO = tp.completedPixels[y][x][3]
	} else {
		exR, exG, exB, exO, _ := tp.summmarizePixel(x, y)
		existingR = byte(exR)
		existingG = byte(exG)
		existingB = byte(exB)
		existingO = byte(exO)
	}

	const ADJUST_TOP_LEVEL = false
	const GAP_IN_MIDDLE = true
	const DIAGONAL_STUFF = true
	if DIAGONAL_STUFF {
		pixWidth := float64(transRight - transLeft)
		pixHeight := float64(transBottom - transTop)
		pixDivider := (pixWidth + pixHeight) / 2.0
		pixMidX := float64(transLeft+transRight) / 2
		pixMidY := float64(transTop+transBottom) / 2

		// These are relative to the midpoint
		// Create a set of 9 subpixels
		xSubPixels := make([]float64, 9)
		ySubPixels := make([]float64, 9)
		counter := 0
		for jj := -0.33; jj < 0.34; jj += 0.33 {
			for ii := -0.33; ii < 0.34; ii += 0.33 {
				xSubPixels[counter] = (float64(x) + ii - pixMidX) / pixDivider
				ySubPixels[counter] = (float64(y) + jj - pixMidY) / pixDivider
				counter++
			}
		}

		// Rotate by 1/256 revolution per octarine digit
		angle := float64(transO) * math.Pi * 2 / 256
		sin := math.Sin(angle)
		cos := math.Cos(angle)
		for i := 0; i < 9; i++ {
			xSubPixels[i], ySubPixels[i] = cos*xSubPixels[i]-sin*ySubPixels[i], sin*xSubPixels[i]+cos*ySubPixels[i]
		}

		stripes := 3.0 // Number of stripes that fit into average of edge width and height
		if pixWidth > pixHeight {
			if math.Abs(sin) > math.Sqrt(0.5) {
				stripes += 2 // More stripes if wide transaction and horizontal stripes
			}
		} else {
			if math.Abs(sin) < math.Sqrt(0.5) {
				stripes += 2 // More stripes if tall transaction and vertical stripes
			}
		}

		// Something is varied periodically as adjuster increases
		// Giving stripes at integer values of adjusterOneCycle
		adjusterOneCycle := make([]float64, 9)
		for i := 0; i < 9; i++ {
			adjusterOneCycle[i] = xSubPixels[i]
			if GAP_IN_MIDDLE {
				adjusterOneCycle[i] += 0.5
			}
		}

		for i := 0; i < 9; i++ {
			adjusterOneCycle[i] *= stripes
		}

		const SPRAY_PAINT_STRIPES = true
		const LAMBERTIAN_STICKS = !SPRAY_PAINT_STRIPES
		if (SPRAY_PAINT_STRIPES || LAMBERTIAN_STICKS) && !isBlockReward {
			if depth > 0 || ADJUST_TOP_LEVEL {
				// Let stripingMultiplier be a striping multiplier across transaction in direction of rotated vecX.

				var modifiedTransR byte
				var modifiedTransG byte
				var modifiedTransB byte
				var modifiedTransO byte
				var stripingMultiplier float64
				var stripingAlpha float64

				if SPRAY_PAINT_STRIPES {
					const GAP_TO_PIPE_RATIO = 5
					brightness, alpha := sprayPaintProfile(adjusterOneCycle[4], GAP_TO_PIPE_RATIO)
					stripingMultiplier = brightness
					stripingAlpha = alpha
				} else if LAMBERTIAN_STICKS {
					// Lambertian pipe profile instead
					const GAP_TO_PIPE_RATIO = 10
					brightness, alpha := lambertianPipeProfile(adjusterOneCycle, GAP_TO_PIPE_RATIO)
					stripingMultiplier = brightness
					stripingAlpha = alpha
				}
				// Save time if we're not on or near a pipe
				if stripingMultiplier == 0.0 {
					newR = existingR
					newG = existingG
					newB = existingB
					newO = existingO
				} else {
					modifiedTransR = byte(float64(transR) * stripingMultiplier)
					modifiedTransG = byte(float64(transG) * stripingMultiplier)
					modifiedTransB = byte(float64(transB) * stripingMultiplier)
					modifiedTransO = byte(float64(transO) * stripingMultiplier)

					// Instead of effectively "striping in black", we stripe in the transaction colour
					amountTransaction := stripingAlpha
					amountUnderlying := 1.0 - stripingAlpha
					newR = byte(amountUnderlying*float64(existingR) + amountTransaction*float64(modifiedTransR))
					newG = byte(amountUnderlying*float64(existingG) + amountTransaction*float64(modifiedTransG))
					newB = byte(amountUnderlying*float64(existingB) + amountTransaction*float64(modifiedTransB))
					newO = byte(amountUnderlying*float64(existingO) + amountTransaction*float64(modifiedTransO))
				}
			} else {
				newR, newG, newB, newO = existingR, existingG, existingB, existingO
			}
		} else {
			newR, newG, newB, newO = taintPixel(existingR, existingG, existingB, existingO, transR, transG, transB, transO, washProportion)
		}
	}

	tp.replaceColour(x, y, newR, newG, newB, newO)
	tp.pixelMutexes[y][x].Unlock()
}

func transactionBandingProfile(u floatCoords, uStart floatCoords, uEnd floatCoords, isFirstTxi bool, isLastTxi bool) float64 {
	uWidth := uEnd - uStart
	paramAcrossBand := (float64((u-uStart)/uWidth) - 0.5) * 2
	//inFirstHalf := (paramAcrossBand < 0.0)

	// Avoid banding for a transaction with a single input
	if isFirstTxi && isLastTxi {
		return 0.0
	}

	/*
		// Avoid multiple layers of banding stripes (looks too strong!) at the edges of transactions
		if inFirstHalf && isFirstTxi {
			return 0.0
		}
		if !inFirstHalf && isLastTxi {
			return 0.0
		}*/

	//sine := math.Sin(paramAcrossBand*math.Pi/2.0)*0.5 + 0.5
	const SCALE = 0.0
	const POWER = 2.0
	const FLAT_PROPORTION = 1.0
	return SCALE * (FLAT_PROPORTION + (1.0-FLAT_PROPORTION)*math.Pow(paramAcrossBand, POWER))
}

func (tp *transactionPixels) outputGraphicsFile(n int) error {
	f, err := os.Create("output" + strconv.Itoa(n) + ".ppm")
	if err != nil {
		println("Couldn't create output.ppm")
		return err
	}
	defer f.Close()

	f.WriteString("P6 " + strconv.Itoa(tp.width) + " " + strconv.Itoa(tp.height) + " 255\n")
	for y := 0; y < tp.height; y++ {
		for x := 0; x < tp.width; x++ {
			var arr [3]byte
			arr[0] = tp.completedPixels[y][x][0]
			arr[1] = tp.completedPixels[y][x][1]
			arr[2] = tp.completedPixels[y][x][2]
			f.Write(arr[0:3])
		}
	}
	return nil
}

func taintPixel(r byte, g byte, b byte, o byte, taintR byte, taintG byte, taintB byte, taintO byte, taintAmt float64) (rdash byte, gdash byte, bdash byte, odash byte) {
	origAmt := 1.0 - taintAmt
	rdashh := float64(r)*origAmt + float64(taintR)*taintAmt
	gdashh := float64(g)*origAmt + float64(taintG)*taintAmt
	bdashh := float64(b)*origAmt + float64(taintB)*taintAmt
	odashh := float64(o)*origAmt + float64(taintO)*taintAmt
	if rdashh > 255 {
		panic("oops")
	}
	if gdashh > 255 {
		panic("oops")
	}
	if bdashh > 255 {
		panic("oops")
	}
	if odashh > 255 {
		panic("oops")
	}
	return byte(rdashh), byte(gdashh), byte(bdashh), byte(odashh)
}

func HashOfInt(in uint64) [32]byte {
	var bytes [8]byte
	binary.LittleEndian.PutUint64(bytes[0:8], in)

	h := sha256.New()
	h.Write(bytes[0:8])

	var outBytes [32]byte
	o := h.Sum(nil)
	for i := 0; i < len(o); i++ {
		outBytes[i] = o[i]
	}
	return outBytes
}

// Pipes are centered at integer values of adjuster
// We are given the number of pipes that would fit into each gap
func lambertianPipeProfile(adjusters []float64, gapToPipeRatio float64) (float64, float64) {
	numSubPixels := len(adjusters)
	brightnesses := make([]float64, numSubPixels)
	valids := make([]bool, numSubPixels)

	for i := 0; i < numSubPixels; i++ {
		// Nearest pipe
		nearestPipe := math.Round(adjusters[i])
		distToNearestPipe := math.Abs(adjusters[i] - nearestPipe)
		// For a gap to pipe ratio of zero, pipe diameter is one (one pipe at each counting number)
		// For a gap to pipe ratio of one, pipe diameter is a half (so a quarter radius, plus half gap, plus quarter radius between counting numbers)
		// For a gap to pipe ratio of two, pipe diameter is a third
		pipeDiameter := 1.0 / (gapToPipeRatio + 1.0)
		numberOfPipeRadiiToNearestPipe := distToNearestPipe / (pipeDiameter / 2.0)
		// If any subpixel is comfortably more than one pipe radius away, we can
		// save time by just returning alpha of 0.0
		if numberOfPipeRadiiToNearestPipe > 2.0 {
			return 0.0, 0.0
		}
		if numberOfPipeRadiiToNearestPipe <= 1.0 {
			// We are inside the pipe
			cosine := math.Sqrt(1.0 - numberOfPipeRadiiToNearestPipe*numberOfPipeRadiiToNearestPipe)
			brightnesses[i] = cosine
			valids[i] = true
		} else {
			// Leave valid as false
		}
	}

	total := 0.0
	count := 0
	for i := 0; i < numSubPixels; i++ {
		if valids[i] {
			total += brightnesses[i]
			count++
		}
	}
	alpha := float64(count) / float64(numSubPixels)
	brightness := total / float64(count)
	return brightness, alpha
}

// Pipes are centered at integer values of adjuster
// We are given the number of pipes that would fit into each gap
func sprayPaintProfile(adjuster float64, gapToPipeRatio float64) (float64, float64) {
	var alpha float64

	// Nearest pipe
	nearestPipe := math.Round(adjuster)
	distToNearestPipe := math.Abs(adjuster - nearestPipe)
	// For a gap to pipe ratio of zero, pipe diameter is one (one pipe at each counting number)
	// For a gap to pipe ratio of one, pipe diameter is a half (so a quarter radius, plus half gap, plus quarter radius between counting numbers)
	// For a gap to pipe ratio of two, pipe diameter is a third
	pipeDiameter := 1.0 / (gapToPipeRatio + 1.0)
	numberOfPipeRadiiToNearestPipe := distToNearestPipe / (pipeDiameter / 2.0)
	// A spray paint pipe actually extends beyond the radius (by twice as much, with some alpha at the edges
	if numberOfPipeRadiiToNearestPipe <= 2.0 {
		// We are inside the pipe
		// Zero is the centre, pi is the transparent edge
		cosineParam := numberOfPipeRadiiToNearestPipe * math.Pi / 2.0
		cosine := math.Cos(cosineParam)
		alpha = cosine/2.0 + 0.5
		return 1.0, alpha
	} else {
		return 0.0, 0.0
	}
}
