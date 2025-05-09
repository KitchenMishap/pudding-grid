package main

import (
	"math"
	"math/rand"
)

func hsv2rgb(h byte, s byte, v byte) (r byte, g byte, b byte) {
	var hh, p, q, t, ff float32
	var i int
	var rrr, ggg, bbb float32

	hhh := float32(360) * float32(h) / float32(256)
	sss := float32(s) / float32(255)
	vvv := float32(v) / float32(255)
	hhh += rand.Float32() * float32(360) / float32(256)
	if sss <= 0.0 { // < is bogus, just shuts up warnings
		r = v
		g = v
		b = v
		return r, g, b
	}
	hh = hhh
	if hh >= 360.0 {
		hh = 0.0
	}
	hh /= 60.0
	i = int(hh)
	ff = float32(hh) - float32(i)
	p = vvv * (1.0 - sss)
	q = vvv * (1.0 - (sss * ff))
	t = vvv * (1.0 - (sss * (1.0 - ff)))

	switch i {
	case 0:
		rrr = vvv
		ggg = t
		bbb = p
	case 1:
		rrr = q
		ggg = vvv
		bbb = p
	case 2:
		rrr = p
		ggg = vvv
		bbb = t
	case 3:
		rrr = p
		ggg = q
		bbb = vvv
	case 4:
		rrr = t
		ggg = p
		bbb = vvv
	default:
		rrr = vvv
		ggg = p
		bbb = q
	}
	r = byte(rrr * 255)
	g = byte(ggg * 255)
	b = byte(bbb * 255)
	return r, g, b
}

// h, s, v goes 0 to 1
func rgb_to_hsv(rr byte, gg byte, bb byte) (h float64, s float64, v float64) {
	r := float64(rr) / 255
	g := float64(gg) / 255
	b := float64(bb) / 255
	maxPartial := math.Max(r, g)
	maxc := math.Max(b, maxPartial)
	minPartial := math.Min(r, g)
	minc := math.Min(b, minPartial)
	vv := maxc
	if minc == maxc {
		return 0, 0, vv
	}
	ss := (maxc - minc) / maxc
	rc := (maxc - r) / (maxc - minc)
	gc := (maxc - g) / (maxc - minc)
	bc := (maxc - b) / (maxc - minc)
	var hh float64
	if r == maxc {
		hh = 0.0 + bc - gc
	} else if g == maxc {
		hh = 2.0 + rc - bc
	} else {
		hh = 4.0 + gc - rc
	}
	hh = (hh / 6.0) - float64(int64(hh/6.0)) // Modulo 1.0
	return hh, ss, vv
}
