//go:build ignore

package main

import (
	"bytes"
	"encoding/binary"
	"golang.org/x/image/draw"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

var artwork image.Image

func icon(size int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(out, out.Bounds(), artwork, artwork.Bounds(), draw.Src, nil)
	return out
}
func savePNG(name string, im image.Image) {
	if e := os.MkdirAll(filepath.Dir(name), 0755); e != nil {
		panic(e)
	}
	f, e := os.Create(name)
	if e != nil {
		panic(e)
	}
	if e = png.Encode(f, im); e != nil {
		panic(e)
	}
	if e = f.Close(); e != nil {
		panic(e)
	}
}
func androidIcons() {
	// 108dp adaptive canvas: keep all colored artwork inside the 66dp safe circle.
	foreground := image.NewNRGBA(image.Rect(0, 0, 432, 432))
	draw.CatmullRom.Scale(foreground, image.Rect(70, 70, 362, 362), artwork, artwork.Bounds(), draw.Src, nil)
	savePNG("android/res/drawable-xxxhdpi/icon_foreground.png", foreground)
	// Themed icons use the mascot and note silhouette, without the dark tile.
	mono := image.NewNRGBA(foreground.Bounds())
	for y := 0; y < 432; y++ {
		for x := 0; x < 432; x++ {
			p := foreground.NRGBAAt(x, y)
			if p.A > 0 && (p.R > 70 || p.G > 70 || p.B > 90) {
				mono.SetNRGBA(x, y, color.NRGBA{255, 255, 255, p.A})
			}
		}
	}
	savePNG("android/res/drawable-xxxhdpi/icon_monochrome.png", mono)
	for _, d := range []struct {
		name string
		size int
	}{{"mdpi", 48}, {"hdpi", 72}, {"xhdpi", 96}, {"xxhdpi", 144}, {"xxxhdpi", 192}} {
		savePNG("android/res/mipmap-"+d.name+"/ic_launcher.png", icon(d.size))
	}
}
func main() {
	source, e := os.Open("assets/icon-source.png")
	if e != nil {
		panic(e)
	}
	artwork, e = png.Decode(source)
	source.Close()
	if e != nil {
		panic(e)
	}
	if artwork.Bounds().Dx() != artwork.Bounds().Dy() {
		panic("Icon artwork must be square")
	}
	androidIcons()
 if e:=os.WriteFile("assets/brand.rgba",icon(32).Pix,0644);e!=nil{panic(e)}
	os.MkdirAll("assets", 0755)
	f, e := os.Create("assets/icon.png")
	if e != nil {
		panic(e)
	}
	if e = png.Encode(f, icon(256)); e != nil {
		panic(e)
	}
	f.Close()
	sizes := []int{16, 24, 32, 48, 64, 128, 256}
	var blobs [][]byte
	offset := 6 + 16*len(sizes)
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, []uint16{0, 1, uint16(len(sizes))})
	for _, n := range sizes {
		im := icon(n)
		var b bytes.Buffer
		mask := ((n + 31) / 32) * 4
		binary.Write(&b, binary.LittleEndian, uint32(40))
		binary.Write(&b, binary.LittleEndian, int32(n))
		binary.Write(&b, binary.LittleEndian, int32(n*2))
		binary.Write(&b, binary.LittleEndian, []uint16{1, 32})
		binary.Write(&b, binary.LittleEndian, []uint32{0, uint32(n * n * 4), 0, 0, 0, 0})
		for y := n - 1; y >= 0; y-- {
			for x := 0; x < n; x++ {
				v := im.NRGBAAt(x, y)
				b.Write([]byte{v.B, v.G, v.R, v.A})
			}
		}
		b.Write(make([]byte, mask*n))
		blob := b.Bytes()
		blobs = append(blobs, blob)
		v := byte(n)
		out.Write([]byte{v, v, 0, 0})
		binary.Write(&out, binary.LittleEndian, []uint16{1, 32})
		binary.Write(&out, binary.LittleEndian, []uint32{uint32(len(blob)), uint32(offset)})
		offset += len(blob)
	}
	for _, b := range blobs {
		out.Write(b)
	}
	if e = os.WriteFile("assets/icon.ico", out.Bytes(), 0644); e != nil {
		panic(e)
	}
}
