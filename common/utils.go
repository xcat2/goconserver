package common

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"

	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"time"
)

var (
	plog = GetLogger("github.com/chenglch/consoleserver/common")
)

func init() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stderr)
	serverConfig = new(ServerConfig)
}

func InitLogger() {

	if serverConfig == nil {
		log.SetOutput(os.Stderr)
		return
	}
	logFile := serverConfig.Global.LogFile
	if logFile == "" {
		log.SetOutput(os.Stderr)
		return
	}
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY, 0666)
	if err == nil {
		log.SetOutput(f)
	} else {
		log.Info("Failed to log to file, using default stderr")
		log.SetOutput(os.Stderr)
	}
}

func WriteJsonFile(filepath string, data []byte) (err error) {
	var out bytes.Buffer
	json.Indent(&out, data, "", "\t")
	f, err := os.Create(filepath)
	if err != nil {
		return err
	}
	out.WriteTo(f)
	defer f.Close()
	return
}

func PathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func Float32ToByte(float float32) []byte {
	bits := math.Float32bits(float)
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, bits)
	return bytes
}

func ByteToFloat32(bytes []byte) float32 {
	bits := binary.LittleEndian.Uint32(bytes)
	return math.Float32frombits(bits)
}

func Float64ToByte(float float64) []byte {
	bits := math.Float64bits(float)
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, bits)
	return bytes
}

func ByteToFloat64(bytes []byte) float64 {
	bits := binary.LittleEndian.Uint64(bytes)
	return math.Float64frombits(bits)
}

func IntToBytes(n int) []byte {
	bytesBuffer := bytes.NewBuffer([]byte{})
	binary.Write(bytesBuffer, binary.BigEndian, int32(n))
	return bytesBuffer.Bytes()
}

func BytesToInt(b []byte) int {
	bytesBuffer := bytes.NewBuffer(b)
	var tmp int32
	binary.Read(bytesBuffer, binary.BigEndian, &tmp)
	return int(tmp)
}

func TimeoutChan(c chan bool, t int) error {
	timeout := make(chan bool, 1)
	go func() {
		time.Sleep(time.Duration(t) * time.Second)
		timeout <- true
	}()
	select {
	case <-c:
	case <-timeout:
		return errors.New(fmt.Sprintf("Timeout happens after waiting %d seconds", t))
	}
	return nil
}
