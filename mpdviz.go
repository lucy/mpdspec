// TODO: implement better weighting
// TODO: use an interface rather than function
package main

import (
	"fmt"
	"io"
	"math"
	"math/cmplx"
	"os"

	"github.com/lucy/go-fftw"
	flag "github.com/lucy/pflag"
	te "github.com/lucy/termbox-go"
	"github.com/mjibson/go-dsp/window"
)

var smooth = []float64{0.8, 0.8, 1, 1, 0.8, 0.8, 1, 0.8, 0.8, 1, 1, 0.8,
	1, 1, 0.8, 0.6, 0.6, 0.7, 0.8, 0.8, 0.8, 0.8, 0.8,
	0.8, 0.8, 0.8, 0.8, 0.8, 0.8, 0.8, 0.8, 0.8, 0.8, 0.8,
	0.8, 0.8, 0.8, 0.8, 0.8, 0.8, 0.8, 0.8, 0.8, 0.8, 0.8,
	0.7, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6, 0.6}

var awfreq = []float64{
	10, 12.5, 16, 20,
	25, 31.5, 40, 50,
	63, 80, 100, 125,
	160, 200, 250, 315,
	400, 500, 630, 800,
	1000, 1250, 1600, 2000,
	2500, 3150, 4000, 5000,
	6300, 8000, 10000, 12500,
	16000, 20000,
}

var awdb = []float64{
	-70.4, -63.4, -56.7, -50.5,
	-44.7, -39.4, -34.6, -30.2,
	-26.2, -22.5, -19.1, -16.1,
	-13.4, -10.9, -8.6, -6.6,
	-4.8, -3.2, -1.9, -0.8,
	0.0, 0.6, 1.0, 1.2,
	1.3, 1.2, 1.0, 0.5,
	-0.1, -1.1, -2.5, -4.3,
	-6.6, -9.3,
}

var (
	color    = flag.StringP("color", "c", "default", "Color to use")
	dim      = flag.BoolP("dim", "d", false, "Turn off bright colors where possible")
	filename = flag.StringP("file", "f", "/tmp/mpd.fifo", "Where to read pcm data from")
	fps      = flag.IntP("fps", "r", 60, "frames per second")
	skip     = flag.IntP("skip", "s", 2, "skip samples")
	icolor   = flag.BoolP("icolor", "i", false, "Color bars according to intensity (spectrum/lines)")
	imode    = flag.String("imode", "dumb", "Mode for colorisation (dumb, 256 or grayscale)")
	overlap  = flag.IntP("overlap", "p", 1, "number of samples to overlap")
	viz      = flag.StringP("viz", "v", "wave", "Visualisation (spectrum, wave or lines)")
	pow      = flag.Float64P("pow", "z", 1, "use n^pow for spectrum")
	flip     = flag.Bool("flip", false, "flip spectrum")
	bard_    = flag.Int("bard", 2, "bars = width/bard")
	bars_    = flag.Int("bars", 0, "0 means use bard")
	barw_    = flag.Int("barw", 0, "0 means use bard-1")
)

func main() {
	flag.Parse()
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "viz: %s\n", err)
		os.Exit(1)
	}
}

func catch(c chan error) {
	r := recover()
	if r != nil {
		c <- fmt.Errorf("%s", r)
	}
}

func run() error {
	f, err := os.Open(*filename)
	if err != nil {
		return err
	}
	defer f.Close()
	te.Init()
	defer te.Close()

	die := make(chan error)

	// input handler
	go func() {
		for {
			ev := te.PollEvent()
			switch {
			case ev.Ch == 'q':
				fallthrough
			case ev.Ch == 0 && ev.Key == te.KeyCtrlC:
				close(die)
				return
			}
		}
	}()

	c := newCtx(f, die)
	go spectrum(c)

	return <-die
}

type colorType int

const (
	color256 colorType = iota
)

type flags struct {
	color colorType
}

type ctx struct {
	fps        int       // frames per second
	overlap    int       // number of samples to overlap
	sampleRate int       // sample rate
	skip       int       // skip samples
	rSamples   []float64 // working buffer
	lSamples   []float64 // working buffer
	rSampleBuf []float64 // float factors
	lSampleBuf []float64 // float factors
	rWB        []float64
	lWB        []float64
	sampleRaw  []int16        // raw samples
	scale      float64        // scaling factor
	colors     []te.Attribute // color table
	back       x4             // draw buffer
	read       io.Reader      // sample reader
	die        chan error     // die channel
	mul        float64
	sc         float64
}

func newCtx(r io.Reader, die chan error) *ctx {
	rate := 44100 * 2 // XXX: don't hardcode
	c := []te.Attribute{
		12, 12, 12,
		12,
		te.ColorDefault, te.ColorDefault,
		te.ColorDefault,
		15, 15,
		15,
		253,
		253,
		254,
		254,
		255,
		255,
		255,
		255,
		21, 27, 33, 39, 45, 50, 49, 48, 47, 83, 82,
		118, 154, 155, 156, 192, 228, 227, 226, 220,
		214, 208, 209, 204, 204, 206, 201,
	}
	skip := *skip
	fps := *fps
	return &ctx{
		fps:        fps,
		overlap:    *overlap,
		sampleRate: rate,
		rSamples:   make([]float64, rate/fps/2**overlap),
		lSamples:   make([]float64, rate/fps/2**overlap),
		rSampleBuf: make([]float64, rate/fps/2**overlap),
		lSampleBuf: make([]float64, rate/fps/2**overlap),
		sampleRaw:  make([]int16, rate/fps),
		skip:       skip,
		scale:      math.Inf(1),
		colors:     c,
		back:       *newX4(c, 0, 0, true),
		read:       r,
		die:        die,
		mul:        20.0,
		sc:         60.0,
	}
}

func (c *ctx) readRaw() {
	err := readInt16s(c.read, c.sampleRaw)
	if err != nil {
		c.die <- err
	}
}

func (c *ctx) scaleRaw() {
	m := c.scale
	in := c.sampleRaw

	m += 1.0 / float64(c.fps)
	for i := 0; i < len(in); i++ {
		s := math.Abs(math.MaxInt16 / float64(in[i]))
		if s < m {
			m = s
		}
	}
	c.scale = m
}

func (c *ctx) convRaw() {
	inr := c.rSamples
	inl := c.lSamples
	inbr := c.rSampleBuf
	inbl := c.lSampleBuf
	in := c.sampleRaw

	s := c.sampleRate / c.fps / 2
	n := c.overlap
	copy(inbr[s:], inbr[:s*(n-1)])
	copy(inbl[s:], inbl[:s*(n-1)])
	for i := 0; i < len(in)-1; i += 2 {
		inbr[i/2] = float64(in[i+0]) / math.MaxInt16
		inbl[i/2] = float64(in[i+1]) / math.MaxInt16
	}
	copy(inr, inbr)
	copy(inl, inbl)
}

func (c *ctx) xy() (int, int) {
	return c.back.w, c.back.h
}

func (c *ctx) set(x, y, n int) {
	c.back.set(x, y, n)
}

func (c *ctx) line(x0, y0, x1, y1 int, n int) {
	c.back.line(x0, y0, x1, y1, n)
}

func (c *ctx) draw() {
	c.back.do()
	c.back.fix()
	te.Flush()
	te.Clear(te.ColorDefault, te.ColorDefault)
}

func spec1(c *ctx, out []complex128, wb *[]float64, a []float64, dir int) (float64, float64) {
	ilen := len(c.colors) - 1
	w, h := c.xy()
	if *flip {
		w, h = h, w
	}
	h /= 2

	bars := *bars_
	if bars == 0 {
		bars = w / *bard_
	}
	barw := *barw_
	if barw == 0 {
		barw = *bard_ - 1
	}

	if len(*wb) != bars {
		*wb = make([]float64, bars)
	}

	m := 1.0
	jo := 0.0
	va := 0.0
	for i := 0; i < bars; i++ {
		// XXX: refactor jesus christ
		// XXX: should cut to around 50Hz-20kHz
		ii := float64(i) * (1 / float64(bars) * 0.8)
		// make each band a fraction of an octave
		j := math.Pow(ii, 2)
		oi := int(float64(len(out)) * jo)
		jl := max(int(float64(len(out))*(j-jo)), 1)
		v := 0.0
		for z := 0; z < jl && z+oi < len(out); z++ {
			v += cmplx.Abs(out[z+oi]) / float64(len(out))
		}
		v /= float64(jl)
		jo = j

		// calculate dB
		v = 10 * math.Log10(math.Pow(v, *pow))

		// a-weighting
		v += a[int(j*float64(len(a)))]

		// cut off and scale to [0,1]
		v += c.sc
		v /= c.sc

		//v *= smooth[int(j*float64(len(smooth)))]
		v = math.Max(0, v)
		v = math.Min(1, v)
		v = math.Pow(v, 2.00)

		max := 0.9
		if v*c.mul > max {
			m *= 0.95 + (max / (v * c.mul) * 0.05)
		}

		v *= c.mul
		va += v
		x := (*wb)[i]
		if x > v {
			(*wb)[i] = x*0.7 + v*0.3
		} else {
			(*wb)[i] = v
		}
	}

	for i := 0; i < bars; i++ {
		hd := int((*wb)[i] * float64(h))
		for j := 0; j < hd; j++ {
			j := j
			if j > h {
				j = h
			}
			//co := int(v * float64(ilen) * (1.5 - float64(h-j)/float64(h)))
			co := int((*wb)[i] * float64(ilen))
			x, y := (i * (w / bars)), h-j*dir
			for off := 0; off < barw; off++ {
				x, y := x+off, y
				if *flip {
					x, y = y, x
				}
				c.set(x, y, min(ilen, co))
			}
		}
	}

	sc := 1.0
	va /= float64(bars)
	if va > 0.25 {
		sc *= 0.99
	} else {
		sc *= 1.01
	}
	return m, sc
}

func aweigh(x float64) float64 {
	return linterp(awfreq, awdb, x)
}

func linterp(x []float64, y []float64, xx float64) float64 {
	if x[0] > xx {
		return y[0]
	}
	for i := 1; i < len(x); i++ {
		if x[i] > xx {
			return y[i-1] + ((xx-x[i-1])/(x[i]-x[i-1]))*(y[i]-y[i-1])
		}
	}
	return y[len(y)-1]
}

func null(x int) []float64 {
	w := make([]float64, x)
	for i := range w {
		w[i] = 1.0
	}
	return w
}

func spectrum(c *ctx) {
	defer catch(c.die)
	var (
		inr      = c.rSamples
		inl      = c.lSamples
		outLen   = len(inr)/2 + 1
		outr     = fftw.Alloc1d(outLen)
		outl     = fftw.Alloc1d(outLen)
		lwb, rwb = []float64(nil), []float64(nil)
		planr    = fftw.PlanDftR2C1d(inr, outr, fftw.Estimate)
		planl    = fftw.PlanDftR2C1d(inl, outl, fftw.Estimate)
		window   = window.Hann(len(inr))
	)

	a := make([]float64, outLen)
	for i := 0; i < len(a); i++ {
		a[i] = aweigh(float64(i * (44100 / 2) / outLen))
	}

	c.mul = 7.0
	for {
		c.mul += 0.008
		if c.mul >= 400.0 {
			c.mul = 400.0
		}
		c.readRaw()
		c.convRaw()
		for i := range inr {
			inr[i] *= window[i]
			inl[i] *= window[i]
		}
		planr.Execute()
		planl.Execute()
		rm, rs := spec1(c, outr, &rwb, a, 1)
		lm, ls := spec1(c, outl, &lwb, a, -1)
		c.mul *= rm * lm
		c.sc *= math.Min(math.Max(rs*ls, 0.995), 1.005)
		c.sc = math.Min(math.Max(c.sc, 30), 120)
		c.draw()
	}
}
