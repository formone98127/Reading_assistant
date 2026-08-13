package audio

import (
	"encoding/binary"
	"math"
	"testing"
)

func makeFloatWAV(samples []float32) []byte {
	const sr = 48000
	data := make([]byte, 44+len(samples)*4)
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:12], "WAVE")
	copy(data[12:16], "fmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 3) // float
	binary.LittleEndian.PutUint16(data[22:24], 1)
	binary.LittleEndian.PutUint32(data[24:28], sr)
	binary.LittleEndian.PutUint32(data[28:32], sr*4)
	binary.LittleEndian.PutUint16(data[32:34], 4)
	binary.LittleEndian.PutUint16(data[34:36], 32)
	copy(data[36:40], "data")
	binary.LittleEndian.PutUint32(data[40:44], uint32(len(samples)*4))
	for i, s := range samples {
		binary.LittleEndian.PutUint32(data[44+i*4:], math.Float32bits(s))
	}
	return data
}

func readWAVRMS(data []byte) (float64, error) {
	out, err := NormalizeWAV(data)
	if err != nil {
		return 0, err
	}
	offset := 44
	var sum float64
	for i := offset; i+4 <= len(out); i += 4 {
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(out[i : i+4])))
		sum += v * v
	}
	n := (len(out) - offset) / 4
	if n == 0 {
		return 0, nil
	}
	return math.Sqrt(sum / float64(n)), nil
}

func TestNormalizeWAV_matchesTarget(t *testing.T) {
	quiet := makeFloatWAV([]float32{0.01, -0.02, 0.015, -0.01})
	loud := makeFloatWAV([]float32{0.5, -0.45, 0.4, -0.35})

	qRMS, err := readWAVRMS(quiet)
	if err != nil {
		t.Fatal(err)
	}
	lRMS, err := readWAVRMS(loud)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(qRMS-targetRMS) > 0.02 {
		t.Fatalf("quiet rms=%f want ~%f", qRMS, targetRMS)
	}
	if math.Abs(lRMS-targetRMS) > 0.02 {
		t.Fatalf("loud rms=%f want ~%f", lRMS, targetRMS)
	}
	if math.Abs(qRMS-lRMS) > 0.02 {
		t.Fatalf("rms mismatch quiet=%f loud=%f", qRMS, lRMS)
	}
}
