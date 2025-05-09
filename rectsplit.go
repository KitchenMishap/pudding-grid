package main

// Various ways to split a rectangle
type SplitRect interface {
	SplitRectIntoWeightedRects(orig rect, weights []floatCoords, depth int, useUnitSquare bool) []rect
}

type SplitRectSlabs struct {
}

// **********************************************************************************************************
// FIRST the original method which distributes all across x or y, alternating between x and y based on depth
// **********************************************************************************************************
func (srs *SplitRectSlabs) SplitRectIntoWeightedRects(orig rect, weights []floatCoords, depth int, useUnitSquare bool) []rect {
	return SplitRectIntoWeightedSlabs(orig, weights, (depth&1) == 1)
}

type rect struct {
	left   floatCoords
	right  floatCoords
	top    floatCoords
	bottom floatCoords
}

func SplitRectIntoWeightedSlabs(orig rect, weights []floatCoords, alongX bool) []rect {
	results := SplitUnitSquareIntoWeightedAreaYSlabs(weights)
	for index, rectCopy := range results {
		// First swap if necessary
		if alongX {
			rectCopy.left, rectCopy.right, rectCopy.top, rectCopy.bottom = rectCopy.top, rectCopy.bottom, rectCopy.left, rectCopy.right
		}
		// Then scale
		width := orig.right - orig.left
		height := orig.bottom - orig.top
		results[index].left = orig.left + width*rectCopy.left
		results[index].right = orig.left + width*rectCopy.right
		results[index].top = orig.top + height*rectCopy.top
		results[index].bottom = orig.top + height*rectCopy.bottom
	}
	return results
}

func SplitUnitSquareIntoWeightedAreaYSlabs(weights []floatCoords) []rect {
	weightTotal := floatCoords(0.0)
	for _, weight := range weights {
		weightTotal += weight
	}
	results := make([]rect, len(weights))
	weightAssigned := floatCoords(0.0)
	for index, weight := range weights {
		results[index].left = 0.0
		results[index].top = weightAssigned / weightTotal
		results[index].right = 1.0
		weightAssigned += weight
		results[index].bottom = weightAssigned / weightTotal
	}
	return results
}

// **********************************************************************************************************
// SECOND the squarify treemap algorithm
// https://vanwijk.win.tue.nl/stm.pdf
// **********************************************************************************************************

type SquarifySplitter struct {
}

func (s *SquarifySplitter) SplitRectIntoWeightedRects(orig rect, weights []floatCoords, depth int, useUnitSquare bool) []rect {
	var results []rect

	if useUnitSquare {
		results = squarifyUnitSquare(weights)

		// Multiply up and translate into given parent rect
		widthMult := orig.right - orig.left
		heightMult := orig.bottom - orig.top
		for index, rectCopy := range results {
			results[index].left = orig.left + rectCopy.left*widthMult
			results[index].right = orig.left + rectCopy.right*widthMult
			results[index].top = orig.top + rectCopy.top*heightMult
			results[index].bottom = orig.top + rectCopy.bottom*heightMult
		}
	} else {
		results = squarifyRect(orig, weights)
	}

	return results
}

func squarifyRect(r rect, weights []floatCoords) []rect {
	// We use the code from https://github.com/jeffwilliams/squarify (in squarify.go)
	sizer := weightsToTreesizer(weights)
	rectangle := Rect{float64(r.left), float64(r.top), float64(r.right - r.left), float64(r.bottom - r.top)}
	margins := Margins{0, 0, 0, 0}
	options := Options{10, &margins, true, 0.0, 0.0}
	blocks, meta := Squarify(&sizer, rectangle, options)
	result := make([]rect, len(weights))
	for index, block := range blocks {
		if meta[index].Depth == 0 {
			newRect := rect{}
			newRect.left = floatCoords(block.X)
			newRect.top = floatCoords(block.Y)
			newRect.right = floatCoords(block.X) + floatCoords(block.W)
			newRect.bottom = floatCoords(block.Y) + floatCoords(block.H)
			// This bit of code "unsorts" the sorting done by size
			ts := block.TreeSizer
			for ind, child := range sizer.children {
				if (!sizer.children[ind].found) && child.size == ts.Size() {
					result[ind] = newRect
					sizer.children[ind].found = true
					break
				}
			}
		}
	}
	if len(result) != len(weights) {
		panic("lost some rectangles somewhere")
	}
	return result
}

func squarifyUnitSquare(weights []floatCoords) []rect {
	return squarifyRect(rect{0.0, 1.0, 0.0, 1.0}, weights)
}

// Implements interface TreeSizer
type treesizer struct {
	size     float64
	children []treesizer
	found    bool
}

func (ts *treesizer) Size() float64 {
	return ts.size
}
func (ts *treesizer) NumChildren() int {
	return len(ts.children)
}
func (ts *treesizer) Child(n int) TreeSizer {
	return &ts.children[n]
}

func weightsToTreesizer(weights []floatCoords) treesizer {
	result := treesizer{} // Starts off with zero size and no children
	result.children = make([]treesizer, len(weights))
	for index, weight := range weights {
		result.children[index].size = float64(weight)
		result.children[index].found = false
		result.size += float64(weight)
	}
	return result
}
