package main

import (
	"encoding/binary"
	"io/ioutil"
	"os"
)

const (
	displayHeight  = 16
	displayWidth   = 280
	bytesPerColumn = 2
)

type psfFont struct {
	Version       uint32
	HeaderSize    uint32
	Flags         uint32
	NumGlyphs     uint32
	BytesPerGlyph uint32
	Height        uint32
	Width         uint32
	GlyphBuffer   []byte
}

func readPSF(f *os.File) psfFont {
	header := make([]byte, 32)
	_, err := f.Read(header)
	if err != nil {
		panic(err)
	}

	if header[0] != 0x72 || header[1] != 0xb5 || header[2] != 0x4a || header[3] != 0x86 {
		panic("file is not a psf font")
	}

	p := psfFont{
		Version:       binary.LittleEndian.Uint32(header[4:8]),
		HeaderSize:    binary.LittleEndian.Uint32(header[8:12]),
		Flags:         binary.LittleEndian.Uint32(header[12:16]),
		NumGlyphs:     binary.LittleEndian.Uint32(header[16:20]),
		BytesPerGlyph: binary.LittleEndian.Uint32(header[20:24]),
		Height:        binary.LittleEndian.Uint32(header[24:28]),
		Width:         binary.LittleEndian.Uint32(header[28:32]),
	}

	f.Seek(32, 0)
	b, _ := ioutil.ReadAll(f)
	p.GlyphBuffer = b

	return p
}

func (p psfFont) getChar(chr int) []byte {
	char := make([]byte, p.BytesPerGlyph)
	for k := range char {
		char[k] = p.GlyphBuffer[chr*int(p.BytesPerGlyph)+k]
	}

	return char
}

func (c *client) setChar(chr []byte, offset int) {
	i := bytesPerColumn * offset * 8
	for col := 0; col < 8; col++ {
		for k := range chr {
			v := chr[len(chr)-1-(k+8)%16]
			mask := byte(0b10000000 >> col)
			if v&mask > 0 {
				squeezeMask := byte(1 << (i % 8))
				c.framebuffer[i/8] |= squeezeMask
			}

			i++
		}
	}
}

func (c *client) setText(text string) {
	if len(text) > displayWidth/8 {
		panic("text too long to display")
	}

	for k, v := range text {
		c.setChar(c.font.getChar(int(v)), k*8)
	}
}
