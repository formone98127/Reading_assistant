package parser

import (
	"fmt"
	"io"
	"strings"
)

// extractMOBI reads uncompressed or PalmDOC-compressed text from common MOBI/KF7 files.
func extractMOBI(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if len(data) < 78 {
		return "", fmt.Errorf("mobi: file too small")
	}
	numRecords := int(uint16(data[76])<<8 | uint16(data[77]))
	if numRecords < 2 {
		return "", fmt.Errorf("mobi: no records")
	}
	listStart := 78
	offsets := make([]int, numRecords)
	for i := 0; i < numRecords; i++ {
		pos := listStart + i*8
		if pos+4 > len(data) {
			return "", fmt.Errorf("mobi: corrupt record list")
		}
		offsets[i] = int(uint32(data[pos])<<24 | uint32(data[pos+1])<<16 | uint32(data[pos+2])<<8 | uint32(data[pos+3]))
	}

	record0 := sliceRecord(data, offsets, 0)
	if len(record0) < 16 {
		return "", fmt.Errorf("mobi: invalid header record")
	}
	comp := int(uint16(record0[0])<<8 | uint16(record0[1]))
	firstText := int(uint16(record0[8])<<8 | uint16(record0[9]))
	textCount := int(uint16(record0[10])<<8 | uint16(record0[11]))
	if firstText <= 0 || firstText >= numRecords {
		return "", fmt.Errorf("mobi: invalid text record index")
	}

	var b strings.Builder
	for i := 0; i < textCount; i++ {
		recIdx := firstText + i
		if recIdx >= numRecords {
			break
		}
		raw := sliceRecord(data, offsets, recIdx)
		dec, err := decompressRecord(raw, comp)
		if err != nil {
			return "", err
		}
		b.WriteString(dec)
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", fmt.Errorf("mobi: no text extracted (DRM, KF8-only, or HuffCDIC may need conversion to EPUB)")
	}
	return out, nil
}

func sliceRecord(data []byte, offsets []int, idx int) []byte {
	start := offsets[idx]
	end := len(data)
	if idx+1 < len(offsets) {
		end = offsets[idx+1]
	}
	if start < 0 || start >= end || end > len(data) {
		return nil
	}
	return data[start:end]
}

func decompressRecord(in []byte, comp int) (string, error) {
	switch comp {
	case 1:
		return string(in), nil
	case 2:
		return palmDocUnpack(in), nil
	default:
		return "", fmt.Errorf("mobi: compression type %d not supported", comp)
	}
}

func palmDocUnpack(src []byte) string {
	var out []byte
	for i := 0; i < len(src); i++ {
		c := src[i]
		if c >= 1 && c <= 8 && i+int(c) < len(src) {
			out = append(out, src[i+1:i+1+int(c)]...)
			i += int(c)
			continue
		}
		switch c {
		case 0x00, 0x08, 0x0b, 0x0c:
			continue
		case 0x0a:
			out = append(out, '\n')
		case 0x0d:
			continue
		case 0x12:
			if i+1 < len(src) {
				out = append(out, src[i+1]^0x20)
				i++
			}
		case 0x1f:
			if i+2 < len(src) {
				dist := (int(src[i+1])<<8 | int(src[i+2])) & 0x3fff
				length := int(c&0x07) + 3
				if dist > 0 && dist <= len(out) {
					start := len(out) - dist
					for j := 0; j < length && start+j < len(out); j++ {
						out = append(out, out[start+j])
					}
				}
				i += 2
			}
		default:
			if c >= 0x80 && i+1 < len(src) {
				out = append(out, ' ', c^0x80)
				i++
			} else {
				out = append(out, c)
			}
		}
	}
	return string(out)
}
