package common

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	BUF_SIZE     = 4096
	TYPE_NO_LOCK = iota
	TYPE_SHARE_LOCK
	TYPE_EXCLUDE_LOCK

	SLEEP_TICK = 100 // millisecond

	Maxint32 = 1<<31 - 1

	RESULT_DELETED   = "Deleted"
	RESULT_ACCEPTED  = "Accepted"
	RESULT_UPDATED   = "Updated"
	RESULT_UNCHANGED = "Unchanged"
	RESULT_CREATED   = "Created"
)

var (
	plog = GetLogger("github.com/xcat2/goconserver/common")
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
	_, err = out.WriteTo(f)
	defer f.Close()
	return
}

func PrintJson(b []byte) {
	var out bytes.Buffer
	json.Indent(&out, b, "", "\t")
	_, err := out.WriteTo(os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to print json data, Error:%s\n", err.Error())
	}
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
		return ErrTimeout
	}
	return nil
}

func RequireLock(reserve *int, rwlock *sync.RWMutex, share bool) error {
	if *reserve == TYPE_EXCLUDE_LOCK || (*reserve == TYPE_SHARE_LOCK && !share) {
		return ErrLocked
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
		return ErrLocked
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
		return ErrUnlocked
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
		return ErrUnlocked
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

func SafeClose(ch chan struct{}) (justClosed bool) {
	defer func() {
		if recover() != nil {
			justClosed = false
		}
	}()
	if ch == nil {
		return false
	}
	close(ch) // panic if ch is closed
	return true
}

func SafeSend(ch chan struct{}, data struct{}) (closed bool) {
	defer func() {
		if recover() != nil {
			closed = true
		}
	}()
	if ch == nil {
		return true
	}
	select {
	case ch <- data:
	default:
	}
	return false
}

func ReverseStringSlice(l []string) {
	for i := 0; i < int(len(l)/2); i++ {
		li := len(l) - i - 1
		l[i], l[li] = l[li], l[i]
	}
}

func ReadTail(path string, tail int) (string, error) {
	b := make([]byte, BUF_SIZE)
	ret := make([]string, tail)
	cur := tail - 1
	fd, err := os.OpenFile(path, os.O_RDONLY, 0600)
	if err != nil {
		return "", err
	}
	defer fd.Close()
	info, err := fd.Stat()
	if err != nil {
		return "", err
	}
	l := info.Size()
	pos := l - BUF_SIZE
	if pos < 0 {
		pos = 0
	}
	var buf []byte
	for pos >= 0 {
		_, err = fd.Seek(pos, os.SEEK_CUR)
		if err != nil {
			return "", err
		}
		_, err := fd.Read(b)
		if err != nil && err != io.EOF {
			return "", err
		}
		if ret[cur] != "" {
			buf = append(b, []byte(ret[cur])...)
		} else {
			buf = b
		}
		lines := strings.Split(string(buf), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if cur < 0 {
				return strings.Join(ret, "\n"), nil
			}
			ret[cur] += lines[i]
			if i != 0 {
				cur--
			}
		}
		pos -= BUF_SIZE
	}
	return strings.Join(ret[cur:], "\n"), nil
}
