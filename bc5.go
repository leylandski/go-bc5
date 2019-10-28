// Copyright 2019 Adam Leyland
// Use of this source code is governed by a BSD-2 style license that can be found in the LICENSE file.

// A package containing an implementation of the BC5 red/green image compression algorithm.
package bc5

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"io"
	"io/ioutil"
	"math"
	"os"
)

// Alias for decompression blue computation constants.
type BlueMode int

const (
	Zero          BlueMode = iota //Always set the blue component to 0 during decompression.
	One                           //Always set the blue component to 1 during decompression.
	ComputeNormal                 //Compute the normal as (sqrt(1-((2*r-1)^2+(2*g-1)^2)))/2+0.5. Suitable for normalised maps.
	Greyscale                     //Computes the blue component to be identical to the red component per pixel.
)

// BC5 holds BC5-compressed red/green image data.
// The spec can be found here: https://docs.microsoft.com/en-us/windows/win32/direct3d10/d3d10-graphics-programming-guide-resources-block-compression#bc5
type BC5 struct {
	Data []byte
	Rect image.Rectangle
	BlueMode
}

// Load reads BC5 encoded image data from imgfile into a BC5 and
// returns a pointer to it. It will return an error if one occurred.
func NewBC5FromFile(bcfile string) (*BC5, error) {

	f, err := os.Open(bcfile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return Decode(f)
}

// NewBC5FromRGBA returns a BC5 containing the compressed form of an RGBA image.
func NewBC5FromRGBA(rgba *image.RGBA) (*BC5, error) {

	img := new(BC5)
	err := img.SetFromRGBA(rgba)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// At performs on-the-fly decompression of b and returns the RGBA color at (x,y).
func (b BC5) At(x, y int) color.RGBA {

	if x < 0 || x >= b.Rect.Size().X || y < 0 || y >= b.Rect.Size().Y {
		//Out of bounds
		return color.RGBA{}
	}

	blockIx := (int(float32(y)/4) * b.Rect.Size().Y) + int(float32(x)/4)*16
	block := decompressBlock(b.Data[blockIx:blockIx+16], b.BlueMode)
	return block.RGBAAt(x%4, y%4)
}

// Size returns the number of bytes of pixel data b holds
func (b BC5) Size() int32 {

	return int32(b.Rect.Size().X) * int32(b.Rect.Size().Y)
}

// SetFromRGBA encodes RGBA data into this BC5 image.
// As this is a red/green compression scheme, the blue and alpha components of the source are discarded.
func (b *BC5) SetFromRGBA(rgba *image.RGBA) error {

	if rgba.Rect.Size().X != rgba.Rect.Size().Y {
		return errors.New("image must be square")
	}

	if rgba.Rect.Size().X%4 != 0 {
		return errors.New("size must be a multiple of 4")
	}

	blocks := makeBlocks(rgba)

	b.Data = make([]byte, len(blocks)*16)
	for i := 0; i < len(blocks); i++ {
		pos := i * 16
		c := compressBlock(blocks[i])
		copy(b.Data[pos:pos+16], c)
	}
	b.Rect = rgba.Rect
	return nil
}

// Decompress returns an RGBA image containing the decompressed contents of b.
func (b BC5) Decompress() *image.RGBA {

	blocks := make([]*image.RGBA, len(b.Data)/16)
	for i := 0; i < len(blocks); i++ {
		pos := i * 16
		blocks[i] = decompressBlock(b.Data[pos:pos+16], b.BlueMode)
	}

	rgba := image.NewRGBA(b.Rect)
	for y := 0; y < rgba.Rect.Size().Y; y++ {
		for x := 0; x < b.Rect.Size().X; x++ {

			blockIx := (int(float32(y)/4) * b.Rect.Size().X / 4) + int(float32(x)/4)
			rgba.SetRGBA(x, y, blocks[blockIx].RGBAAt(x%4, y%4))
		}
	}
	return rgba
}

// Decode reads BC5 encoded data from a reader into a new BC5 and returns a pointer to it.
// It expects a signature equal to "BC5 ", then two uint32 values for width and height,
// followed by all the block data. It will return an error if the data could not be
// decoded properly.
func Decode(r io.Reader) (*BC5, error) {

	readBytes, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if len(readBytes) < 12 {
		return nil, errors.New("not enough data for BC5")
	}

	buf := bytes.NewBuffer(readBytes)

	signature := binary.BigEndian.Uint32(buf.Next(4))
	if signature != strToDword("BC5 ") {
		return nil, errors.New("invalid file signature")
	}

	width := binary.BigEndian.Uint32(buf.Next(4))
	height := binary.BigEndian.Uint32(buf.Next(4))

	if len(readBytes) < 13 {
		return nil, errors.New("no image data found")
	}

	img := new(BC5)
	img.Rect = image.Rect(0, 0, int(width), int(height))
	img.Data = readBytes[12:]
	return img, nil
}

// Encode writes the contents of img to w, along with a 12 byte header containing the
// uint32 encoding of "BC5 ", followed by two more uint32 values for width and height,
// followed by all the block data.
func Encode(img *BC5, w io.Writer) error {

	headerBytes := make([]byte, 12)
	binary.BigEndian.PutUint32(headerBytes[:4], strToDword("BC5 "))
	binary.BigEndian.PutUint32(headerBytes[4:8], uint32(img.Rect.Size().X))
	binary.BigEndian.PutUint32(headerBytes[8:12], uint32(img.Rect.Size().Y))
	n, err := w.Write(headerBytes)
	if err != nil {
		return err
	}
	if n != 12 {
		return errors.New("failed to write header")
	}

	n, err = w.Write(img.Data)
	if err != nil {
		return err
	}
	if n != len(img.Data) {
		return errors.New("failed to write image data")
	}
	return nil
}

// converts string to uint32
func strToDword(s string) uint32 {

	b := []byte(s)
	return binary.BigEndian.Uint32(b)
}

// returns array of 4x4 sub-images from img
func makeBlocks(img *image.RGBA) []*image.RGBA {

	blocks := make([]*image.RGBA, (img.Rect.Size().X/4)*(img.Rect.Size().Y/4))
	currentBlockIx := 0
	for y := 0; y < img.Rect.Size().Y/4; y++ {
		for x := 0; x < img.Rect.Size().X/4; x++ {
			subImg := img.SubImage(image.Rect(x*4, y*4, (x*4)+4, (y*4)+4))
			blocks[currentBlockIx] = image.NewRGBA(image.Rect(0, 0, 4, 4))
			draw.Draw(blocks[currentBlockIx], blocks[currentBlockIx].Bounds(), subImg, subImg.Bounds().Min, draw.Src)
			currentBlockIx++
		}
	}

	return blocks
}

// returns 16 byte BC5 compressed block bytes for the given 4x4 RGBA image
func compressBlock(block *image.RGBA) []byte {

	var minR, maxR, minG, maxG byte = 255, 0, 255, 0
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			c := block.RGBAAt(x, y)

			if c.R < minR {
				minR = c.R
			}
			if c.R > maxR {
				maxR = c.R
			}

			if c.G < minG {
				minG = c.G
			}
			if c.G > maxG {
				maxG = c.G
			}
		}
	}

	palR := generatePalette(normalize(minR), normalize(maxR))
	palG := generatePalette(normalize(minG), normalize(maxG))
	nearest := func(pal [8]float64, v byte) byte {
		ni := 0
		for i := 0; i < 8; i++ {
			if math.Abs(pal[i]-normalize(v)) < math.Abs(pal[ni]-normalize(v)) {
				ni = i
			}
		}
		return byte(ni)
	}

	//Compare red and green values and select closest in palette
	rIndexU, gIndexU := uint64(0), uint64(0)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			c := block.RGBAAt(x, y)
			rIndexU = (rIndexU << 3) | uint64(nearest(palR, c.R))
			gIndexU = (gIndexU << 3) | uint64(nearest(palG, c.G))
		}
	}

	rIxBytes, gIxBytes := make([]byte, 8), make([]byte, 8)
	binary.BigEndian.PutUint64(rIxBytes, rIndexU)
	binary.BigEndian.PutUint64(gIxBytes, gIndexU)

	blockBytes := make([]byte, 16)
	blockBytes[0] = denormalize(palR[0])
	blockBytes[1] = denormalize(palR[1])
	copy(blockBytes[2:8], rIxBytes[2:8])

	blockBytes[8] = denormalize(palG[0])
	blockBytes[9] = denormalize(palG[1])
	copy(blockBytes[10:], gIxBytes[2:8])

	return blockBytes
}

// returns an RGBA image containing the decompressed contents of block
func decompressBlock(block []byte, blueMode BlueMode) *image.RGBA {

	if len(block) != 16 {
		panic("invalid block size")
	}

	//First two bytes are reference reds
	r := generatePalette(normalize(block[0]), normalize(block[1]))
	rIndices := getIndices(block[2:8])

	g := generatePalette(normalize(block[8]), normalize(block[9]))
	gIndices := getIndices(block[10:])

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	pxIndex := 0
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {

			pxR := denormalize(r[rIndices[pxIndex]])
			pxG := denormalize(g[gIndices[pxIndex]])
			var pxB byte
			switch blueMode {
			case ComputeNormal:
				pxB = denormalize((math.Sqrt(1-math.Pow(2*r[rIndices[pxIndex]]-1, 2)+math.Pow(2*g[gIndices[pxIndex]]-1, 2)))/2 + 0.5)
			case Greyscale:
				pxB = pxR
			case One:
				pxB = denormalize(1)
			default:
				pxB = 0
			}
			img.SetRGBA(x, y, color.RGBA{
				R: pxR,
				G: pxG,
				B: pxB,
				A: 1.0,
			})
			pxIndex++
		}
	}
	return img
}

// generates the block palette from the reference colors
func generatePalette(c0, c1 float64) [8]float64 {

	//Get signed float normalized palette (0 to 1)
	pal := [8]float64{}
	pal[0], pal[1] = c0, c1
	if c0 > c1 {
		pal[2] = (6*c0 + 1*c1) / 7
		pal[3] = (5*c0 + 2*c1) / 7
		pal[4] = (4*c0 + 3*c1) / 7
		pal[5] = (3*c0 + 4*c1) / 7
		pal[6] = (2*c0 + 5*c1) / 7
		pal[7] = (1*c0 + 6*c1) / 7
	} else {
		pal[2] = (4*c0 + 1*c1) / 5
		pal[3] = (3*c0 + 2*c1) / 5
		pal[4] = (2*c0 + 3*c1) / 5
		pal[5] = (1*c0 + 4*c1) / 5
		pal[6] = 0
		pal[7] = 1
	}
	return pal
}

// returns an array of 16 indices parsed from b, separating out the 3-bit index values
func getIndices(b []byte) [16]int {

	if len(b) != 6 {
		panic("invalid index array size")
	}

	data := binary.BigEndian.Uint64(append([]byte{0, 0}, b...))

	ix := [16]int{}
	for i := 0; i < 16; i++ {
		//Bit shift data right by i*3 and & with 0x0111 to get index
		ix[i] = int((data >> uint(i*3)) & 7)
	}
	return ix
}

// returns v as a float normalized between 0 and 1
func normalize(v byte) float64 {

	return float64(v) / 255
}

// returns a byte representation of the normalized float v
func denormalize(v float64) byte {

	return byte(v * 255)
}
