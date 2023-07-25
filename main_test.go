package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScanChecksumMD5(t *testing.T) {
	data := strings.NewReader("md5: e3b56c4d7669e6577629efcc3eadbd38  filename.img")
	checksum, err := scanChecksum("filename.img", data)

	assert.Nil(t, err)
	assert.Equal(t, "e3b56c4d7669e6577629efcc3eadbd38", checksum)
}

func TestScanChecksumMD5SUM(t *testing.T) {
	data := strings.NewReader("e3b56c4d7669e6577629efcc3eadbd38  filename.img")
	checksum, err := scanChecksum("filename.img", data)

	assert.Nil(t, err)
	assert.Equal(t, "e3b56c4d7669e6577629efcc3eadbd38", checksum)
}

func TestScanChecksumSHA1(t *testing.T) {
	data := strings.NewReader("sha1: ef455af318e57186f66e056ffd6daa64271bc654  filename.img")
	checksum, err := scanChecksum("filename.img", data)

	assert.Nil(t, err)
	assert.Equal(t, "ef455af318e57186f66e056ffd6daa64271bc654", checksum)
}

func TestScanChecksumSHA1SUM(t *testing.T) {
	data := strings.NewReader("ef455af318e57186f66e056ffd6daa64271bc654  filename.img")
	checksum, err := scanChecksum("filename.img", data)

	assert.Nil(t, err)
	assert.Equal(t, "ef455af318e57186f66e056ffd6daa64271bc654", checksum)
}

func TestScanChecksumSHA256(t *testing.T) {
	data := strings.NewReader("sha256: 0261f6c651ef377b13367412049230b5f0307f38e62dc8f78d3525ef89df8c02  filename.img")
	checksum, err := scanChecksum("filename.img", data)

	assert.Nil(t, err)
	assert.Equal(t, "0261f6c651ef377b13367412049230b5f0307f38e62dc8f78d3525ef89df8c02", checksum)
}

func TestScanChecksumSHA256SUM(t *testing.T) {
	data := strings.NewReader("0261f6c651ef377b13367412049230b5f0307f38e62dc8f78d3525ef89df8c02  filename.img")
	checksum, err := scanChecksum("filename.img", data)

	assert.Nil(t, err)
	assert.Equal(t, "0261f6c651ef377b13367412049230b5f0307f38e62dc8f78d3525ef89df8c02", checksum)
}

func TestScanChecksumRockyLinux(t *testing.T) {
	data := strings.NewReader("# Comment\nSHA256(filename.img) = 0261f6c651ef377b13367412049230b5f0307f38e62dc8f78d3525ef89df8c02")
	checksum, err := scanChecksum("filename.img", data)

	assert.Nil(t, err)
	assert.Equal(t, "0261f6c651ef377b13367412049230b5f0307f38e62dc8f78d3525ef89df8c02", checksum)
}
