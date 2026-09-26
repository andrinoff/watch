package dlna

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// IconPNG is the picture televisions show next to the device name. Generated
// here rather than shipped as a file: a projection lamp with a play mark,
// drawn at 240 px and scaled down by whatever asks for it.
func IconPNG() []byte {
	const size = 240
	canvas := image.NewRGBA(image.Rect(0, 0, size, size))

	lamp := color.RGBA{R: 0xe8, G: 0xb4, B: 0x5a, A: 0xff}
	mark := color.RGBA{R: 0x12, G: 0x10, B: 0x14, A: 0xff}
	centre := float64(size) / 2
	radius := float64(size) * 0.46

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - centre
			dy := float64(y) - centre
			if math.Hypot(dx, dy) <= radius {
				canvas.Set(x, y, lamp)
			}
		}
	}

	// A play triangle, filled with the room colour so it reads on the lamp.
	triangle := [3][2]float64{
		{centre - radius*0.26, centre - radius*0.42},
		{centre - radius*0.26, centre + radius*0.42},
		{centre + radius*0.44, centre},
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if insideTriangle(float64(x)+0.5, float64(y)+0.5, triangle) {
				canvas.Set(x, y, mark)
			}
		}
	}

	var out bytes.Buffer
	_ = png.Encode(&out, canvas)
	return out.Bytes()
}

func insideTriangle(px, py float64, t [3][2]float64) bool {
	sign := func(a, b, c [2]float64) float64 {
		return (a[0]-c[0])*(b[1]-c[1]) - (b[0]-c[0])*(a[1]-c[1])
	}
	p := [2]float64{px, py}
	d1 := sign(p, t[0], t[1])
	d2 := sign(p, t[1], t[2])
	d3 := sign(p, t[2], t[0])
	hasNegative := d1 < 0 || d2 < 0 || d3 < 0
	hasPositive := d1 > 0 || d2 > 0 || d3 > 0
	return !(hasNegative && hasPositive)
}
