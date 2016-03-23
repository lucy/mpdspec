package main

import "github.com/lucy/termbox-go"

const (
	UL = (1 << iota)
	UR
	BL
	BR
)

var codes = [...]rune{
	' ', '▘', '▝', '▀',
	'▖', '▌', '▞', '▛',
	'▗', '▚', '▐', '▜',
	'▄', '▙', '▟', '█',
}

type x4 struct {
	arr    []int
	w, h   int
	color  bool
	on     termbox.Attribute
	off    termbox.Attribute
	colors []termbox.Attribute
}

func newX4(colors []termbox.Attribute, on, off termbox.Attribute, color bool) *x4 {
	b := &x4{
		arr:    nil,
		w:      0,
		h:      0,
		color:  color,
		on:     termbox.ColorDefault,
		off:    termbox.ColorDefault,
		colors: colors,
	}
	b.fix()
	return b
}

func (x *x4) fix() {
	x.w, x.h = termbox.Size()
	x.w, x.h = x.w*2, x.h*2
	l := x.w * x.h
	if l > cap(x.arr) {
		x.arr = make([]int, l)
	}
	x.arr = x.arr[:l]
	for i := range x.arr {
		x.arr[i] = -1
	}
}

func (b *x4) set(x, y int, c int) {
	if y*b.w+x >= len(b.arr) || x < 0 || y < 0 {
		return
	}
	b.arr[y*b.w+x] = c
}

func sign(x bool) int {
	if x {
		return 1
	}
	return -1
}

func (b *x4) line(x0, y0, x1, y1 int, c int) {
	dx, dy := abs(x1-x0), abs(y1-y0)
	sx, sy := sign(x0 < x1), sign(y0 < y1)
	e := dx - dy
	for {
		b.set(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := e * 2
		if e2 > -dy {
			e -= dy
			x0 += sx
		}
		if e2 < dx {
			e += dx
			y0 += sy
		}
	}
}

func (b *x4) get(x, y int) int {
	if y*b.w+x >= len(b.arr) || x < 0 || y < 0 {
		return 0
	}
	return b.arr[y*b.w+x]
}

func (b *x4) do() {
	w, h := b.w/2, b.h/2
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			xx, yy := x*2, y*2
			ul, ur, bl, br :=
				b.arr[(yy+0)*b.w+(xx+0)],
				b.arr[(yy+0)*b.w+(xx+1)],
				b.arr[(yy+1)*b.w+(xx+0)],
				b.arr[(yy+1)*b.w+(xx+1)]
			ic := 0
			c := 0
			a := 0
			if ul >= 0 {
				ic += ul
				c |= UL
				a++
			}
			if ur >= 0 {
				ic += ur
				c |= UR
				a++
			}
			if bl >= 0 {
				ic += bl
				c |= BL
				a++
			}
			if br >= 0 {
				ic += br
				c |= BR
				a++
			}
			on := b.on
			if b.color {
				if a > 0 {
					on = b.colors[ic/a]
				} else {
					on = b.colors[0]
				}
			}
			termbox.SetCell(x, y, codes[c], on, b.off)
		}
	}
}
