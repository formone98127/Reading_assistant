package audio

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	targetRMS = 0.08
	maxPeak   = 0.95
)

// NormalizeWAV scales PCM WAV loudness to a consistent RMS with peak limiting.
// Returns the input unchanged when parsing fails or audio is silent.
func NormalizeWAV(data []byte) ([]byte, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return data, nil
	}

	var (
		audioFormat   uint16
		numChannels   uint16
		bitsPerSample uint16
		dataOffset    int
		dataSize      int
	)

	offset := 12
	for offset+8 <= len(data) {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkStart := offset + 8
		if chunkStart+chunkSize > len(data) {
			break
		}
		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return data, nil
			}
			audioFormat = binary.LittleEndian.Uint16(data[chunkStart:])
			numChannels = binary.LittleEndian.Uint16(data[chunkStart+2:])
			bitsPerSample = binary.LittleEndian.Uint16(data[chunkStart+14:])
		case "data":
			dataOffset = chunkStart
			dataSize = chunkSize
		}
		offset = chunkStart + chunkSize
		if chunkSize%2 == 1 {
			offset++
		}
	}

	if dataOffset == 0 || dataSize == 0 || numChannels == 0 {
		return data, nil
	}
	if audioFormat != 1 && audioFormat != 3 {
		return data, nil
	}

	samples, err := decodeSamples(data[dataOffset : dataOffset+dataSize], audioFormat, bitsPerSample, numChannels)
	if err != nil || len(samples) == 0 {
		return data, err
	}

	gain := rmsGain(samples)
	if gain == 1 {
		return data, nil
	}

	for i := range samples {
		samples[i] *= gain
	}

	encoded, err := encodeSamples(samples, audioFormat, bitsPerSample, numChannels)
	if err != nil {
		return data, err
	}
	if len(encoded) > dataSize {
		return data, fmt.Errorf("wavnorm: encoded size mismatch")
	}

	out := make([]byte, len(data))
	copy(out, data)
	copy(out[dataOffset:dataOffset+len(encoded)], encoded)
	return out, nil
}

func rmsGain(samples []float64) float64 {
	var sum float64
	for _, s := range samples {
		sum += s * s
	}
	rms := math.Sqrt(sum / float64(len(samples)))
	if rms < 1e-6 {
		return 1
	}
	gain := targetRMS / rms
	peak := 0.0
	for _, s := range samples {
		v := math.Abs(s * gain)
		if v > peak {
			peak = v
		}
	}
	if peak > maxPeak {
		gain *= maxPeak / peak
	}
	return gain
}

func decodeSamples(raw []byte, audioFormat, bitsPerSample, numChannels uint16) ([]float64, error) {
	switch audioFormat {
	case 3: // IEEE float
		if bitsPerSample != 32 {
			return nil, fmt.Errorf("wavnorm: unsupported float bits %d", bitsPerSample)
		}
		if len(raw)%4 != 0 {
			return nil, fmt.Errorf("wavnorm: bad float data length")
		}
		n := len(raw) / 4
		out := make([]float64, n)
		for i := 0; i < n; i++ {
			bits := binary.LittleEndian.Uint32(raw[i*4:])
			out[i] = float64(math.Float32frombits(bits))
		}
		return out, nil
	case 1: // PCM
		switch bitsPerSample {
		case 16:
			if len(raw)%2 != 0 {
				return nil, fmt.Errorf("wavnorm: bad pcm16 data length")
			}
			n := len(raw) / 2
			out := make([]float64, n)
			for i := 0; i < n; i++ {
				v := int16(binary.LittleEndian.Uint16(raw[i*2:]))
				out[i] = float64(v) / 32768.0
			}
			return out, nil
		default:
			return nil, fmt.Errorf("wavnorm: unsupported pcm bits %d", bitsPerSample)
		}
	default:
		return nil, fmt.Errorf("wavnorm: unsupported format %d", audioFormat)
	}
}

func encodeSamples(samples []float64, audioFormat, bitsPerSample, numChannels uint16) ([]byte, error) {
	_ = numChannels
	switch audioFormat {
	case 3:
		raw := make([]byte, len(samples)*4)
		for i, s := range samples {
			binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(float32(s)))
		}
		return raw, nil
	case 1:
		raw := make([]byte, len(samples)*2)
		for i, s := range samples {
			if s > 1 {
				s = 1
			} else if s < -1 {
				s = -1
			}
			v := int16(math.Round(s * 32767))
			binary.LittleEndian.PutUint16(raw[i*2:], uint16(v))
		}
		return raw, nil
	default:
		return nil, fmt.Errorf("wavnorm: unsupported format %d", audioFormat)
	}
}
