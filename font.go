package main

import (
	"encoding/binary"
	"io/ioutil"
	"os"
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
		logger.Panicw("unable to read font header",
			"err", err)
	}

	if header[0] != 0x72 || header[1] != 0xb5 || header[2] != 0x4a || header[3] != 0x86 {
		logger.Panic("file is not a psf font")
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
