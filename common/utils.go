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
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const (
	TYPE_NO_LOCK = iota
	TYPE_SHARE_LOCK
	TYPE_EXCLUDE_LOCK

	SLEEP_TICK    = 100 // millisecond
	ACTION_DELETE = 1
	ACTION_PUT    = 0
	ACTION_NIL    = -1

	Maxint32 = 1<<31 - 1
)

var (
	plog = GetLogger("github.com/chenglch/goconserver/common")
)

func init() {
	log.SetFormatter(&log.JSONFormatter{})
	log.SetOutput(os.Stderr)
	serverConfig = new(ServerConfig)
}

func WriteJsonFile(filepath string, data []byte) (err error) {
	var out bytes.Buffer
	json.Indent(&out, data, "", "\t")
	f, err := os.OpenFile(filepath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	out.WriteTo(f)
	defer f.Close()
	return
}

func PrintJson(b []byte) {
	var out bytes.Buffer
	json.Indent(&out, b, "", "\t")
	out.WriteTo(os.Stdout)
	fmt.Printf("\n")
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
		plog.Warn(fmt.Sprintf("Timeout happens after waiting %d seconds", t))
	}
	return nil
}

func RequireLock(reserve *int, rwlock *sync.RWMutex, share bool) error {
	if *reserve == TYPE_EXCLUDE_LOCK || (*reserve == TYPE_SHARE_LOCK && !share) {
		return errors.New(fmt.Sprintf("%s: Locked, temporary unavailable"))
	}
	if share == true {
		rwlock.RLock()
	} else {
		rwlock.Lock()
	}
	defer func() {
		if share == true {
			rwlock.RUnlock()
		} else {
			rwlock.Unlock()
		}
	}()
	// with lock and check again
	if *reserve == TYPE_EXCLUDE_LOCK || (*reserve == TYPE_SHARE_LOCK && !share) {
		return errors.New(fmt.Sprintf("%s: Locked, temporary unavailable"))
	}
	if share == true {
		*reserve = TYPE_SHARE_LOCK
	} else {
		*reserve = TYPE_EXCLUDE_LOCK
	}
	return nil
}

func ReleaseLock(reserve *int, rwlock *sync.RWMutex, share bool) error {
	if *reserve == TYPE_NO_LOCK {
		return errors.New(fmt.Sprintf("%s: Not locked"))
	}
	if share == true {
		rwlock.RLock()
	} else {
		rwlock.Lock()
	}
	defer func() {
		if share == true {
			rwlock.RUnlock()
		} else {
			rwlock.Unlock()
		}
	}()
	if *reserve == TYPE_NO_LOCK {
		return errors.New(fmt.Sprintf("%s: Not locked"))
	}
	*reserve = TYPE_NO_LOCK
	return nil
}

func CopyFile(dst, src string) (int64, error) {
	s, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer s.Close()
	d, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return 0, err
	}
	defer d.Close()
	d.Truncate(0)
	if err != nil {
		return 0, err
	}
	return io.Copy(d, s)
}

func Notify(c chan bool, addr *uint32, val uint32) {
	old := atomic.SwapUint32(addr, val)
	if old != val {
		c <- true
	}
}

func Wait(c chan bool, addr *uint32, val uint32, fc interface{}) {
	for {
		time.Sleep(time.Duration(SLEEP_TICK * time.Millisecond))
		select {
		case <-c:
			atomic.SwapUint32(addr, val)
			fc.(func())()
		}
	}
}

func If(condition bool, trueVal, falseVal interface{}) interface{} {
	if condition {
		return trueVal
	}
	return falseVal
}
