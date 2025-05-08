package main

import (
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
	// Pixels
	width  int
	height int
	// The transaction is treated as the unit square
	// The pixel grid maps to the following transaction rectangle relative to the unit square
	left   floatCoords
	right  floatCoords
	top    floatCoords
	bottom floatCoords
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
	// The big arrays from the files
	firstTxi []uint64
	firstTxo []uint64
	txiTx    []uint32
	txiVout  []uint32
	txoValue []uint64
	red      []byte
	green    []byte
	blue     []byte
	octarine []byte
	hashMSBs []uint32
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

func createTransactionImage(transactionIndex uint32) error {
	var fwm fileWriteManager
	err := fwm.readConfig()
	if err != nil {
		return err
	}
	fwm.configureFileChoices()
	fwm.openFiles()
	defer fwm.closeFiles()

	println("Reading firstTxi file")
	firstTxi, err := fwm.flatFileInts["TransFirstTxi"].wholeFileAsInt64()
	if err != nil {
		println("Could not read TransFirstTxi file")
		return err
	}

	println("Reading firstTxo file")
	firstTxo, err := fwm.flatFileInts["TransFirstTxo"].wholeFileAsInt64()
	if err != nil {
		println("Could not read TransFirstTxi file")
		return err
	}

	println("Reading TxiTx file")
	txiTx, err := fwm.flatFileInts["TxiTx"].wholeFileAsInt32()
	if err != nil {
		println("Could not read TxiTx file")
		return err
	}

	println("Reading TxiVout file")
	txiVout, err := fwm.flatFileInts["TxiVout"].wholeFileAsInt32()
	if err != nil {
		println("Could not read TxiVout file")
		return err
	}

	println("Reading TxoValue file")
	txoValue, err := fwm.flatFileInts["TxoValue"].wholeFileAsInt64()
	if err != nil {
		println("Could not read TxoValue file")
		return err
	}

	println("Reading colourdb files: Red")
	red, err := fwm.flatFileInts["TransRed"].wholeFileAsByte()
	if err != nil {
		println("Could not read TransRed file")
		return err
	}
	println("Reading colourdb files: Green")
	green, err := fwm.flatFileInts["TransGreen"].wholeFileAsByte()
	if err != nil {
		println("Could not read TransGreen file")
		return err
	}
	println("Reading colourdb files: Blue")
	blue, err := fwm.flatFileInts["TransBlue"].wholeFileAsByte()
	if err != nil {
		println("Could not read TransBlue file")
		return err
	}
	println("Reading colourdb files: Octarine")
	octarine, err := fwm.flatFileInts["TransOctarine"].wholeFileAsByte()
	if err != nil {
		println("Could not read TransOctarine file")
		return err
	}

	println("Reading hashMSBs file")
	hashMSBs, err := fwm.transHashLookupFiles.WholeFileAsInt32()
	if err != nil {
		println("Could not read hashes file")
		return err
	}

	//for zoom := 1; zoom <= 1048576*1048576; zoom *= 4 {
	for zoom := 1; zoom <= 1; zoom *= 4 {
		tp := NewTransactionPixels(firstTxi, firstTxo, txiTx, txiVout, txoValue, red, green, blue, octarine, hashMSBs, floatCoords(zoom))
		tp.drawTransactionPixels(transactionIndex)
		tp.outputGraphicsFile(zoom)
	}

	return nil
}

func NewTransactionPixels(firstTxi []uint64, firstTxo []uint64, txiTx []uint32, txiVout []uint32, txoValue []uint64, red []byte, green []byte, blue []byte, octarine []byte, hashMSBs []uint32, zoom floatCoords) *transactionPixels {
	tp := new(transactionPixels)
	tp.width = 1000
	tp.height = 1000
	//tp.width = 1920 // Full HD
	//tp.height = 1080
	//tp.width = 3440	// My monitor
	//tp.height = 1440
	//tp.width = 4950 // A3 300 DPI
	//tp.height = 3510
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
	tp.left = 0.5 - 0.5/zoom
	tp.right = 0.5 + 0.5/zoom
	tp.top = 0.5 - 0.5/zoom
	tp.bottom = 0.5 + 0.5/zoom
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
	tp.doneThreshold = 255.0 / 256.0
	tp.maxDepth = 700000
	tp.areaThreshold = 0.5
	tp.maxContributions = 30
	tp.maxGoroutines = 2
	tp.goroutines = tp.maxGoroutines
	// The arrays from files
	tp.firstTxi = firstTxi
	tp.firstTxo = firstTxo
	tp.txiTx = txiTx
	tp.txiVout = txiVout
	tp.txoValue = txoValue
	tp.red = red
	tp.green = green
	tp.blue = blue
	tp.octarine = octarine
	tp.hashMSBs = hashMSBs
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
	if proportion <= 0 {
		panic("Non positive proportion")
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
	xScale := (tp.right - tp.left) / floatCoords(tp.width)
	yScale := (tp.bottom - tp.top) / floatCoords(tp.height)
	left = tp.left + floatCoords(x)*xScale
	right = left + xScale
	top = tp.top + floatCoords(y)*yScale
	bottom = top + yScale
	return left, right, top, bottom
}

func (tp *transactionPixels) transactionToPixelCoords(x floatCoords, y floatCoords) (px floatCoords, py floatCoords) {
	xScale := (tp.right - tp.left) / floatCoords(tp.width)
	yScale := (tp.bottom - tp.top) / floatCoords(tp.height)
	px = (x - tp.left) / xScale
	py = (y - tp.top) / yScale
	return px, py
}

func (tp *transactionPixels) drawTransactionPixels(transactionIndex uint32) {
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
	// (Find the overall taint first)
	hashMSB := tp.hashMSBs[transactionIndex]
	tr := byte((hashMSB & 0xFF000000) >> 24)
	tg := byte((hashMSB & 0x00FF0000) >> 16)
	tb := byte((hashMSB & 0x0000FF00) >> 8)
	to := byte((hashMSB & 0x000000FF))
	finished = finished || tp.drawTransactionRecurse(transactionIndex, 0.0, 1.0, 0.0, 1.0, true, 0, false, tr, tg, tb, to, 0.00)
	if finished {
		println("Properly finished")
	} else {
		println("Finished but not finished!")
		tp.incompletePixelsReport()
	}
}

// Returns true if finished overall
func (tp *transactionPixels) drawTransactionRecurse(transactionIndex uint32, left floatCoords, right floatCoords, top floatCoords, bottom floatCoords, xInsteadOfY bool, depth int, insistOneLastDepth bool, taintR byte, taintG byte, taintB byte, taintO byte, proportionTaint float64) bool {
	finished := false
	// Gather / calculate some things...

	// The average colour of the transaction before we recurse to higher detail and before we apply the taint
	r := tp.red[transactionIndex]
	g := tp.green[transactionIndex]
	b := tp.blue[transactionIndex]
	o := tp.octarine[transactionIndex]

	// Apply the taint from the level above
	// These numbers are only used if we don't recurse to the next level
	r, g, b, o = taintPixel(r, g, b, o, taintR, taintG, taintB, taintO, proportionTaint)

	// Work out the taint that the tx itself applies to its children
	hashMSB := tp.hashMSBs[transactionIndex]
	tr := byte((hashMSB & 0xFF000000) >> 24)
	tg := byte((hashMSB & 0x00FF0000) >> 16)
	tb := byte((hashMSB & 0x0000FF00) >> 8)
	to := byte((hashMSB & 0x000000FF))
	// And the taint amount. Scale the taint amount by the octarine to give various alphas
	//ta := 0.15 * float64(to) / 255.0

	ta := 0.00
	// Only apply the taint if there are more than one inputs to the transaction
	// #artisticlicense This avoids an image being "Broadly all one colour"
	begbeg := tp.firstTxi[transactionIndex]
	endend := tp.firstTxi[transactionIndex+1]
	skipTaint := (endend-begbeg == 1)

	// Adjust the taint amount
	nextProportionTaint := proportionTaint + (1-proportionTaint)*ta
	if skipTaint {
		nextProportionTaint = proportionTaint
	} else {
		// Don't forget to taint the input taint!
		taintR, taintG, taintB, taintO = taintPixel(tr, tg, tb, to, taintR, taintG, taintB, taintO, proportionTaint)
	}

	// We need to get the intersection of the image and T
	// We check whether there IS any intersection initially
	if tp.left >= right {
		return false // Image is wholly to right of transaction
	}
	if tp.right <= left {
		return false // image is wholly to left of transaction
	}
	if tp.top >= bottom {
		return false // image is wholly below transaction
	}
	if tp.bottom <= top {
		return false // image is wholly above transaction
	}
	// Start by presuming the intersection IS the image, then clip it to the transaction
	interLeft := tp.left
	interRight := tp.right
	interTop := tp.top
	interBottom := tp.bottom
	if interLeft < left {
		interLeft = left
	}
	if interRight > right {
		interRight = right
	}
	if interTop < top {
		interTop = top
	}
	if interBottom > bottom {
		interBottom = bottom
	}
	// Now we convert from transaction co-ordinates to pixel co-ordinates (as floats)
	pixLeft, pixTop := tp.transactionToPixelCoords(interLeft, interTop)
	pixRight, pixBottom := tp.transactionToPixelCoords(interRight, interBottom)
	// If we have a zero area rectangle we can't expect progress!
	if pixLeft == pixRight || pixTop == pixBottom {
		return false
	}
	// We now use rounding to gather the range of pixels affected (both partially and wholly)
	pixLeftAffected := int(math.Floor(float64(pixLeft)))
	pixTopAffected := int(math.Floor(float64(pixTop)))
	pixRightAffected := int(math.Floor(float64(pixRight)))
	pixBottomAffected := int(math.Floor(float64(pixBottom)))

	// Z is the direction (x or y) that we're splitting the transaction along
	// zLength is the length of the transaction, in pixels, along that direction x or y
	// The following vals will help us decide whether to go deeper
	width := pixRight - pixLeft
	height := pixBottom - pixTop
	letsGoDeeper := width*height > tp.areaThreshold
	oneLastDepthWorthwhile := false

	// Is T a block reward? (a single input from transaction zero)
	isBlockReward := (tp.txiTx[tp.firstTxi[transactionIndex]] == 0)

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
		total := uint64(0)
		beg := tp.firstTxi[transactionIndex]
		end := tp.firstTxi[transactionIndex+1]
		for txi := beg; txi < end; txi++ {
			// Get the transaction the input came from
			tx := tp.txiTx[txi]
			// Get the vout of the input
			vout := tp.txiVout[txi]
			// Get the txo index corresponding to the txi
			txo := tp.firstTxo[tx] + uint64(vout)
			// Get the bitcoin value of the txo (in satoshis)
			sats := tp.txoValue[txo]
			total += sats
		}
		// Now recurse
		// z is "either" x or y
		var prevZ floatCoords
		if xInsteadOfY {
			prevZ = left
		} else {
			prevZ = top
		}
		satSum := uint64(0) // The satoshis we have encountered so far

		// We want to do the biggest input first, as it is more likely to complete
		// some pixels (and that will mean others might not need to be examined in depth).
		// So gather the info we need about the inputs into arrays before we start with biggest.
		var txs []uint32
		var sats []uint64
		var zstarts []floatCoords
		var zends []floatCoords
		var dones []bool
		for txi := tp.firstTxi[transactionIndex]; txi < tp.firstTxi[transactionIndex+1]; txi++ {
			// Get the transaction the input came from
			tx := tp.txiTx[txi]
			txs = append(txs, tx)
			// Get the vout of the input
			vout := tp.txiVout[txi]
			// Get the txo index corresponding to the txi
			txo := tp.firstTxo[tx] + uint64(vout)
			// Get the bitcoin value of the txo (in satoshis)
			sat := tp.txoValue[txo]
			sats = append(sats, sat)
			satSum += sat

			var z floatCoords
			if xInsteadOfY {
				z = left + (floatCoords(satSum)/floatCoords(total))*(right-left)
			} else {
				z = top + (floatCoords(satSum)/floatCoords(total))*(bottom-top)
			}
			zstarts = append(zstarts, prevZ)
			zends = append(zends, z)
			prevZ = z
			dones = append(dones, false)
		}
		// Now recurse, biggest first, until all dones are true
		// Here is where we start thinking about multithreading using goroutines
		var wg sync.WaitGroup
		routines := 0

		for {
			biggestSats := uint64(0)
			biggestIndex := -1
			// Find the i with the biggest sats[i]
			for i := 0; i < len(txs); i++ {
				if !dones[i] && sats[i] > biggestSats {
					biggestSats = sats[i]
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
					if xInsteadOfY {
						go func(_tx uint32, _l floatCoords, _r floatCoords, _t floatCoords, _b floatCoords) {
							defer wg.Done()
							tp.drawTransactionRecurse(_tx, _l, _r, _t, _b, false, depth+1, oneLastDepthWorthwhile, taintR, taintG, taintB, taintO, nextProportionTaint)
						}(txs[biggestIndex], zstarts[biggestIndex], zends[biggestIndex], top, bottom)
					} else {
						go func(_tx uint32, _l floatCoords, _r floatCoords, _t floatCoords, _b floatCoords) {
							defer wg.Done()
							tp.drawTransactionRecurse(_tx, _l, _r, _t, _b, true, depth+1, oneLastDepthWorthwhile, taintR, taintG, taintB, taintO, nextProportionTaint)
						}(txs[biggestIndex], left, right, zstarts[biggestIndex], zends[biggestIndex])
					}
				} else {
					// Recurse as standard in the current goroutine
					// This could be because we are running in a single-threaded mode
					// But COULD be multithreaded; we've just run out of goroutines
					if xInsteadOfY {
						finished = finished || tp.drawTransactionRecurse(txs[biggestIndex], zstarts[biggestIndex], zends[biggestIndex], top, bottom, false, depth+1, oneLastDepthWorthwhile, taintR, taintG, taintB, taintO, nextProportionTaint)
					} else {
						finished = finished || tp.drawTransactionRecurse(txs[biggestIndex], left, right, zstarts[biggestIndex], zends[biggestIndex], true, depth+1, oneLastDepthWorthwhile, taintR, taintG, taintB, taintO, nextProportionTaint)
					}
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
	return finished
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
