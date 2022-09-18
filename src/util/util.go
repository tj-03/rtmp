package util

import (
	"encoding/binary"
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	InactiveTimeout int    `json:"inactiveTimeout"`
	LogDirectory    string `json:"logDirectory"`
}

func CmpSlice(data1 []byte, data2 []byte) bool {
	if len(data1) != len(data2) {
		return false
	}
	for i := range data1 {
		if data1[i] != data2[i] {
			return false
		}
	}
	return true
}

func ReadConfig() (Config, error) {
	fileData, err := ioutil.ReadFile("config.json")
	if err != nil {
		return Config{}, err
	}

	var config Config
	err = json.Unmarshal(fileData, &config)
	if err != nil {
		return Config{}, err
	}
	return config, nil

}

func Uint32ToBuf(num uint32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, num)
	return buf
}

func Uint16ToBuf(num uint16) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, num)
	return buf
}

func FilterSlice[T any](s []T, shouldExlude func(obj T) bool) []T {
	filtered := []T{}
	for i := range s {
		if shouldExlude(s[i]) {
			continue
		}
		filtered = append(filtered, s[i])

	}
	return filtered
}

func Max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func Min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
